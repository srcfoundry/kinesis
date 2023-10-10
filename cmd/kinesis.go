package main

import (
	"log"
	"os"

	"net/http"
	_ "net/http/pprof"

	"github.com/srcfoundry/kinesis"
	_ "github.com/srcfoundry/kinesis/addons"
	"github.com/srcfoundry/kinesis/component"
)

func init() {
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
}

func main() {
	app := new(kinesis.App)
	app.Name = "kinesis"
	err := app.Add(app)
	if err != nil {
		log.Printf("failed to start %s, due to %s", app.GetName(), err)
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
