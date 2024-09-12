package replikator

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
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
