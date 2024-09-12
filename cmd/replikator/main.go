package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

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
			return
		}
		log.Fatalln("exited with error:", err.Error())
	}()
	defer rg.Guard(&err)

	log.Println("replikator", AppVersion)

	flags := rg.Must(replikator.ParseFlags())

	tasks := rg.Must(replikator.LoadTasks(flags.Conf))

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
		log.Println("received signal:", sig.String())
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
