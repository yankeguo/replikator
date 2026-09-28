package replikator

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/util/validation"
)

const serviceAccountNamespacePath = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

// readInClusterNamespace is replaced in tests.
var readInClusterNamespace = func() (string, error) {
	buf, err := os.ReadFile(serviceAccountNamespacePath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(buf)), nil
}

// TaskDefinitionList is a set of task documents loaded from configuration files.
type TaskDefinitionList []TaskDefinition

// Build compiles every definition. Duplicate replication rules are rejected.
func (defs TaskDefinitionList) Build() (TaskList, error) {
	tasks := make(TaskList, 0, len(defs))
	seen := make(map[string]struct{}, len(defs))
	for _, def := range defs {
		task, err := def.Build()
		if err != nil {
			return nil, err
		}
		id := task.String()
		if _, ok := seen[id]; ok {
			if def.origin != "" {
				return nil, fmt.Errorf("%s: duplicate task %s", def.origin, id)
			}
			return nil, fmt.Errorf("duplicate task %s", id)
		}
		seen[id] = struct{}{}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

// TaskDefinition is one YAML document describing a replication rule.
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

	origin string
}

// Build compiles the definition into a Task.
func (def TaskDefinition) Build() (*Task, error) {
	task, err := def.build()
	if err != nil && def.origin != "" {
		return nil, fmt.Errorf("%s: %w", def.origin, err)
	}
	return task, err
}

func (def TaskDefinition) build() (*Task, error) {
	out := &Task{}

	def.Resource = strings.TrimSpace(def.Resource)
	def.Source.Namespace = strings.TrimSpace(def.Source.Namespace)
	def.Source.Name = strings.TrimSpace(def.Source.Name)
	def.Target.Namespace = strings.TrimSpace(def.Target.Namespace)
	def.Target.Name = strings.TrimSpace(def.Target.Name)

	if def.Resource == "" {
		return nil, errors.New("resource is required")
	}
	resource, err := ParseGroupVersionResource(def.Resource)
	if err != nil {
		return nil, err
	}
	out.resource = resource

	if def.Source.Namespace == "" {
		ns, readErr := readInClusterNamespace()
		ns = strings.TrimSpace(ns)
		if readErr != nil || ns == "" {
			return nil, errors.New("source.namespace is required")
		}
		def.Source.Namespace = ns
	}
	if err := validateDNS1123Label("source.namespace", def.Source.Namespace); err != nil {
		return nil, err
	}
	out.srcNamespace = def.Source.Namespace

	if def.Source.Name == "" {
		return nil, errors.New("source.name is required")
	}
	if err := validateDNS1123Subdomain("source.name", def.Source.Name); err != nil {
		return nil, err
	}
	out.srcName = def.Source.Name

	if def.Target.Namespace == "" {
		return nil, errors.New("target.namespace is required")
	}
	pattern, err := regexp.Compile(def.Target.Namespace)
	if err != nil {
		return nil, fmt.Errorf("target.namespace: %w", err)
	}
	out.dstNamespace = pattern

	if def.Target.Name == "" {
		def.Target.Name = def.Source.Name
	}
	if err := validateDNS1123Subdomain("target.name", def.Target.Name); err != nil {
		return nil, err
	}
	out.dstName = def.Target.Name

	if len(def.Modification.JSONPatch) > 0 {
		buf, err := json.Marshal(def.Modification.JSONPatch)
		if err != nil {
			return nil, fmt.Errorf("modification.jsonpatch: %w", err)
		}
		patch, err := jsonpatch.DecodePatch(buf)
		if err != nil {
			return nil, fmt.Errorf("modification.jsonpatch: %w", err)
		}
		out.jsonpatch = patch
	}
	out.javascript = strings.TrimSpace(def.Modification.Javascript)
	return out, nil
}

func validateDNS1123Label(field, value string) error {
	if errs := validation.IsDNS1123Label(value); len(errs) > 0 {
		return fmt.Errorf("%s %q: %s", field, value, errs[0])
	}
	return nil
}

func validateDNS1123Subdomain(field, value string) error {
	if errs := validation.IsDNS1123Subdomain(value); len(errs) > 0 {
		return fmt.Errorf("%s %q: %s", field, value, errs[0])
	}
	return nil
}

// LoadTaskDefinitionsFromFile loads every YAML document in file.
// Empty documents are skipped. Later documents keep their 1-based index in error messages.
func LoadTaskDefinitionsFromFile(file string) (TaskDefinitionList, error) {
	buf, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", file, err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(buf))
	var defs TaskDefinitionList
	for i := 1; ; i++ {
		var node yaml.Node
		err := dec.Decode(&node)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode %s document %d: %w", file, i, err)
		}
		if isEmptyYAMLDocument(&node) {
			continue
		}

		var def TaskDefinition
		if err := node.Decode(&def); err != nil {
			return nil, fmt.Errorf("decode %s document %d: %w", file, i, err)
		}
		def.origin = fmt.Sprintf("%s document %d", file, i)
		defs = append(defs, def)
	}
	return defs, nil
}

func isEmptyYAMLDocument(node *yaml.Node) bool {
	n := node
	if n.Kind == yaml.DocumentNode {
		if len(n.Content) == 0 {
			return true
		}
		if len(n.Content) == 1 {
			n = n.Content[0]
		}
	}
	return n.Tag == "!!null" || n.Kind == 0
}

// LoadTaskDefinitionsFromDir loads YAML files from dir in lexical order.
// Subdirectories are ignored.
func LoadTaskDefinitionsFromDir(dir string) (TaskDefinitionList, error) {
	files, err := listTaskDefinitionFiles(dir)
	if err != nil {
		return nil, err
	}
	var defs TaskDefinitionList
	for _, file := range files {
		fileDefs, err := LoadTaskDefinitionsFromFile(file)
		if err != nil {
			return nil, err
		}
		defs = append(defs, fileDefs...)
	}
	return defs, nil
}

// LoadTasks loads and compiles every task definition in dir.
func LoadTasks(dir string) (TaskList, error) {
	defs, err := LoadTaskDefinitionsFromDir(dir)
	if err != nil {
		return nil, err
	}
	return defs.Build()
}

// DigestTaskDefinitionsFromDir returns a content digest of the YAML files in dir.
// The digest changes when a file name, length, or body changes.
func DigestTaskDefinitionsFromDir(dir string) (string, error) {
	files, err := listTaskDefinitionFiles(dir)
	if err != nil {
		return "", err
	}

	hash := sha256.New()
	for _, file := range files {
		buf, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", file, err)
		}
		rel, err := filepath.Rel(dir, file)
		if err != nil {
			rel = file
		}
		fmt.Fprintf(hash, "%s\n%d\n", filepath.ToSlash(rel), len(buf))
		hash.Write(buf)
		hash.Write([]byte{'\n'})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func listTaskDefinitionFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read config dir %s: %w", dir, err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		files = append(files, filepath.Join(dir, name))
	}
	sort.Strings(files)
	return files, nil
}
