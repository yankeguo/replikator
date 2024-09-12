package main

import (
	"log"

	"github.com/yankeguo/replikator"
	"github.com/yankeguo/rg"
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

	flags := replikator.ParseFlags()

	tasks := rg.Must(replikator.LoadTasks(flags.Conf))

	client := rg.Must(replikator.CreateClient(flags.Kubeconfig))

	_ = tasks
	_ = client
}
