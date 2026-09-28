package replikator

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Flags is the parsed command line.
type Flags struct {
	Conf       string
	Kubeconfig struct {
		Path      string
		InCluster bool
	}
}

// ParseFlags parses os.Args.
func ParseFlags() (Flags, error) {
	return parseFlags(os.Args[1:], os.Stderr)
}

func parseFlags(args []string, output io.Writer) (Flags, error) {
	var flags Flags
	fs := flag.NewFlagSet("replikator", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: replikator [flags]\n\nReplicate Kubernetes resources across namespaces.\n\n")
		fs.PrintDefaults()
	}
	fs.StringVar(&flags.Kubeconfig.Path, "kubeconfig", "", "(optional) absolute path to the kubeconfig file")
	fs.StringVar(&flags.Conf, "conf", ".", "path to the configuration directory")
	if err := fs.Parse(args); err != nil {
		return Flags{}, err
	}

	expandedConf := strings.TrimSpace(os.ExpandEnv(flags.Conf))
	if expandedConf == "" {
		return Flags{}, errors.New("conf is required")
	}
	flags.Conf = filepath.Clean(expandedConf)

	flags.Kubeconfig.Path = os.ExpandEnv(flags.Kubeconfig.Path)
	if flags.Kubeconfig.Path != "" {
		flags.Kubeconfig.Path = filepath.Clean(flags.Kubeconfig.Path)
	}

	if flags.Kubeconfig.Path == "" {
		switch {
		case os.Getenv("KUBERNETES_SERVICE_HOST") != "":
			flags.Kubeconfig.InCluster = true
		case os.Getenv("KUBECONFIG") != "":
			flags.Kubeconfig.Path = filepath.Clean(os.Getenv("KUBECONFIG"))
		default:
			home, err := os.UserHomeDir()
			if err != nil || home == "" {
				return Flags{}, errors.New("kubeconfig is required")
			}
			flags.Kubeconfig.Path = filepath.Join(home, ".kube", "config")
		}
	}
	return flags, nil
}

func (flags Flags) restConfig() (*rest.Config, error) {
	var (
		conf *rest.Config
		err  error
	)
	if flags.Kubeconfig.InCluster {
		conf, err = rest.InClusterConfig()
	} else {
		conf, err = clientcmd.BuildConfigFromFlags("", flags.Kubeconfig.Path)
	}
	if err != nil {
		return nil, fmt.Errorf("kubernetes config: %w", err)
	}
	// Leave Timeout unset. A request timeout also aborts long-lived watches.
	conf.UserAgent = "replikator"
	return conf, nil
}

// CreateKubernetesClient builds a typed client and a dynamic client from flags.
func (flags Flags) CreateKubernetesClient() (kubernetes.Interface, dynamic.Interface, error) {
	conf, err := flags.restConfig()
	if err != nil {
		return nil, nil, err
	}
	client, err := kubernetes.NewForConfig(conf)
	if err != nil {
		return nil, nil, fmt.Errorf("kubernetes client: %w", err)
	}
	dynClient, err := dynamic.NewForConfig(conf)
	if err != nil {
		return nil, nil, fmt.Errorf("dynamic client: %w", err)
	}
	return client, dynClient, nil
}
