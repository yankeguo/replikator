package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	log "github.com/sirupsen/logrus"
	"github.com/yankeguo/replikator"
	"github.com/yankeguo/rg"
)

var (
	AppVersion = "dev"
)

func main() {
	var (
		err     error
		started bool
	)
	defer func() {
		if err == nil {
			if started {
				log.Info("replikator exited")
			}
			return
		}
		log.WithError(err).Error("replikator exited")
		os.Exit(1)
	}()
	defer rg.Guard(&err)

	configureLogging()
	log.WithField("version", AppVersion).Info("replikator starting")

	flags, err := replikator.ParseFlags()
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			err = nil
		}
		return
	}

	client, dynClient, err := flags.CreateKubernetesClient()
	if err != nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go watchSignals(cancel)

	started = true
	err = replikator.Run(ctx, flags.Conf, replikator.RunOptions{
		TaskOptions: replikator.TaskOptions{
			Client:        client,
			DynamicClient: dynClient,
		},
	})
}

func configureLogging() {
	verbose, _ := strconv.ParseBool(os.Getenv("VERBOSE"))
	if verbose {
		log.SetLevel(log.DebugLevel)
		log.SetFormatter(&log.TextFormatter{ForceColors: true})
		return
	}
	log.SetLevel(log.InfoLevel)
	log.SetFormatter(&log.TextFormatter{})
}

func watchSignals(cancel context.CancelFunc) {
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	sig := <-sigCh
	log.WithField("signal", sig.String()).Warn("shutdown signal received")
	cancel()

	sig = <-sigCh
	log.WithField("signal", sig.String()).Error("second signal received, exiting")
	os.Exit(1)
}
