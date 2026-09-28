package replikator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic/fake"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

var secretGVR = schema.GroupVersionResource{Version: "v1", Resource: "secrets"}

func testSecret(namespace, name, token string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]any{
			"name":              name,
			"namespace":         namespace,
			"uid":               "uid-1",
			"resourceVersion":   "7",
			"labels":            map[string]any{"app": "demo"},
			"annotations":       map[string]any{"note": "keep"},
			"managedFields":     []any{map[string]any{"manager": "kubectl"}},
			"ownerReferences":   []any{map[string]any{"name": "owner"}},
			"creationTimestamp": "2020-01-01T00:00:00Z",
		},
		"type":   "Opaque",
		"data":   map[string]any{"token": token},
		"status": map[string]any{"phase": "Ready"},
	}}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "Secret"})
	return obj
}

func installApplyReactor(dyn *fake.FakeDynamicClient) {
	dyn.PrependReactor("patch", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		patch, ok := action.(clienttesting.PatchAction)
		if !ok {
			return false, nil, nil
		}
		obj := &unstructured.Unstructured{}
		if err := obj.UnmarshalJSON(patch.GetPatch()); err != nil {
			return true, nil, err
		}
		gvr := action.GetResource()
		ns := action.GetNamespace()
		_, err := dyn.Tracker().Get(gvr, ns, obj.GetName())
		if apierrors.IsNotFound(err) {
			if err := dyn.Tracker().Create(gvr, obj, ns); err != nil {
				return true, nil, err
			}
			return true, obj.DeepCopy(), nil
		}
		if err != nil {
			return true, nil, err
		}
		if err := dyn.Tracker().Update(gvr, obj, ns); err != nil {
			return true, nil, err
		}
		return true, obj.DeepCopy(), nil
	})
}

func patchCount(dyn *fake.FakeDynamicClient) int {
	count := 0
	for _, action := range dyn.Actions() {
		if action.GetVerb() == "patch" {
			count++
		}
	}
	return count
}

func testSession(t *testing.T, src *unstructured.Unstructured, namespaces ...*corev1.Namespace) (*Session, *fake.FakeDynamicClient) {
	t.Helper()
	objects := make([]runtime.Object, 0, len(namespaces))
	for _, namespace := range namespaces {
		objects = append(objects, namespace)
	}
	client := kubernetesfake.NewSimpleClientset(objects...)
	dyn := fake.NewSimpleDynamicClient(runtime.NewScheme(), src)
	installApplyReactor(dyn)

	def := TaskDefinition{}
	def.Resource = "secrets"
	def.Source.Namespace = "src-ns"
	def.Source.Name = "src"
	def.Target.Namespace = "dst-.*"
	def.Target.Name = "copy"
	def.Modification.JSONPatch = []any{map[string]any{"op": "remove", "path": "/status"}}
	def.Modification.Javascript = `resource.metadata.labels = resource.metadata.labels || {}; resource.metadata.labels.copied = "yes";`
	task, err := def.Build()
	require.NoError(t, err)

	return task.NewSession(TaskOptions{Client: client, DynamicClient: dyn}), dyn
}

func TestReconcileCopiesSourceAndSkipsUnchanged(t *testing.T) {
	now := metav1.Now()
	session, dyn := testSession(t, testSecret("src-ns", "src", "YQ=="),
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "src-ns"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "dst-a"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "dst-b"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "other"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "dst-term", DeletionTimestamp: &now, Finalizers: []string{"keep"}}},
	)

	err := session.reconcile(context.Background(), reconcileRequest{all: true, forceAll: true})
	require.NoError(t, err)
	require.Equal(t, 2, patchCount(dyn))

	for _, namespace := range []string{"dst-a", "dst-b"} {
		stored, err := dyn.Tracker().Get(secretGVR, namespace, "copy")
		require.NoError(t, err)
		got := stored.(*unstructured.Unstructured)
		require.Equal(t, namespace, got.GetNamespace())
		require.Equal(t, "copy", got.GetName())
		require.Empty(t, string(got.GetUID()))
		require.Empty(t, got.GetResourceVersion())
		require.Equal(t, map[string]string{"app": "demo", "copied": "yes"}, got.GetLabels())
		require.Equal(t, "true", got.GetAnnotations()[AnnotationManaged])
		require.Equal(t, "src-ns", got.GetAnnotations()[AnnotationSourceNamespace])
		require.Equal(t, "src", got.GetAnnotations()[AnnotationSourceName])
		require.Equal(t, "keep", got.GetAnnotations()["note"])
		token, found, err := unstructured.NestedString(got.Object, "data", "token")
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, "YQ==", token)
		_, found, err = unstructured.NestedFieldNoCopy(got.Object, "status")
		require.NoError(t, err)
		require.False(t, found)
		_, found, err = unstructured.NestedFieldNoCopy(got.Object, "metadata", "ownerReferences")
		require.NoError(t, err)
		require.False(t, found)
	}

	_, err = dyn.Tracker().Get(secretGVR, "src-ns", "copy")
	require.True(t, apierrors.IsNotFound(err))
	_, err = dyn.Tracker().Get(secretGVR, "other", "copy")
	require.True(t, apierrors.IsNotFound(err))
	_, err = dyn.Tracker().Get(secretGVR, "dst-term", "copy")
	require.True(t, apierrors.IsNotFound(err))

	err = session.reconcile(context.Background(), reconcileRequest{all: true})
	require.NoError(t, err)
	require.Equal(t, 2, patchCount(dyn))

	updated := testSecret("src-ns", "src", "YQ==")
	updated.Object["status"] = map[string]any{"phase": "Changed"}
	require.NoError(t, dyn.Tracker().Update(secretGVR, updated, "src-ns"))
	err = session.reconcile(context.Background(), reconcileRequest{all: true})
	require.NoError(t, err)
	require.Equal(t, 2, patchCount(dyn))

	updated.Object["data"] = map[string]any{"token": "Yw=="}
	require.NoError(t, dyn.Tracker().Update(secretGVR, updated, "src-ns"))
	err = session.reconcile(context.Background(), reconcileRequest{all: true})
	require.NoError(t, err)
	require.Equal(t, 4, patchCount(dyn))
	stored, err := dyn.Tracker().Get(secretGVR, "dst-a", "copy")
	require.NoError(t, err)
	token, _, err := unstructured.NestedString(stored.(*unstructured.Unstructured).Object, "data", "token")
	require.NoError(t, err)
	require.Equal(t, "Yw==", token)
}

