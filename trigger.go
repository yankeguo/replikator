package replikator

import "sync"

// reconcileRequest is a coalesced set of replication triggers.
type reconcileRequest struct {
	all       bool
	forceAll  bool
	names     map[string]struct{}
	forceName map[string]struct{}
	deleted   map[string]struct{}
}

func (req reconcileRequest) empty() bool {
	return !req.all && len(req.names) == 0 && len(req.deleted) == 0
}

func (req reconcileRequest) forces(namespace string) bool {
	if req.forceAll {
		return true
	}
	_, ok := req.forceName[namespace]
	return ok
}

// triggerQueue merges watch and timer events so producers never block.
type triggerQueue struct {
	mu        sync.Mutex
	notify    chan struct{}
	all       bool
	forceAll  bool
	names     map[string]struct{}
	forceName map[string]struct{}
	deleted   map[string]struct{}
}

func newTriggerQueue() *triggerQueue {
	return &triggerQueue{notify: make(chan struct{}, 1)}
}

func (q *triggerQueue) signalLocked() {
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

// syncAll requests a reconciliation of every matching namespace.
// force bypasses the unchanged-object cache.
func (q *triggerQueue) syncAll(force bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.all = true
	if force {
		q.forceAll = true
	}
	q.signalLocked()
}

// syncNamespace requests a reconciliation of one namespace and bypasses the cache.
func (q *triggerQueue) syncNamespace(namespace string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.names == nil {
		q.names = map[string]struct{}{}
	}
	if q.forceName == nil {
		q.forceName = map[string]struct{}{}
	}
	q.names[namespace] = struct{}{}
	q.forceName[namespace] = struct{}{}
	delete(q.deleted, namespace)
	q.signalLocked()
}

// forgetNamespace drops cached state for a namespace that has been deleted.
func (q *triggerQueue) forgetNamespace(namespace string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.deleted == nil {
		q.deleted = map[string]struct{}{}
	}
	q.deleted[namespace] = struct{}{}
	delete(q.names, namespace)
	delete(q.forceName, namespace)
	q.signalLocked()
}

func (q *triggerQueue) drain() reconcileRequest {
	q.mu.Lock()
	defer q.mu.Unlock()
	req := reconcileRequest{
		all:       q.all,
		forceAll:  q.forceAll,
		names:     q.names,
		forceName: q.forceName,
		deleted:   q.deleted,
	}
	q.all = false
	q.forceAll = false
	q.names = nil
	q.forceName = nil
	q.deleted = nil
	return req
}
