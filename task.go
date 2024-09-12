package replikator

import (
	"bytes"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yankeguo/rg"
	"gopkg.in/yaml.v2"
)

type Task struct {
	Interval string `yaml:"interval"`
	From     struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
		Name       string `yaml:"name"`
		Namespace  string `yaml:"namespace"`
	} `yaml:"from"`
	To struct {
		Namespace string `yaml:"namespace"`
		Name      string `yaml:"name"`
	} `yaml:"to"`
}

func sanitizeTask(task *Task) (err error) {
	if task.Interval == "" {
		task.Interval = "1m"
	}
	if _, err = time.ParseDuration(task.Interval); err != nil {
		return
	}
	if task.From.APIVersion == "" {
		task.From.APIVersion = "v1"
	}
	if task.From.Kind == "" {
		err = errors.New("from.kind is required")
		return
	}
	if task.From.Namespace == "" {
		buf, _ := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
		if len(buf) > 0 {
			task.From.Namespace = string(buf)
		} else {
			err = errors.New("from.namespace is required")
			return
		}
	}
	if task.From.Name == "" {
		err = errors.New("from.name is required")
		return
	}
	if task.To.Namespace == "" {
		err = errors.New("to.namespace is required")
		return
	}
	if task.To.Name == "" {
		task.To.Name = task.From.Name
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

		log.Println("loading task from", entry.Name())

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
