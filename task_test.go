package replikator

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadTasks(t *testing.T) {
	tasks, err := LoadTasks("testdata")
	require.NoError(t, err)
	for _, task := range tasks {
		t.Logf("%+v", task)
	}
}
