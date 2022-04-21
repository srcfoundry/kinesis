package main

import (
	"log"
	"os"

	"github.com/srcfoundry/kinesis"
	"github.com/srcfoundry/kinesis/common"
	"github.com/srcfoundry/kinesis/component"
)

func main() {
	app := new(kinesis.App)
	app.Name = "kinesis"
	app.Add(app)

	httpServer := new(common.HttpServer)
	httpServer.Name = "httpserver"
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
