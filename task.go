package replikator

import (
	"context"
	"regexp"
	"time"

	jsonpatch "github.com/evanphx/json-patch"
	log "github.com/sirupsen/logrus"
	"github.com/yankeguo/rg"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func (t *Task) Do(ctx context.Context, opts TaskOptions) (err error) {
	defer rg.Guard(&err)

	var targetNamespaces []string

	for _, namespace := range rg.Must(opts.Client.CoreV1().Namespaces().List(ctx, metaV1.ListOptions{})).Items {
		if namespace.Name == t.srcNamespace {
			continue
		}
		if t.dstNamespace.MatchString(namespace.Name) {
			targetNamespaces = append(targetNamespaces, namespace.Name)
		}
	}

	log := log.WithField("res", t.resource.String()).
		WithField(
			"src", t.srcNamespace+"/"+t.srcName,
		).
		WithField(
			"dst", t.dstNamespace.String()+"/"+t.dstName,
		)

	log.Info("task started")

	item := rg.Must(opts.DynamicClient.Resource(t.resource).Namespace(t.srcNamespace).Get(ctx, t.srcName, metaV1.GetOptions{}))

	// sanitize item
	delete(item.Object, "status")
	if metadata, ok := item.Object["metadata"].(map[string]interface{}); ok {
		item.Object["metadata"] = map[string]interface{}{
			"name":        metadata["name"],
			"namespace":   metadata["namespace"],
			"labels":      metadata["labels"],
			"annotations": metadata["annotations"],
		}
	}

	for _, namespace := range targetNamespaces {
		log := log.WithField("dst", namespace+"/"+t.dstName)

		item := item.DeepCopy()

		if metadata, ok := item.Object["metadata"].(map[string]interface{}); ok {
			metadata["namespace"] = namespace
			metadata["name"] = t.dstName
		}

		log.Info("replicating")

		if _, err = opts.DynamicClient.Resource(t.resource).Namespace(namespace).Apply(ctx, t.dstName, item, metaV1.ApplyOptions{
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
