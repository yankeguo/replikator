package replikator

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/yankeguo/rg"
	"gopkg.in/yaml.v3"
)

type TaskDefinitionList []TaskDefinition

func (defs TaskDefinitionList) Build() (tasks TaskList, err error) {
	for _, def := range defs {
		var task *Task
		if task, err = def.Build(); err != nil {
			return
		}
		tasks = append(tasks, task)
	}
	return
}

// TaskDefinition is the definition of a Task
type TaskDefinition struct {
	Resource string `yaml:"resource"`
	Source   struct {
		Namespace string `yaml:"namespace"`
		Name      string `yaml:"name"`
	} `yaml:"source"`
	Target struct {
		Namespace string `yaml:"namespace"`
		Name      string `yaml:"name"`
	} `yaml:"target"`
	Modification struct {
		JSONPatch  []any  `yaml:"jsonpatch"`
		Javascript string `yaml:"javascript"`
	} `yaml:"modification"`
}

// Build creates a Task from TaskDefinition
func (def TaskDefinition) Build() (out *Task, err error) {
	out = &Task{}

	// resource
	if def.Resource == "" {
		err = errors.New("resource is required")
		return
	}
	if out.resource, err = ParseGroupVersionResource(def.Resource); err != nil {
		return
	}

	// srcNamespace
	if def.Source.Namespace == "" {
		buf, _ := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
		if len(buf) > 0 {
			def.Source.Namespace = string(bytes.TrimSpace(buf))
		} else {
			err = errors.New("source.namespace is required")
			return
		}
	}
	out.srcNamespace = def.Source.Namespace

	// srcName
	if def.Source.Name == "" {
		err = errors.New("source.name is required")
		return
	}
	out.srcName = def.Source.Name

	// dstNamespace
	if def.Target.Namespace == "" {
		err = errors.New("target.namespace is required")
		return
	}
	if out.dstNamespace, err = regexp.Compile(def.Target.Namespace); err != nil {
		return
	}

	// dstName
	if def.Target.Name == "" {
		def.Target.Name = def.Source.Name
	}
	out.dstName = def.Target.Name

	// jsonpatch
	if len(def.Modification.JSONPatch) > 0 {
		var buf []byte
		if buf, err = json.Marshal(def.Modification.JSONPatch); err != nil {
			return
		}
		if out.jsonpatch, err = jsonpatch.DecodePatch(buf); err != nil {
			return
		}
	}

	// javascript
	out.javascript = strings.TrimSpace(def.Modification.Javascript)

	return
}

// LoadTaskDefinitionsFromFile loads TaskDefinition from file
func LoadTaskDefinitionsFromFile(file string) (defs TaskDefinitionList, err error) {
	defer rg.Guard(&err)

	buf := rg.Must(os.ReadFile(file))

	dec := yaml.NewDecoder(bytes.NewReader(buf))

	for {
		var def TaskDefinition

		if err = dec.Decode(&def); err != nil {
			if errors.Is(err, io.EOF) {
				err = nil
				break
			} else {
				return
			}
		}

		defs = append(defs, def)
	}

	return
}

// LoadTaskDefinitionsFromDir loads TaskDefinitions from dir
func LoadTaskDefinitionsFromDir(dir string) (defs TaskDefinitionList, err error) {
	defer rg.Guard(&err)

	for _, entry := range rg.Must(os.ReadDir(dir)) {
		if entry.IsDir() {
			continue
		}
		if (!strings.HasSuffix(entry.Name(), ".yaml")) && (!strings.HasSuffix(entry.Name(), ".yml")) {
			continue
		}

		defs = append(defs, rg.Must(LoadTaskDefinitionsFromFile(filepath.Join(dir, entry.Name())))...)
	}

	return
}

// DigestTaskDefinitionsFromDir creates digest for TaskDefinitions in dir, for change detection
func DigestTaskDefinitionsFromDir(dir string) (digest string, err error) {
	defer rg.Guard(&err)

	var files []string

	for _, entry := range rg.Must(os.ReadDir(dir)) {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}

	sort.Strings(files)

	h := md5.New()

	for _, file := range files {
		rg.Must(h.Write(rg.Must(os.ReadFile(file))))
	}

	digest = hex.EncodeToString(h.Sum(nil))

	return
}
