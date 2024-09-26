package replikator

import (
	"context"
	"regexp"
	"time"

	jsonpatch "github.com/evanphx/json-patch"
	log "github.com/sirupsen/logrus"
	"github.com/yankeguo/rg"
	v1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const FieldManagerReplikator = "io.github.yankeguo/replikator"

type Task struct {
	interval time.Duration

	resource     schema.GroupVersionResource
	srcNamespace string
	srcName      string
	dstNamespace *regexp.Regexp
	dstName      string

	javascript string
	jsonpatch  jsonpatch.Patch
}

type TaskOptions struct {
	Client        *kubernetes.Clientset
	DynamicClient *dynamic.DynamicClient
}

func (t *Task) ListDestinationNamespaces(ctx context.Context, opts TaskOptions) (namespaces []string, err error) {
	var list *v1.NamespaceList
	if list, err = opts.Client.CoreV1().Namespaces().List(ctx, metaV1.ListOptions{}); err != nil {
		return
	}

	for _, namespace := range list.Items {
		if namespace.Name == t.srcNamespace {
			continue
		}
		if t.dstNamespace.MatchString(namespace.Name) {
			namespaces = append(namespaces, namespace.Name)
		}
	}
	return
}

func (t *Task) FetchSource(ctx context.Context, opts TaskOptions) (source *unstructured.Unstructured, err error) {
	if source, err = opts.DynamicClient.Resource(t.resource).Namespace(t.srcNamespace).Get(ctx, t.srcName, metaV1.GetOptions{}); err != nil {
		return
	}

	delete(source.Object, "status")
	if metadata, ok := source.Object["metadata"].(map[string]interface{}); ok {
		source.Object["metadata"] = map[string]interface{}{
			"name":        metadata["name"],
			"namespace":   metadata["namespace"],
			"labels":      metadata["labels"],
			"annotations": metadata["annotations"],
		}
	}
	return
}

func (t *Task) ReplicateSource(ctx context.Context, source *unstructured.Unstructured, namespace string, name string) (obj *unstructured.Unstructured, err error) {
	obj = source.DeepCopy()

	// update metadata
	if metadata, ok := obj.Object["metadata"].(map[string]interface{}); ok {
		metadata["namespace"] = namespace
		metadata["name"] = t.dstName
	}

	// apply jsonpatch
	if t.jsonpatch != nil {
		var buf []byte
		if buf, err = obj.MarshalJSON(); err != nil {
			return
		}
		if buf, err = t.jsonpatch.Apply(buf); err != nil {
			return
		}
		obj = &unstructured.Unstructured{}
		if err = obj.UnmarshalJSON(buf); err != nil {
			return
		}
	}

	// apply javascript
	if t.javascript != "" {
		var buf []byte
		if buf, err = obj.MarshalJSON(); err != nil {
			return
		}

		var out string
		if out, err = EvaluateJavascriptModification(string(buf), t.javascript); err != nil {
			return
		}

		obj = &unstructured.Unstructured{}
		if err = obj.UnmarshalJSON([]byte(out)); err != nil {
			return
		}
	}
	return
}

func (t *Task) Do(ctx context.Context, opts TaskOptions) (err error) {
	defer rg.Guard(&err)

	log := log.WithField("res", t.resource.String()).
		WithField(
			"src", t.srcNamespace+"/"+t.srcName,
		).
		WithField(
			"dst", t.dstNamespace.String()+"/"+t.dstName,
		)

	log.Info("task started")

	namespaces := rg.Must(t.ListDestinationNamespaces(ctx, opts))

	source := rg.Must(t.FetchSource(ctx, opts))

	for _, namespace := range namespaces {
		log := log.WithField("dst", namespace+"/"+t.dstName)

		obj := rg.Must(t.ReplicateSource(ctx, source, namespace, t.dstName))

		log.Info("replicating")

		if _, err = opts.DynamicClient.Resource(t.resource).Namespace(namespace).Apply(ctx, t.dstName, obj, metaV1.ApplyOptions{
			Force:        true,
			FieldManager: FieldManagerReplikator,
		}); err != nil {
			log.WithError(err).Error("replication failed")
			err = nil
		} else {
			log.Info("replicated")
		}
	}

	log.Info("task finished")

	return
}

func (t *Task) Run(ctx context.Context, opts TaskOptions) {
	for {
		if ctx.Err() != nil {
			return
		}

		if err := t.Do(ctx, opts); err != nil {
			log.Println(err.Error())
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(t.interval):
		}
	}
}
