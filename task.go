package replikator

import (
	"regexp"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/yankeguo/rg"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type TaskList []*Task

func (list TaskList) NewSessions(opts TaskOptions) (out SessionList) {
	for _, task := range list {
		out = append(out, task.NewSession(opts))
	}
	return
}

type Task struct {
	resource     schema.GroupVersionResource
	srcNamespace string
	srcName      string
	dstNamespace *regexp.Regexp
	dstName      string

	javascript string
	jsonpatch  jsonpatch.Patch
}

// TaskOptions is the options for creating a new session
type TaskOptions struct {
	Client        *kubernetes.Clientset
	DynamicClient *dynamic.DynamicClient
}

// NewSession creates a new session for the task with kubernetes client and dynamic client
func (t *Task) NewSession(opts TaskOptions) *Session {
	return &Session{
		task:      t,
		client:    opts.Client,
		dynClient: opts.DynamicClient,
		log: logrus.WithField("res", t.resource.String()).
			WithField("src", t.srcNamespace+"/"+t.srcName).
			WithField("dst", t.dstNamespace.String()+"/"+t.dstName).
			WithField("session", rg.Must(uuid.NewV7()).String()),
		versions: map[string]string{},
	}
}
