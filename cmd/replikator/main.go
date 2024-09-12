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

	flags := replikator.ParseFlags()

	tasks := rg.Must(replikator.LoadTasks(flags.Conf))

	client, dynClient := rg.Must2(replikator.CreateClient(flags.Kubeconfig))

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
