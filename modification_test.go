package replikator

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEvaluateJavaScriptModification(t *testing.T) {
	obj, err := json.Marshal(map[string]any{
		"hello": "world",
	})
	require.NoError(t, err)

	out, err := EvaluateJavaScriptModification(string(obj), `
	resource.hello = resource.hello.toUpperCase();
	`)
	require.NoError(t, err)

	var res map[string]any
	err = json.Unmarshal([]byte(out), &res)
	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"hello": "WORLD",
	}, res)
}

func TestEvaluateJavaScriptModificationTimeout(t *testing.T) {
	obj, err := json.Marshal(map[string]any{
		"hello": "world",
	})
	require.NoError(t, err)

	_, err = EvaluateJavaScriptModification(string(obj), `
	resource.hello = resource.hello.toUpperCase();
	while(true){}
	`)
	require.Error(t, err)
	require.Equal(t, ErrScriptTimeout, err)
}

func TestEvaluateJavaScriptModificationRejectsInvalidScript(t *testing.T) {
	_, err := EvaluateJavaScriptModification(`{"hello":"world"}`, `function(`)
	require.Error(t, err)

	_, err = EvaluateJavaScriptModification(`{"hello":"world"}`, `resource = undefined`)
	require.Error(t, err)
}
