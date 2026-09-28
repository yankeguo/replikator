package replikator

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTaskNewSession(t *testing.T) {
	defs, err := LoadTaskDefinitionsFromFile(filepath.Join("testdata", "task2.yaml"))
	require.NoError(t, err)

	tasks, err := defs.Build()
	require.NoError(t, err)

	first := tasks[0].NewSession(TaskOptions{})
	second := tasks[0].NewSession(TaskOptions{})
	require.NotNil(t, first)
	require.NotEqual(t, first.log.Data["session"], second.log.Data["session"])
	require.Contains(t, first.task.String(), "secrets auto-ops/mysecret1")
}
