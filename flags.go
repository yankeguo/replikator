package replikator

import (
	"flag"
	"os"
	"path/filepath"

	"k8s.io/client-go/util/homedir"
)

type Flags struct {
	Conf       string
	Kubeconfig string
}

func ParseFlags() (flags Flags) {
	envKubeconfig := os.Getenv("KUBECONFIG")

	if envKubeconfig == "" {
		if home := homedir.HomeDir(); home != "" {
			flag.StringVar(&flags.Kubeconfig, "kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
		} else {
			flag.StringVar(&flags.Kubeconfig, "kubeconfig", "", "absolute path to the kubeconfig file")
		}
	} else {
		flag.StringVar(&flags.Kubeconfig, "kubeconfig", envKubeconfig, "(optional) absolute path to the kubeconfig file")
	}
	flag.StringVar(&flags.Conf, "conf", "", "absolute path to the configuration directory")
	flag.Parse()

	flags.Conf = os.ExpandEnv(flags.Conf)
	flags.Kubeconfig = os.ExpandEnv(flags.Kubeconfig)
	return
}
