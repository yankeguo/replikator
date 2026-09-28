package replikator

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseFlags(t *testing.T) {
	t.Setenv("MY_CONF", "/data/conf")
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	t.Setenv("KUBECONFIG", "/should/not/use")

	flags, err := parseFlags([]string{"-conf", "$MY_CONF", "-kubeconfig", "/tmp/kube"}, io.Discard)
	require.NoError(t, err)
	require.Equal(t, "/data/conf", flags.Conf)
	require.Equal(t, "/tmp/kube", flags.Kubeconfig.Path)
	require.False(t, flags.Kubeconfig.InCluster)

	flags, err = parseFlags(nil, io.Discard)
	require.NoError(t, err)
	require.True(t, flags.Kubeconfig.InCluster)
	require.Equal(t, ".", flags.Conf)

	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	flags, err = parseFlags(nil, io.Discard)
	require.NoError(t, err)
	require.False(t, flags.Kubeconfig.InCluster)
	require.Equal(t, "/should/not/use", flags.Kubeconfig.Path)

	t.Setenv("KUBECONFIG", "")
	flags, err = parseFlags(nil, io.Discard)
	require.NoError(t, err)
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, ".kube", "config"), flags.Kubeconfig.Path)

	_, err = parseFlags([]string{"-h"}, io.Discard)
	require.ErrorIs(t, err, flag.ErrHelp)

	_, err = parseFlags([]string{"-conf", ""}, io.Discard)
	require.Error(t, err)
	_, err = parseFlags([]string{"-unknown"}, io.Discard)
	require.Error(t, err)
}

func TestRESTConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")
	body := []byte(`apiVersion: v1
kind: Config
clusters:
- name: c
  cluster:
    server: https://127.0.0.1:1
users:
- name: u
  user:
    token: t
contexts:
- name: x
  context:
    cluster: c
    user: u
current-context: x
`)
	require.NoError(t, os.WriteFile(path, body, 0o644))

	flags := Flags{}
	flags.Kubeconfig.Path = path
	conf, err := flags.restConfig()
	require.NoError(t, err)
	require.Equal(t, "replikator", conf.UserAgent)
	require.Equal(t, "https://127.0.0.1:1", conf.Host)
	require.Zero(t, conf.Timeout)

	client, dynClient, err := flags.CreateKubernetesClient()
	require.NoError(t, err)
	require.NotNil(t, client)
	require.NotNil(t, dynClient)

	flags.Kubeconfig.InCluster = true
	flags.Kubeconfig.Path = ""
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")
	_, err = flags.restConfig()
	require.Error(t, err)
}
