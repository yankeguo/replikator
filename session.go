package replikator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// SessionList runs a set of sessions together.
type SessionList []*Session

// Run blocks until every session returns. Each session stops when ctx is cancelled.
func (list SessionList) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, session := range list {
		wg.Add(1)
		go func() {
			defer wg.Done()
			session.Run(ctx)
		}()
	}
	wg.Wait()
}

// Session replicates one task.
type Session struct {
	task               *Task
	client             kubernetes.Interface
	dynClient          dynamic.Interface
	log                *logrus.Entry
	versions           map[string]string
	fullSyncInterval   time.Duration
	watchRetryInterval time.Duration
}

// Run replicates until ctx is cancelled.
func (s *Session) Run(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	q := newTriggerQueue()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.watchLoop(ctx, q)
	}()

	interval := s.fullSyncInterval
	if interval <= 0 {
		interval = defaultFullSyncInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case <-q.notify:
		case <-ticker.C:
			q.syncAll(true)
			continue
		}

		req := q.drain()
		if req.empty() {
			continue
		}
		if err := s.reconcile(ctx, req); err != nil {
			if ctx.Err() != nil {
				wg.Wait()
				return
			}
			s.log.WithError(err).Error("task error")
		}
	}
}

func (s *Session) reconcile(ctx context.Context, req reconcileRequest) error {
	for namespace := range req.deleted {
		delete(s.versions, namespace)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	src, err := s.fetchResource(ctx)
	if apierrors.IsNotFound(err) {
		s.log.Info("source missing; deleting managed replicas")
		return s.deleteReplicas(ctx)
	}
	if err != nil {
		return fmt.Errorf("fetch source: %w", err)
	}

	fingerprint, err := sourceFingerprint(src, s.task.srcNamespace, s.task.srcName)
	if err != nil {
		return fmt.Errorf("fingerprint source: %w", err)
	}

	namespaces, err := s.destinationNamespaces(ctx, req)
	if err != nil {
		return err
	}

	var errs []error
	for _, namespace := range namespaces {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !req.forces(namespace) && s.versions[namespace] == fingerprint {
			s.log.WithField("dst", namespace+"/"+s.task.dstName).Debug("unchanged")
			continue
		}
		if err := s.replicateOne(ctx, src, namespace); err != nil {
			errs = append(errs, fmt.Errorf("%s/%s: %w", namespace, s.task.dstName, err))
			continue
		}
		s.versions[namespace] = fingerprint
	}
	return errors.Join(errs...)
}

func (s *Session) destinationNamespaces(ctx context.Context, req reconcileRequest) ([]string, error) {
	if req.all {
		return s.listDestinationNamespaces(ctx)
	}
	namespaces := make([]string, 0, len(req.names))
	for namespace := range req.names {
		if namespace == s.task.srcNamespace || !s.task.dstNamespace.MatchString(namespace) {
			continue
		}
		namespaces = append(namespaces, namespace)
	}
	sort.Strings(namespaces)
	return namespaces, nil
}

func (s *Session) listDestinationNamespaces(ctx context.Context) ([]string, error) {
	list, err := s.client.CoreV1().Namespaces().List(ctx, metaV1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}
	namespaces := make([]string, 0, len(list.Items))
	for _, namespace := range list.Items {
		if namespace.Name == s.task.srcNamespace {
			continue
		}
		if namespace.DeletionTimestamp != nil || namespace.Status.Phase == "Terminating" {
			continue
		}
		if s.task.dstNamespace.MatchString(namespace.Name) {
			namespaces = append(namespaces, namespace.Name)
		}
	}
	sort.Strings(namespaces)
	return namespaces, nil
}

func (s *Session) fetchResource(ctx context.Context) (*unstructured.Unstructured, error) {
	return s.dynClient.Resource(s.task.resource).Namespace(s.task.srcNamespace).Get(ctx, s.task.srcName, metaV1.GetOptions{})
}

func (s *Session) replicateOne(ctx context.Context, src *unstructured.Unstructured, namespace string) error {
	obj, err := s.createReplicatedResource(src, namespace)
	if err != nil {
		return err
	}
	log := s.log.WithField("dst", namespace+"/"+s.task.dstName).WithField("resourceVersion", src.GetResourceVersion())
	log.Info("replicating")
	_, err = s.dynClient.Resource(s.task.resource).Namespace(namespace).Apply(ctx, s.task.dstName, obj, metaV1.ApplyOptions{
		Force:        true,
		FieldManager: FieldManagerReplikator,
	})
	if err != nil {
		log.WithError(err).Error("replication failed")
		return err
	}
	return nil
}

func (s *Session) createReplicatedResource(src *unstructured.Unstructured, namespace string) (*unstructured.Unstructured, error) {
	obj := src.DeepCopy()
	var err error
	obj, err = applyJSONPatch(obj, s.task.jsonpatch)
	if err != nil {
		return nil, fmt.Errorf("jsonpatch: %w", err)
	}
	if s.task.javascript != "" {
		obj, err = applyJavaScript(obj, s.task.javascript)
		if err != nil {
			return nil, fmt.Errorf("javascript: %w", err)
		}
	}
	sanitizeForApply(obj, namespace, s.task.dstName)
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[AnnotationManaged] = "true"
	annotations[AnnotationSourceNamespace] = s.task.srcNamespace
	annotations[AnnotationSourceName] = s.task.srcName
	obj.SetAnnotations(annotations)
	return obj, nil
}

func (s *Session) deleteReplicas(ctx context.Context) error {
	namespaces, err := s.listDestinationNamespaces(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, namespace := range namespaces {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.deleteReplica(ctx, namespace); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		s.versions = map[string]string{}
	}
	return errors.Join(errs...)
}

func (s *Session) deleteReplica(ctx context.Context, namespace string) error {
	client := s.dynClient.Resource(s.task.resource).Namespace(namespace)
	obj, err := client.Get(ctx, s.task.dstName, metaV1.GetOptions{})
	if apierrors.IsNotFound(err) {
		delete(s.versions, namespace)
		return nil
	}
	if err != nil {
		return fmt.Errorf("get %s/%s: %w", namespace, s.task.dstName, err)
	}
	if !ownedByTask(obj, s.task) {
		return nil
	}
	if err := client.Delete(ctx, s.task.dstName, metaV1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete %s/%s: %w", namespace, s.task.dstName, err)
	}
	delete(s.versions, namespace)
	s.log.WithField("dst", namespace+"/"+s.task.dstName).Info("deleted replica")
	return nil
}

func ownedByTask(obj *unstructured.Unstructured, task *Task) bool {
	annotations := obj.GetAnnotations()
	return annotations[AnnotationManaged] == "true" &&
		annotations[AnnotationSourceNamespace] == task.srcNamespace &&
		annotations[AnnotationSourceName] == task.srcName
}

// sanitizeForApply drops status and identity metadata that must not be copied,
// then pins the replica name and namespace. Modifications run before this so
// patches can still see fields such as status.
func sanitizeForApply(obj *unstructured.Unstructured, namespace, name string) {
	unstructured.RemoveNestedField(obj.Object, "status")
	labels := obj.GetLabels()
	annotations := obj.GetAnnotations()
	obj.Object["metadata"] = map[string]any{}
	obj.SetName(name)
	obj.SetNamespace(namespace)
	if len(labels) > 0 {
		obj.SetLabels(labels)
	}
	if len(annotations) > 0 {
		obj.SetAnnotations(annotations)
	}
}

func sourceFingerprint(src *unstructured.Unstructured, namespace, name string) (string, error) {
	obj := src.DeepCopy()
	sanitizeForApply(obj, namespace, name)
	buf, err := obj.MarshalJSON()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:]), nil
}
