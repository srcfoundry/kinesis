package main

import (
	"log"
	"os"
	"sync"

	"github.com/srcfoundry/kinesis"
	"github.com/srcfoundry/kinesis/common"
	"github.com/srcfoundry/kinesis/component"
)

func main() {
	app := new(kinesis.App)
	app.Name = "kinesis"
	app.RWMutex = &sync.RWMutex{}
	err := app.Add(app)
	if err != nil {
		log.Printf("failed to start %s, due to %s", app.GetName(), err)
		os.Exit(1)
	}

	httpServer := new(common.HttpServer)
	httpServer.Name = "httpserver"
	httpServer.RWMutex = &sync.RWMutex{}
	httpServer.SetAsNonRestEntity(true)
	err = app.Add(httpServer)
	if err != nil {
		log.Printf("failed to start %s, due to %s", httpServer.GetName(), err)
		os.Exit(1)
	}

	subscribe := make(chan interface{}, 1)
	defer close(subscribe)

	app.Subscribe("main.subscriber", subscribe)

	for notification := range subscribe {
		if notification == component.Stopped {
			log.Println("Exiting")
			os.Exit(0)
		}
	}
}
