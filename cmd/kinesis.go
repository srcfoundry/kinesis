package main

import (
	"os"

	"net/http"
	_ "net/http/pprof"

	"github.com/srcfoundry/kinesis"
	_ "github.com/srcfoundry/kinesis/addons"
	"github.com/srcfoundry/kinesis/anylogger"
)

func init() {
	go func() {
		anylogger.Errorf()("%s", http.ListenAndServe("localhost:6060", nil))
	}()
}

func main() {
	app := new(kinesis.App)
	app.Name = "kinesis"
	err := app.Add(app)
	if err != nil {
		anylogger.Errorf()("failed to start %s, due to %s", app.GetName(), err)
		os.Exit(1)
	}

	select {}
}
