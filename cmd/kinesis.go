package main

import (
	"log"
	"os"

	"github.com/srcfoundry/kinesis"
	"github.com/srcfoundry/kinesis/component"
)

func main() {
	app := &kinesis.App{}
	app.Name = "kinesis"
	app.Add(app)

	subscribe := make(chan interface{}, 1)
	defer close(subscribe)

	app.Subscribe("main.subscriber", subscribe)

	for notification := range subscribe {
		log.Println(notification)
		if notification == component.Stopped {
			log.Println("Exiting")
			os.Exit(0)
		}
	}
}
