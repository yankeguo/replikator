package main

import (
	"context"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/yankeguo/replikator"
	"github.com/yankeguo/rg"
)

var (
	AppVersion = "dev"
)

func createReloadChanel(mainCtx context.Context, dir string) chan struct{} {
	reload := make(chan struct{}, 1)

	digest, _ := replikator.DigestTaskDefinitionsFromDir(dir)

	go func() {
		defer close(reload)

		for {
			select {
			case <-mainCtx.Done():
				return
			case <-time.After(time.Second * 10):
			}

			if newDigest, _ := replikator.DigestTaskDefinitionsFromDir(dir); newDigest != digest {
				digest = newDigest
				log.WithField("digest", digest).Info("task definitions changed")
				select {
				case <-mainCtx.Done():
				case reload <- struct{}{}:
				}
			}
		}
	}()

	return reload
}

func createReloadContextChannel(mainCtx context.Context, dir string) chan context.Context {
	chCtx := make(chan context.Context, 1)

	reload := createReloadChanel(mainCtx, dir)

	go func() {
		defer close(chCtx)

		ctx, cancel := context.WithCancel(mainCtx)

		chCtx <- ctx

		for {
			select {
			case <-mainCtx.Done():
				if cancel != nil {
					cancel()
				}
				return
			case <-reload:
				if cancel != nil {
					cancel()
				}
				ctx, cancel = context.WithCancel(mainCtx)
				chCtx <- ctx
			}
		}
	}()

	return chCtx
}

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

	if verbose, _ := strconv.ParseBool(os.Getenv("VERBOSE")); verbose {
		log.SetLevel(log.DebugLevel)
		log.SetFormatter(&log.TextFormatter{ForceColors: true})
	} else {
		log.SetLevel(log.InfoLevel)
		log.SetFormatter(&log.TextFormatter{})
	}

	log.WithField("version", AppVersion).Info("replikator starting")

	flags := rg.Must(replikator.ParseFlags())

	client, dynClient := rg.Must2(flags.CreateKubernetesClient())

	mainCtx, cancelMainCtx := context.WithCancel(context.Background())
	defer cancelMainCtx()

	go func() {
		chSig := make(chan os.Signal, 1)
		signal.Notify(chSig, syscall.SIGTERM, syscall.SIGINT)
		sig := <-chSig
		log.WithField("signal", sig.String()).Warn("signal received")
		cancelMainCtx()
	}()

	routine := func(ctx context.Context) (err error) {
		defer rg.Guard(&err)
		defs := rg.Must(replikator.LoadTaskDefinitionsFromDir(flags.Conf))
		tasks := rg.Must(defs.Build())
		log.WithField("count", len(tasks)).Info("tasks loaded")
		tasks.NewSessions(replikator.TaskOptions{
			Client:        client,
			DynamicClient: dynClient,
		}).Run(ctx)
		return
	}

	chCtx := createReloadContextChannel(mainCtx, flags.Conf)

	for ctx := range chCtx {
		if err = routine(ctx); err != nil {
			return
		}
	}
}
