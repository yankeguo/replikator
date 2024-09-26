package replikator

import (
	"testing"

	"github.com/stretchr/testify/require"
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

}
