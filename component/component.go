package component

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

type stage int

//go:generate stringer -type=stage
const (
	Submitted stage = iota
	Preinitializing
	Preinitialized
	Initializing
	Initialized
	MessageReceiverStarted
	Starting
	Started
	Stopping
	Stopped
	Tearingdown
	Teareddown
	// Error stages follow
	Aborting
	Restarting
)

type state int

//go:generate stringer -type=state
const (
	Active state = iota
	InActive
)

type Component interface {
	setStage(stage)
	Stage() stage

	setState(state)
	State() state

	preInit()
	tearDown()

	Init(context.Context) error
	Start(context.Context) error
	Stop(context.Context) error
	MessageReceiver(context.Context) error

	// IsRestartableWithDelay indicates if component is to be restarted if Start() fails with error. The method could include logic
	// for exponential backoff to return the delay duration between restarts.
	IsRestartableWithDelay() (bool, time.Duration)

	GetContainer() *Container

	Send(context.Context, interface{}) error
	GetNotificationCh() chan interface{}

	fmt.Stringer
	http.Handler
}

type SimpleComponent struct {
	Name      string
	cURI      string
	container *Container
	stage     stage
	state     state

	notificationCh chan interface{}
	MessageCh      chan interface{}

	mutex *sync.Mutex
}

func (d *SimpleComponent) preInit() {
	d.mutex = &sync.Mutex{}
	d.notificationCh = make(chan interface{}, 1)
	d.MessageCh = make(chan interface{}, 1)
}

func (d *SimpleComponent) tearDown() {
	close(d.MessageCh)
	close(d.notificationCh)
}

func (d *SimpleComponent) String() string {
	return d.cURI
}

func (d *SimpleComponent) setStage(s stage) {
	log.Println(d, s)
	d.stage = s
}

func (d *SimpleComponent) State() state {
	return d.state
}

func (d *SimpleComponent) setState(s state) {
	log.Println(d, s)
	d.state = s
}

func (d *SimpleComponent) Stage() stage {
	return d.stage
}

func (d *SimpleComponent) Init(context.Context) error { return nil }

func (d *SimpleComponent) MessageReceiver(context.Context) error { return nil }

func (d *SimpleComponent) Start(context.Context) error { return nil }

func (d *SimpleComponent) IsRestartableWithDelay() (bool, time.Duration) {
	return true, 3 * time.Second
}

func (d *SimpleComponent) Stop(context.Context) error { return nil }

func (d *SimpleComponent) GetContainer() *Container {
	return d.container
}

func (d *SimpleComponent) Send(context.Context, interface{}) error { return nil }

func (d *SimpleComponent) GetNotificationCh() chan interface{} {
	return d.notificationCh
}

func (d *SimpleComponent) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("{ok}"))
}
