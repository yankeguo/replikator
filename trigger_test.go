package replikator

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTriggerQueueCoalesces(t *testing.T) {
	q := newTriggerQueue()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q.syncAll(false)
			q.syncNamespace("ns")
			q.syncAll(true)
		}()
	}
	wg.Wait()

	select {
	case <-q.notify:
	case <-time.After(time.Second):
		t.Fatal("expected notification")
	}
	select {
	case <-q.notify:
	default:
	}

	req := q.drain()
	require.True(t, req.all)
	require.True(t, req.forceAll)
	require.Contains(t, req.names, "ns")
	require.Contains(t, req.forceName, "ns")
	require.False(t, req.empty())
	require.True(t, q.drain().empty())
}

func TestTriggerQueueForgetOverridesSync(t *testing.T) {
	q := newTriggerQueue()
	q.syncNamespace("ns")
	q.forgetNamespace("ns")

	req := q.drain()
	require.Contains(t, req.deleted, "ns")
	require.NotContains(t, req.names, "ns")
	require.False(t, req.forces("other"))
	require.False(t, req.forces("ns"))

	q.forgetNamespace("ns")
	q.syncNamespace("ns")
	req = q.drain()
	require.NotContains(t, req.deleted, "ns")
	require.True(t, req.forces("ns"))
}
