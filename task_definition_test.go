package replikator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestMain(m *testing.M) {
	readInClusterNamespace = func() (string, error) {
		return "", os.ErrNotExist
	}
	os.Exit(m.Run())
}

func TestTaskDefBuild(t *testing.T) {
	def := TaskDefinition{}

	def.Resource = "apps/deployments"
	_, err := def.Build()
	require.Error(t, err)

	def.Source.Namespace = "auto-ops"
	_, err = def.Build()
	require.Error(t, err)

	def.Source.Name = "default-registry"
	_, err = def.Build()
	require.Error(t, err)

	def.Target.Namespace = ".+"
	def.Target.Name = "custom-registry"
	def.Modification.Javascript = "var a = 0;"
	def.Modification.JSONPatch = []any{
		map[string]any{
			"op":   "remove",
			"path": "/status",
		},
	}
	tsk, err := def.Build()
	require.NoError(t, err)
	require.Equal(t, schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "deployments",
	}, tsk.resource)
	require.Equal(t, "auto-ops", tsk.srcNamespace)
	require.Equal(t, "default-registry", tsk.srcName)
	require.Equal(t, ".+", tsk.dstNamespace.String())
	require.Equal(t, "custom-registry", tsk.dstName)
	require.Equal(t, "var a = 0;", tsk.javascript)
	require.Len(t, tsk.jsonpatch, 1)
	require.Equal(t, "remove", tsk.jsonpatch[0].Kind())
	path, err := tsk.jsonpatch[0].Path()
	require.NoError(t, err)
	require.Equal(t, "/status", path)
}

func TestLoadTaskDefinitionsFromFile(t *testing.T) {
	def1 := TaskDefinition{}
	def1.Resource = "secrets"
	def1.Source.Namespace = "auto-ops"
	def1.Source.Name = "mysecret1"
	def1.Target.Namespace = ".+"
	def1.Target.Name = "newsecret1"
	def1.Modification.Javascript = "var a = 0;"
	def1.origin = filepath.Join("testdata", "task2.yaml") + " document 1"

	def2 := TaskDefinition{}
	def2.Resource = "apps/deployments"
	def2.Source.Namespace = "default"
	def2.Source.Name = "mysecret2"
	def2.Target.Namespace = ".+"
	def2.Target.Name = "newsecret2"
	def2.Modification.JSONPatch = []any{
		map[string]any{
			"op":   "remove",
			"path": "/status",
		},
	}
	def2.origin = filepath.Join("testdata", "task2.yaml") + " document 2"

	defs, err := LoadTaskDefinitionsFromFile(filepath.Join("testdata", "task2.yaml"))
	require.NoError(t, err)
	require.Equal(t, TaskDefinitionList{def1, def2}, defs)
}

func TestTaskDefinitionListBuild(t *testing.T) {
	defs, err := LoadTaskDefinitionsFromFile(filepath.Join("testdata", "task2.yaml"))
	require.NoError(t, err)

	_, err = defs.Build()
	require.NoError(t, err)
}

func TestLoadTaskDefinitionFromDir(t *testing.T) {
	defs, err := LoadTaskDefinitionsFromDir("testdata")
	require.NoError(t, err)
	require.Len(t, defs, 3)
}

func TestDigestTaskDefinitionsFromDir(t *testing.T) {
	digest, err := DigestTaskDefinitionsFromDir("testdata")
	require.NoError(t, err)
	require.NotEmpty(t, digest)

	again, err := DigestTaskDefinitionsFromDir("testdata")
	require.NoError(t, err)
	require.Equal(t, digest, again)

	left := t.TempDir()
	right := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(left, "a.yaml"), []byte("ab"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(left, "b.yaml"), []byte(""), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(right, "a.yaml"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(right, "b.yaml"), []byte("b"), 0o644))
	leftDigest, err := DigestTaskDefinitionsFromDir(left)
	require.NoError(t, err)
	rightDigest, err := DigestTaskDefinitionsFromDir(right)
	require.NoError(t, err)
	require.NotEqual(t, leftDigest, rightDigest)

	require.NoError(t, os.WriteFile(filepath.Join(left, "notes.txt"), []byte("ab"), 0o644))
	withIgnored, err := DigestTaskDefinitionsFromDir(left)
	require.NoError(t, err)
	require.Equal(t, leftDigest, withIgnored)
}

func TestLoadTaskDefinitionsSkipsEmptyDocuments(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "tasks.yaml")
	body := []byte(`---
# ignored
---
resource: secrets
source:
  namespace: default
  name: demo
target:
  namespace: app-.*
`)
	require.NoError(t, os.WriteFile(file, body, 0o644))

	defs, err := LoadTaskDefinitionsFromFile(file)
	require.NoError(t, err)
	require.Len(t, defs, 1)
	require.Regexp(t, `document [0-9]+$`, defs[0].origin)

	tasks, err := defs.Build()
	require.NoError(t, err)
	require.Equal(t, "demo", tasks[0].dstName)
}

func TestBuildRejectsDuplicateAndInvalidTasks(t *testing.T) {
	dir := t.TempDir()
	body := []byte(`
resource: secrets
source:
  namespace: default
  name: demo
target:
  namespace: "["
---
resource: not/a/valid/resource
source:
  namespace: default
  name: demo
target:
  namespace: ".+"
---
resource: secrets
source:
  namespace: default
  name: demo
target:
  namespace: ".+"
---
resource: secrets
source:
  namespace: default
  name: demo
target:
  namespace: ".+"
`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.yaml"), body, 0o644))

	_, err := LoadTasks(dir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "target.namespace")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte(`
resource: secrets
source:
  namespace: default
  name: demo
target:
  namespace: ".+"
---
resource: secrets
source:
  namespace: default
  name: demo
target:
  namespace: ".+"
`), 0o644))
	_, err = LoadTasks(dir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate task")
}

func TestSourceNamespaceDefaultsToServiceAccount(t *testing.T) {
	previous := readInClusterNamespace
	t.Cleanup(func() { readInClusterNamespace = previous })
	readInClusterNamespace = func() (string, error) {
		return "pod-ns\n", nil
	}

	def := TaskDefinition{}
	def.Resource = "secrets"
	def.Source.Name = "demo"
	def.Target.Namespace = ".+"
	task, err := def.Build()
	require.NoError(t, err)
	require.Equal(t, "pod-ns", task.srcNamespace)
	require.Equal(t, "demo", task.dstName)
}
