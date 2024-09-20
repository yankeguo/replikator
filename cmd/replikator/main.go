package main

import (
	"context"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"

	log "github.com/sirupsen/logrus"
	"github.com/yankeguo/replikator"
	"github.com/yankeguo/rg"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	AppVersion = "dev"
)

func main() {
	var err error
	defer func() {
		if err == nil {
			log.Info("replikator exited")
		} else {
			log.WithError(err).Error("replikator exited")
			os.Exit(1)
		}
	}()
	defer rg.Guard(&err)

	// setup logrus
	if verbose, _ := strconv.ParseBool(os.Getenv("VERBOSE")); verbose {
		log.SetLevel(log.DebugLevel)
		log.SetFormatter(&log.TextFormatter{ForceColors: true})
	} else {
		log.SetLevel(log.InfoLevel)
		log.SetFormatter(&log.TextFormatter{})
	}

	log.WithField("version", AppVersion).Info("replikator starting")

	flags := rg.Must(replikator.ParseFlags())

	tasks := rg.Must(replikator.LoadTasks(flags.Conf))

	log.WithField("count", len(tasks)).Info("tasks loaded")

	var conf *rest.Config

	if flags.Kubeconfig.InCluster {
		conf = rg.Must(rest.InClusterConfig())
	} else {
		conf = rg.Must(clientcmd.BuildConfigFromFlags("", flags.Kubeconfig.Path))
	}

	client := rg.Must(kubernetes.NewForConfig(conf))
	dynClient := rg.Must(dynamic.NewForConfig(conf))

	wg := &sync.WaitGroup{}

	ctx := context.Background()
	ctx, ctxCancel := context.WithCancel(ctx)

	go func() {
		chSig := make(chan os.Signal, 1)
		signal.Notify(chSig, syscall.SIGTERM, syscall.SIGINT)
		sig := <-chSig
		log.WithField("signal", sig.String()).Warn("signal received")
		ctxCancel()
	}()

	for _, task := range tasks {
		wg.Add(1)
		go replikator.Run(
			ctx, replikator.RunOptions{
				WaitGroup: wg,
				Task:      task,
				Client:    client,
				DynClient: dynClient,
			},
		)
	}

	wg.Wait()
}
