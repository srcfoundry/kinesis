////go:build http

package addons

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	. "github.com/srcfoundry/kinesis/component"
	"go.uber.org/zap"
)

func init() {
	httpServer := new(HttpServer)
	httpServer.Name = "httpserver"
	httpServer.SetAsNonRestEntity(true)
	AttachComponent(false, httpServer)
}

var (
	runOnceHttp     sync.Once
	defaultHttpPort string = "8080"
)

// HttpSever is designed to be a singleton component which listens to http requests on the defaultHttpPort
type HttpServer struct {
	SimpleComponent
	HttpPort string
	router   *mux.Router
	server   http.Server
}

// Init includes checks for singleton HttpServer
func (h *HttpServer) Init(ctx context.Context) error {
	isAlreadyStarted := make(chan bool, 2)
	defer close(isAlreadyStarted)

	runOnceHttp.Do(func() {
		// indicate if initializing for the first time
		isAlreadyStarted <- false
		h.router = mux.NewRouter()
	})

	isAlreadyStarted <- true

	// check the first bool value written to the channel and return error if an HttpServer component had already been initialized.
	if <-isAlreadyStarted {
		return fmt.Errorf("error initializing %v since already running", h.GetName())
	}

	// if starting for first time would have to drain the channel of remaining value before returning, to avoid memory leak
	<-isAlreadyStarted

	return nil
}

func (h *HttpServer) Start(ctx context.Context) error {
	h.router.PathPrefix("/").HandlerFunc(httpHandlerFunc(ctx, h.GetContainer()))

	addr := "0.0.0.0:"
	if len(h.HttpPort) <= 0 {
		h.HttpPort = defaultHttpPort
	}

	addr += h.HttpPort

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

func httpHandlerFunc(ctx context.Context, c *Container) http.HandlerFunc {
	logger := LoggerFromContext(ctx)
	if c == nil {
		logger.Error("encountered nil container")
	}

	return func(w http.ResponseWriter, r *http.Request) {
		httpHandler := c.GetHttpHandler(r.URL.Path)
		if httpHandler != nil {
			traceMiddleware(ctx, httpHandler)(w, r)
			return
		}
		http.NotFound(w, r)
	}
}

// Middleware to check for traceId and inject if missing
func traceMiddleware(ctx context.Context, next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := LoggerFromContext(ctx)

		traceID := ""
		// Check headers for trace ID. header.Get() assumes keys stored in canonicalized form, thus case insensitive.
		for _, header := range []string{"TraceID", "Trace-ID"} {
			traceID = r.Header.Get(header)
			if len(traceID) > 0 {
				break
			}
		}

		// Check if the traceId is present in the incoming request
		if traceID == "" {
			// Generate a new traceId if not present
			traceID = uuid.New().String()
			logger.Debug("generated new traceId", zap.String("traceID", traceID))
		} else {
			logger.Debug("found existing traceId", zap.String("traceID", traceID))
		}

		// pass traceId in the context for use in handlers or downstream calls
		ctx := context.WithValue(r.Context(), TraceID, traceID)
		r = r.WithContext(ctx)

		// Add the traceId to the response headers as well (optional)
		w.Header().Set("traceId", traceID)

		// Call the next handler in the chain
		next.ServeHTTP(w, r)
	})
}
