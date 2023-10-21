package main

import (
	"log"
	"os"

	"net/http"
	_ "net/http/pprof"

	"github.com/srcfoundry/kinesis"
	_ "github.com/srcfoundry/kinesis/addons"
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

	select {}
}
