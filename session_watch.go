package replikator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
)

var errWatchClosed = errors.New("watch closed")

func (s *Session) watchLoop(ctx context.Context, q *triggerQueue) {
	for {
		if ctx.Err() != nil {
			return
		}
		err := s.watchOnce(ctx, q)
		if ctx.Err() != nil {
			return
		}
		delay := s.watchRetryInterval
		if delay <= 0 {
			delay = defaultWatchRetryInterval
		}
		if err != nil {
			if errors.Is(err, errWatchClosed) {
				s.log.Debug("watch closed; restarting")
				delay = watchRestartInterval
			} else {
				s.log.WithError(err).Error("watch error")
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

func (s *Session) watchOnce(ctx context.Context, q *triggerQueue) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	resourceWatch, err := s.dynClient.Resource(s.task.resource).Namespace(s.task.srcNamespace).Watch(ctx, metaV1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("metadata.name", s.task.srcName).String(),
	})
	if err != nil {
		return fmt.Errorf("watch source: %w", err)
	}
	defer resourceWatch.Stop()

	namespaceWatch, err := s.client.CoreV1().Namespaces().Watch(ctx, metaV1.ListOptions{})
	if err != nil {
		return fmt.Errorf("watch namespaces: %w", err)
	}
	defer namespaceWatch.Stop()

	// Watch is established before the first sync so events during the list are not missed.
	q.syncAll(true)

	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := s.consumeResourceWatch(ctx, resourceWatch, q); err != nil {
			errCh <- err
			cancel()
		}
	}()
	go func() {
		defer wg.Done()
		if err := s.consumeNamespaceWatch(ctx, namespaceWatch, q); err != nil {
			errCh <- err
			cancel()
		}
	}()
	wg.Wait()

	select {
	case err := <-errCh:
		return err
	default:
	}
	if ctx.Err() != nil {
		return nil
	}
	return errWatchClosed
}

func (s *Session) consumeResourceWatch(ctx context.Context, watcher watch.Interface, q *triggerQueue) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return errWatchClosed
			}
			if err := s.handleResourceEvent(event, q); err != nil {
				return err
			}
		}
	}
}

func (s *Session) consumeNamespaceWatch(ctx context.Context, watcher watch.Interface, q *triggerQueue) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return errWatchClosed
			}
			if err := s.handleNamespaceEvent(event, q); err != nil {
				return err
			}
		}
	}
}

func (s *Session) handleResourceEvent(event watch.Event, q *triggerQueue) error {
	switch event.Type {
	case watch.Added, watch.Modified:
		if s.eventName(event) != s.task.srcName {
			return nil
		}
		q.syncAll(false)
	case watch.Deleted:
		if s.eventName(event) != s.task.srcName {
			return nil
		}
		q.syncAll(true)
	case watch.Bookmark:
		return nil
	case watch.Error:
		return watchEventError(event)
	}
	return nil
}

func (s *Session) handleNamespaceEvent(event watch.Event, q *triggerQueue) error {
	switch event.Type {
	case watch.Added:
		name := s.eventName(event)
		if name == "" || name == s.task.srcNamespace {
			return nil
		}
		if !s.task.dstNamespace.MatchString(name) || objectTerminating(event.Object) {
			return nil
		}
		q.syncNamespace(name)
	case watch.Deleted:
		name := s.eventName(event)
		if name == "" {
			return nil
		}
		q.forgetNamespace(name)
	case watch.Bookmark:
		return nil
	case watch.Error:
		return watchEventError(event)
	}
	return nil
}

func (s *Session) eventName(event watch.Event) string {
	name, err := RetrieveMetadataName(event.Object)
	if err != nil {
		s.log.WithError(err).Warn("ignoring watch event without a name")
		return ""
	}
	return name
}

func objectTerminating(obj any) bool {
	ro, ok := obj.(runtime.Object)
	if !ok {
		return false
	}
	accessor, err := meta.Accessor(ro)
	if err != nil || accessor.GetDeletionTimestamp() == nil {
		return false
	}
	return true
}

func watchEventError(event watch.Event) error {
	if status, ok := event.Object.(*metaV1.Status); ok {
		return fmt.Errorf("watch failed: %s: %s", status.Reason, status.Message)
	}
	return fmt.Errorf("watch failed: %v", event.Object)
}
