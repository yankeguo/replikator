package replikator

import (
	"context"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/yankeguo/rg"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const FieldManagerReplikator = "io.github.yankeguo/replikator"

type ReplikatorMetadata struct {
	Name        string             `json:"name"`
	Namespace   string             `json:"namespace"`
	Labels      *map[string]string `json:"labels"`
	Annotations *map[string]string `json:"annotations"`
}

type RunOptions struct {
	WaitGroup *sync.WaitGroup
	Task      Task
	Client    *kubernetes.Clientset
	DynClient *dynamic.DynamicClient
}

func runOnce(ctx context.Context, opts RunOptions) (err error) {
	defer rg.Guard(&err)

	var targetNamespaces []string

	for _, namespace := range rg.Must(opts.Client.CoreV1().Namespaces().List(ctx, metaV1.ListOptions{})).Items {
		if namespace.Name == opts.Task.Source.Namespace {
			continue
		}
		if opts.Task.Target.Namespace.MatchString(namespace.Name) {
			targetNamespaces = append(targetNamespaces, namespace.Name)
		}
	}

	log := log.WithField("resource", opts.Task.Resource).WithField("source", opts.Task.Source.Namespace+"/"+opts.Task.Source.Name)

	log.Info("task started")

	res := rg.Must(schema.ParseGroupVersion(opts.Task.ResourceVersion)).WithResource(opts.Task.Resource)

	item := rg.Must(opts.DynClient.Resource(res).Namespace(opts.Task.Source.Namespace).Get(ctx, opts.Task.Source.Name, metaV1.GetOptions{}))

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
		log := log.WithField("namespace", namespace)

		item := item.DeepCopy()

		if metadata, ok := item.Object["metadata"].(map[string]interface{}); ok {
			metadata["namespace"] = namespace
			metadata["name"] = opts.Task.Target.Name
		}

		log.Info("replicating")

		if _, err = opts.DynClient.Resource(res).Namespace(namespace).Apply(ctx, opts.Task.Target.Name, item, metaV1.ApplyOptions{
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

func Run(ctx context.Context, opts RunOptions) {
	defer opts.WaitGroup.Done()

	for {
		if ctx.Err() != nil {
			return
		}

		if err := runOnce(ctx, opts); err != nil {
			log.Println(err.Error())
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(opts.Task.Interval):
		}
	}
}
