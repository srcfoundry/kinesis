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
	app.Add(app)

	httpServer := new(common.HttpServer)
	httpServer.Name = "httpserver"
	httpServer.RWMutex = &sync.RWMutex{}
	app.Add(httpServer)

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
