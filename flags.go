package replikator

import (
	"errors"
	"flag"
	"os"
	"path/filepath"

	"github.com/yankeguo/rg"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Flags struct {
	Conf       string
	Kubeconfig struct {
		Path      string
		InCluster bool
	}
}

func ParseFlags() (flags Flags, err error) {
	flag.StringVar(&flags.Kubeconfig.Path, "kubeconfig", "", "(optional) absolute path to the kubeconfig file")
	flag.StringVar(&flags.Conf, "conf", ".", "absolute path to the configuration directory")
	flag.Parse()

	flags.Conf = os.ExpandEnv(flags.Conf)
	flags.Kubeconfig.Path = os.ExpandEnv(flags.Kubeconfig.Path)

	if flags.Kubeconfig.Path == "" {
		if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
			flags.Kubeconfig.InCluster = true
		} else if envKubeconfig := os.Getenv("KUBECONFIG"); envKubeconfig != "" {
			flags.Kubeconfig.Path = envKubeconfig
		} else if home, _ := os.UserHomeDir(); home != "" {
			flags.Kubeconfig.Path = filepath.Join(home, ".kube", "config")
		} else {
			err = errors.New("kubeconfig is required")
			return
		}
	}

	return
}

func (flags Flags) CreateKubernetesClient() (client *kubernetes.Clientset, dynClient *dynamic.DynamicClient, err error) {
	defer rg.Guard(&err)

	var conf *rest.Config

	if flags.Kubeconfig.InCluster {
		conf = rg.Must(rest.InClusterConfig())
	} else {
		conf = rg.Must(clientcmd.BuildConfigFromFlags("", flags.Kubeconfig.Path))
	}

	client = rg.Must(kubernetes.NewForConfig(conf))
	dynClient = rg.Must(dynamic.NewForConfig(conf))
	return
}
