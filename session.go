package replikator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/yankeguo/rg"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const FieldManagerReplikator = "io.github.yankeguo/replikator"

type SessionList []*Session

func (list SessionList) Run(ctx context.Context) {
	wg := &sync.WaitGroup{}
	for _, _session := range list {
		session := _session
		wg.Add(1)
		go func() {
			defer wg.Done()
			session.Run(ctx)
		}()
	}
	wg.Wait()
}

type Session struct {
	task      *Task
	client    *kubernetes.Clientset
	dynClient *dynamic.DynamicClient
	log       *logrus.Entry
	versions  map[string]string
}

func (s *Session) listDestinationNamespaces(ctx context.Context) (namespaces []string, err error) {
	defer rg.Guard(&err)

	for _, namespace := range rg.Must(s.client.CoreV1().Namespaces().List(ctx, metaV1.ListOptions{})).Items {
		// skip source namespace
		if namespace.Name == s.task.srcNamespace {
			continue
		}
		if s.task.dstNamespace.MatchString(namespace.Name) {
			namespaces = append(namespaces, namespace.Name)
		}
	}
	return
}

func (s *Session) fetchResource(ctx context.Context) (src *unstructured.Unstructured, rv string, err error) {
	defer rg.Guard(&err)

	src = rg.Must(s.dynClient.Resource(s.task.resource).Namespace(s.task.srcNamespace).Get(ctx, s.task.srcName, metaV1.GetOptions{}))

	rv = src.GetResourceVersion()

	delete(src.Object, "status")
	if metadata, ok := src.Object["metadata"].(map[string]interface{}); ok {
		src.Object["metadata"] = map[string]interface{}{
			"name":        metadata["name"],
			"namespace":   metadata["namespace"],
			"labels":      metadata["labels"],
			"annotations": metadata["annotations"],
		}
	}

	return
}

func (s *Session) createReplicatedResource(source *unstructured.Unstructured, namespace string) (obj *unstructured.Unstructured, err error) {
	defer rg.Guard(&err)

	obj = source.DeepCopy()
	obj.SetNamespace(namespace)
	obj.SetName(s.task.dstName)

	// apply jsonpatch
	if s.task.jsonpatch != nil {
		buf := rg.Must(obj.MarshalJSON())
		buf = rg.Must(s.task.jsonpatch.Apply(buf))
		obj = &unstructured.Unstructured{}
		rg.Must0(obj.UnmarshalJSON(buf))
	}

	// apply javascript
	if s.task.javascript != "" {
		buf := rg.Must(obj.MarshalJSON())
		out := rg.Must(EvaluateJavaScriptModification(string(buf), s.task.javascript))
		obj = &unstructured.Unstructured{}
		rg.Must0(obj.UnmarshalJSON([]byte(out)))
	}

	return
}

func (s *Session) Do(ctx context.Context, namespace string) (err error) {
	defer rg.Guard(&err)

	var namespaces []string

	if namespace == "" {
		namespaces = rg.Must(s.listDestinationNamespaces(ctx))
	} else {
		namespaces = []string{namespace}
	}

	src, rv := rg.Must2(s.fetchResource(ctx))

	for _, namespace := range namespaces {
		if s.versions[namespace] == rv {
			continue
		}

		log := s.log.WithField("dst", namespace+"/"+s.task.dstName)

		obj := rg.Must(s.createReplicatedResource(src, namespace))

		log.Info("replicating")

		if _, err = s.dynClient.Resource(s.task.resource).Namespace(namespace).Apply(ctx, s.task.dstName, obj, metaV1.ApplyOptions{
			Force:        true,
			FieldManager: FieldManagerReplikator,
		}); err != nil {
			log.WithError(err).Error("replication failed")
			err = nil
		} else {
			s.versions[namespace] = rv
		}
	}

	return
}

func (s *Session) Watch(ctx context.Context, triggers chan string) {
	var err error
	for {
		if ctx.Err() != nil {
			return
		}

		if err = s.watch(ctx, triggers); err != nil {
			if ctx.Err() == nil {
				s.log.WithError(err).Error("watch error")
			}
		}

		time.Sleep(5 * time.Second)
	}
}

func (s *Session) watch(ctx context.Context, triggers chan string) (err error) {
	defer rg.Guard(&err)

	if ctx.Err() != nil {
		return
	}

	// cancel context when done
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// trigger initial synchronization
	triggers <- ""

	wg := &sync.WaitGroup{}

	// watch for resource changes
	watchResource := rg.Must(s.dynClient.Resource(s.task.resource).Namespace(s.task.srcNamespace).Watch(ctx, metaV1.ListOptions{}))
	defer watchResource.Stop()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for event := range watchResource.ResultChan() {
			if err = func() (err error) {
				defer rg.Guard(&err)
				switch event.Type {
				case watch.Modified:
					if name := rg.Must(RetrieveMetadataName(event.Object)); name == s.task.srcName {
						triggers <- ""
					}
				case watch.Error:
					err = fmt.Errorf("watch error: %+v", event.Object)
					return
				}
				return
			}(); err != nil {
				cancel()
				return
			}
		}
	}()

	// watch for namespace changes
	watchNamespace := rg.Must(s.client.CoreV1().Namespaces().Watch(ctx, metaV1.ListOptions{}))
	defer watchNamespace.Stop()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for event := range watchNamespace.ResultChan() {
			if err = func() (err error) {
				defer rg.Guard(&err)
				switch event.Type {
				case watch.Added:
					if name := rg.Must(RetrieveMetadataName(event.Object)); s.task.dstNamespace.MatchString(name) {
						triggers <- name
					}
				case watch.Error:
					err = fmt.Errorf("watch error: %+v", event.Object)
					return
				}
				return
			}(); err != nil {
				cancel()
				return
			}
		}
	}()

	<-ctx.Done()

	wg.Wait()

	return
}

// Run the task until context is done
func (s *Session) Run(ctx context.Context) {
	triggers := make(chan string, 1)
	defer close(triggers)

	if ctx.Err() != nil {
		return
	}

	go s.Watch(ctx, triggers)

	for {
		var namespace string

		select {
		case <-ctx.Done():
			return
		case namespace = <-triggers:
		case <-time.After(10 * time.Minute):
			namespace = ""
		}

		if err := s.Do(ctx, namespace); err != nil {
			s.log.WithError(err).Error("task error")
		}
	}
}
