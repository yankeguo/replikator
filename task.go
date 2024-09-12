package replikator

import (
	"bytes"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/yankeguo/rg"
	"gopkg.in/yaml.v3"
)

type Task struct {
	Interval    time.Duration `yaml:"-"`
	RawInterval string        `yaml:"interval"`

	ResourceVersion string `yaml:"resource_version"`
	Resource        string `yaml:"resource"`

	Source struct {
		Namespace string `yaml:"namespace"`
		Name      string `yaml:"name"`
	} `yaml:"source"`

	Target struct {
		Namespace    *regexp.Regexp `yaml:"-"`
		RawNamespace string         `yaml:"namespace"`
		Name         string         `yaml:"name"`
	} `yaml:"target"`
}

func sanitizeTask(task *Task) (err error) {
	if task.RawInterval == "" {
		task.RawInterval = "1m"
	}

	if task.Interval, err = time.ParseDuration(task.RawInterval); err != nil {
		return
	}

	if task.ResourceVersion == "" {
		task.ResourceVersion = "v1"
	}

	if task.Resource == "" {
		err = errors.New("resource is required")
		return
	}

	if task.Source.Namespace == "" {
		buf, _ := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
		if len(buf) > 0 {
			task.Source.Namespace = string(buf)
		} else {
			err = errors.New("source.namespace is required")
			return
		}
	}

	if task.Source.Name == "" {
		err = errors.New("source.name is required")
		return
	}

	if task.Target.RawNamespace == "" {
		err = errors.New("target.namespace is required")
		return
	}

	if task.Target.Namespace, err = regexp.Compile(task.Target.RawNamespace); err != nil {
		return
	}

	if task.Target.Name == "" {
		task.Target.Name = task.Source.Name
	}
	return
}

func LoadTasks(dir string) (tasks []Task, err error) {
	defer rg.Guard(&err)

	for _, entry := range rg.Must(os.ReadDir(dir)) {
		if entry.IsDir() {
			continue
		}
		if (!strings.HasSuffix(entry.Name(), ".yaml")) && (!strings.HasSuffix(entry.Name(), ".yml")) {
			continue
		}

		log.Println("loading tasks from:", entry.Name())

		buf := rg.Must(os.ReadFile(filepath.Join(dir, entry.Name())))

		dec := yaml.NewDecoder(bytes.NewReader(buf))

		for {
			var task Task

			if err = dec.Decode(&task); err != nil {
				if errors.Is(err, io.EOF) {
					err = nil
					break
				} else {
					return
				}
			}

			rg.Must0(sanitizeTask(&task))

			tasks = append(tasks, task)
		}
	}

	return
}
