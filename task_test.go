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

	session := tasks[0].NewSession(TaskOptions{})
	require.NotNil(t, session)
}
