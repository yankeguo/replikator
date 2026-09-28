package replikator

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestParseGroupVersionResource(t *testing.T) {
	res, err := ParseGroupVersionResource("v1/pods")
	require.NoError(t, err)
	require.Equal(t, "", res.Group)
	require.Equal(t, "v1", res.Version)
	require.Equal(t, "pods", res.Resource)

	res, err = ParseGroupVersionResource("pods")
	require.NoError(t, err)
	require.Equal(t, "", res.Group)
	require.Equal(t, "v1", res.Version)
	require.Equal(t, "pods", res.Resource)

	res, err = ParseGroupVersionResource("apps/deployments")
	require.NoError(t, err)
	require.Equal(t, "apps", res.Group)
	require.Equal(t, "v1", res.Version)
	require.Equal(t, "deployments", res.Resource)

	res, err = ParseGroupVersionResource("networking.k8s.io/v1/ingresses")
	require.NoError(t, err)
	require.Equal(t, "networking.k8s.io", res.Group)
	require.Equal(t, "v1", res.Version)
	require.Equal(t, "ingresses", res.Resource)

	res, err = ParseGroupVersionResource("v1beta1/ingresses")
	require.NoError(t, err)
	require.Equal(t, "", res.Group)
	require.Equal(t, "v1beta1", res.Version)
	require.Equal(t, "ingresses", res.Resource)

	res, err = ParseGroupVersionResource("view/customresources")
	require.NoError(t, err)
	require.Equal(t, "view", res.Group)
	require.Equal(t, "v1", res.Version)
	require.Equal(t, "customresources", res.Resource)

	_, err = ParseGroupVersionResource("")
	require.Error(t, err)
	_, err = ParseGroupVersionResource("apps/v1/")
	require.Error(t, err)
	_, err = ParseGroupVersionResource("a/b/c/d")
	require.Error(t, err)
	_, err = ParseGroupVersionResource("networking.k8s.io/not-a-version/ingresses")
	require.Error(t, err)
}

func TestRetrieveMetadataName(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetName("demo")
	name, err := RetrieveMetadataName(obj)
	require.NoError(t, err)
	require.Equal(t, "demo", name)

	name, err = RetrieveMetadataName(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns"}})
	require.NoError(t, err)
	require.Equal(t, "ns", name)

	name, err = RetrieveMetadataName(map[string]any{
		"metadata": map[string]any{"name": "from-map"},
	})
	require.NoError(t, err)
	require.Equal(t, "from-map", name)

	_, err = RetrieveMetadataName(nil)
	require.Error(t, err)
	_, err = RetrieveMetadataName(&unstructured.Unstructured{})
	require.Error(t, err)
}