func TestReconcileContinuesAfterNamespaceFailure(t *testing.T) {
	session, dyn := testSession(t, testSecret("src-ns", "src", "YQ=="),
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "src-ns"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "dst-a"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "dst-b"}},
	)
	dyn.PrependReactor("patch", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if action.GetNamespace() == "dst-b" {
			return true, nil, errors.New("boom")
		}
		return false, nil, nil
	})

	err := session.reconcile(context.Background(), reconcileRequest{all: true, forceAll: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "dst-b/copy")
	_, err = dyn.Tracker().Get(secretGVR, "dst-a", "copy")
	require.NoError(t, err)
	_, err = dyn.Tracker().Get(secretGVR, "dst-b", "copy")
	require.True(t, apierrors.IsNotFound(err))
	require.Contains(t, session.versions, "dst-a")
	require.NotContains(t, session.versions, "dst-b")
}

func TestReconcileDeletesOnlyManagedReplicas(t *testing.T) {
	session, dyn := testSession(t, testSecret("src-ns", "src", "YQ=="),
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "src-ns"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "dst-a"}},
	)
	require.NoError(t, session.reconcile(context.Background(), reconcileRequest{all: true, forceAll: true}))

	foreign := testSecret("dst-a", "foreign", "YQ==")
	require.NoError(t, dyn.Tracker().Create(secretGVR, foreign, "dst-a"))
	require.NoError(t, dyn.Tracker().Delete(secretGVR, "src-ns", "src"))

	err := session.reconcile(context.Background(), reconcileRequest{all: true, forceAll: true})
	require.NoError(t, err)
	_, err = dyn.Tracker().Get(secretGVR, "dst-a", "copy")
	require.True(t, apierrors.IsNotFound(err))
	_, err = dyn.Tracker().Get(secretGVR, "dst-a", "foreign")
	require.NoError(t, err)
	require.Empty(t, session.versions)
}

func TestRunReturnsWhenContextIsCancelled(t *testing.T) {
	session, _ := testSession(t, testSecret("src-ns", "src", "YQ=="),
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "src-ns"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "dst-a"}},
	)
	session.fullSyncInterval = time.Hour
	session.watchRetryInterval = time.Hour

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		session.Run(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("session did not stop")
	}
}

func TestWatchDispatchesEvents(t *testing.T) {
	session, dyn := testSession(t, testSecret("src-ns", "src", "YQ=="))
	nsWatcher := watch.NewRaceFreeFake()
	resWatcher := watch.NewRaceFreeFake()
	selector := make(chan string, 1)

	session.client.(*kubernetesfake.Clientset).PrependWatchReactor("namespaces", func(action clienttesting.Action) (bool, watch.Interface, error) {
		return true, nsWatcher, nil
	})
	dyn.PrependWatchReactor("*", func(action clienttesting.Action) (bool, watch.Interface, error) {
		if watchAction, ok := action.(clienttesting.WatchAction); ok {
			selector <- watchAction.GetWatchRestrictions().Fields.String()
		}
		return true, resWatcher, nil
	})

	q := newTriggerQueue()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- session.watchOnce(ctx, q)
	}()

	select {
	case got := <-selector:
		require.Contains(t, got, "metadata.name=src")
	case <-time.After(2 * time.Second):
		t.Fatal("watch was not started")
	}
	select {
	case <-q.notify:
	case <-time.After(2 * time.Second):
		t.Fatal("missing initial sync")
	}
	require.True(t, q.drain().forceAll)

	nsWatcher.Add(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "dst-a"}})
	select {
	case <-q.notify:
	case <-time.After(2 * time.Second):
		t.Fatal("missing namespace sync")
	}
	req := q.drain()
	require.Contains(t, req.names, "dst-a")
	require.True(t, req.forces("dst-a"))

	now := metav1.Now()
	nsWatcher.Add(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "dst-b", DeletionTimestamp: &now}})
	nsWatcher.Add(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "other"}})
	select {
	case <-q.notify:
		t.Fatal("ignored namespaces queued a sync")
	case <-time.After(50 * time.Millisecond):
	}

	nsWatcher.Delete(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "dst-a"}})
	select {
	case <-q.notify:
	case <-time.After(2 * time.Second):
		t.Fatal("missing namespace delete")
	}
	require.Contains(t, q.drain().deleted, "dst-a")

	changed := &unstructured.Unstructured{}
	changed.SetName("src")
	resWatcher.Modify(changed)
	select {
	case <-q.notify:
	case <-time.After(2 * time.Second):
		t.Fatal("missing resource sync")
	}
	req = q.drain()
	require.True(t, req.all)
	require.False(t, req.forceAll)

	resWatcher.Error(&metav1.Status{Reason: "Expired", Message: "boom"})
	select {
	case err := <-done:
		require.Error(t, err)
		require.Contains(t, err.Error(), "boom")
	case <-time.After(2 * time.Second):
		t.Fatal("watch error was not returned")
	}
}
