package replikator

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func TestRunStopsOnCancelAndKeepsTasksWhenReloadFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "task.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
resource: secrets
source:
  namespace: src-ns
  name: src
target:
  namespace: dst-.*
  name: copy
`), 0o644))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, dir, RunOptions{
			PollInterval: 20 * time.Millisecond,
			TaskOptions: TaskOptions{
				Client:        fake.NewSimpleClientset(),
				DynamicClient: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()),
			},
		})
	}()

	time.Sleep(40 * time.Millisecond)
	require.NoError(t, os.WriteFile(path, []byte(`
resource: secrets
source:
  namespace: src-ns
  name: src
target:
  namespace: "["
`), 0o644))
	time.Sleep(80 * time.Millisecond)

	select {
	case err := <-done:
		t.Fatalf("exited before cancellation: %v", err)
	default:
	}

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

func TestRunRejectsInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "task.yaml"), []byte("resource: a/b/c/d\n"), 0o644))

	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), dir, RunOptions{
			TaskOptions: TaskOptions{
				Client:        fake.NewSimpleClientset(),
				DynamicClient: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()),
			},
		})
	}()
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Run blocked on invalid config")
	}

	err := Run(context.Background(), filepath.Join(t.TempDir(), "missing"), RunOptions{})
	require.Error(t, err)
}
