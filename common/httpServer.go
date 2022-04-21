package common

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/srcfoundry/kinesis/component"
)

var (
	runOnce         sync.Once
	defaultHttpPort string = "8080"
)

// HttpSever is designed to be a singleton component which listens to http requests on the defaultHttpPort
type HttpServer struct {
	component.SimpleComponent
	HttpPort string
	router   *mux.Router
	server   http.Server
}

// Init includes checks for singleton HttpServer
func (h *HttpServer) Init(ctx context.Context) error {
	isAlreadyStarted := make(chan bool, 2)
	defer close(isAlreadyStarted)

	runOnce.Do(func() {
		// indicate if initializing for the first time
		isAlreadyStarted <- false
		h.router = mux.NewRouter()
	})

	isAlreadyStarted <- true

	// check the first bool value written to the channel and return error if an HttpServer component had already been initialized.
	if <-isAlreadyStarted {
		return errors.New("already running")
	}

	// if starting for first time would have to drain the channel of remaining value before returning, to avoid memory leak
	<-isAlreadyStarted

	return nil
}

func (h *HttpServer) Start(ctx context.Context) error {
	h.router.PathPrefix("/").HandlerFunc(httpHandlerFunc(h.GetContainer()))

	addr := "0.0.0.0:"
	if len(h.HttpPort) <= 0 {
		addr += defaultHttpPort
	} else {
		addr += h.HttpPort
	}

	h.server = http.Server{
		Addr: addr,
		// Good practice to set timeouts to avoid Slowloris attacks.
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      h.router, // Pass instance of gorilla/mux in.
	}

	if err := h.server.ListenAndServe(); err != nil {
		return err
	}
	return nil
}

func (h *HttpServer) Stop(ctx context.Context) error {
	return h.server.Shutdown(ctx)
}

func httpHandlerFunc(c *component.Container) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		httpHandler := c.GetHttpHandler(r.URL.Path)
		if httpHandler != nil {
			httpHandler(w, r)
			return
		}
		ServeHTTP_404(w, r)
	}
}

func ServeHTTP_404(w http.ResponseWriter, r *http.Request) {
	log.Println("URI:", r.URL, "not found")
	w.WriteHeader(http.StatusNotFound)
}

func ServeHTTP_501(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}
