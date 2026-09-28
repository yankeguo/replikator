package replikator

import (
	"fmt"
	"regexp"
	"strconv"
	"sync/atomic"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

var sessionCounter int64

// TaskList is an ordered set of replication tasks.
type TaskList []*Task

// NewSessions builds one session per task. Sessions share the clients in opts.
func (list TaskList) NewSessions(opts TaskOptions) SessionList {
	out := make(SessionList, 0, len(list))
	for _, task := range list {
		out = append(out, task.NewSession(opts))
	}
	return out
}

// Task is a compiled replication rule.
type Task struct {
	resource     schema.GroupVersionResource
	srcNamespace string
	srcName      string
	dstNamespace *regexp.Regexp
	dstName      string

	javascript string
	jsonpatch  jsonpatch.Patch
}

// String returns a stable identity used in logs and duplicate detection.
func (t *Task) String() string {
	resource := t.resource.Resource
	switch {
	case t.resource.Group != "":
		resource = t.resource.Group + "/" + t.resource.Version + "/" + t.resource.Resource
	case t.resource.Version != "" && t.resource.Version != "v1":
		resource = t.resource.Version + "/" + t.resource.Resource
	}
	pattern := ""
	if t.dstNamespace != nil {
		pattern = t.dstNamespace.String()
	}
	return fmt.Sprintf("%s %s/%s -> %s/%s", resource, t.srcNamespace, t.srcName, pattern, t.dstName)
}

// TaskOptions supplies API clients for a session.
type TaskOptions struct {
	Client        kubernetes.Interface
	DynamicClient dynamic.Interface
}

// NewSession creates a session that replicates this task until its context is cancelled.
func (t *Task) NewSession(opts TaskOptions) *Session {
	session := strconv.FormatInt(atomic.AddInt64(&sessionCounter, 1), 10)
	return &Session{
		task:               t,
		client:             opts.Client,
		dynClient:          opts.DynamicClient,
		fullSyncInterval:   defaultFullSyncInterval,
		watchRetryInterval: defaultWatchRetryInterval,
		log: logrus.WithField("res", t.resource.String()).
			WithField("src", t.srcNamespace+"/"+t.srcName).
			WithField("dst", t.dstNamespace.String()+"/"+t.dstName).
			WithField("session", session),
		versions: map[string]string{},
	}
}
