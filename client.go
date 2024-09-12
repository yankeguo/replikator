package replikator

import (
	"errors"
	"os"

	"github.com/yankeguo/rg"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// CreateClient creates a kubernetes client with both in-cluster and out-of-cluster support
func CreateClient(kubeconfig string) (client *kubernetes.Clientset, err error) {
	defer rg.Guard(&err)

	var config *rest.Config

	if kubeconfig == "" {
		if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
			config = rg.Must(rest.InClusterConfig())
		} else {
			err = errors.New("kubeconfig is required")
			return
		}
	} else {
		config = rg.Must(clientcmd.BuildConfigFromFlags("", kubeconfig))
	}

	client = rg.Must(kubernetes.NewForConfig(config))
	return
}
