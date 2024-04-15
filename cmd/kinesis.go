package main

import (
	"context"
	"os"

	"net/http"
	_ "net/http/pprof"

	"github.com/srcfoundry/kinesis"
	_ "github.com/srcfoundry/kinesis/addons"
	. "github.com/srcfoundry/kinesis/component"
	"go.uber.org/zap"
)

func init() {
	go func() {
		LoggerFromContext(context.Background()).Info("starting profiler", zap.Error(http.ListenAndServe("localhost:6060", nil)))
	}()
}

func main() {
	app := new(kinesis.App)
	app.Name = "kinesis"
	err := app.Add(app)
	if err != nil {
		app.GetLogger().Error("failed to start", zap.Error(err))
		os.Exit(1)
	}

	select {}
}
