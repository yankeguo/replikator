package replikator

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

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
	require.Equal(t, "\"remove\"", string(*tsk.jsonpatch[0]["op"]))
	require.Equal(t, "\"/status\"", string(*tsk.jsonpatch[0]["path"]))
}

func TestLoadTaskDefinitionFromFile(t *testing.T) {
	def1 := TaskDefinition{}
	def1.Resource = "secrets"
	def1.Source.Namespace = "auto-ops"
	def1.Source.Name = "mysecret1"
	def1.Target.Namespace = ".+"
	def1.Target.Name = "newsecret1"
	def1.Modification.Javascript = "var a = 0;"

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

	defs, err := LoadTaskDefinitionFromFile(filepath.Join("testdata", "task2.yaml"))
	require.NoError(t, err)
	require.Equal(t, []TaskDefinition{def1, def2}, defs)
}

func TestLoadTaskDefinitionFromDir(t *testing.T) {
	defs, err := LoadTaskDefinitionsFromDir("testdata")
	require.NoError(t, err)
	require.Len(t, defs, 3)
}
