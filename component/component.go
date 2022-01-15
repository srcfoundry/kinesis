package component

import (
	"context"
	"fmt"
	"net/http"
	"sync"
)

type state int

//go:generate stringer -type=state
const (
	Preinitializing state = iota
	Preinitialized
	Initializing
	Initialized
	Starting
	Started
	Stopping
	Stopped
	Tearingdown
	Teareddown
	// Error states follow
	Aborting
)

type Component interface {
	setState(state)
	State() state

	preInit()
	tearDown()

	// Aside
	Init(context.Context)
	Start(context.Context)
	Stop(context.Context)

	GetContainer() *Container

	Send(interface{})
	GetNotificationCh() chan interface{}

	fmt.Stringer
	http.Handler
}

type SimpleComponent struct {
	Name      string
	cURI      string
	container *Container
	state     state

	notificationCh chan interface{}
	MessageCh      chan interface{}

	mutex *sync.Mutex

	// Max time(ms) that container would wait before Init or Stop call is cancelled.
	// Infra components which must ensure a hard requirement like successful db connection, could set Timeout value to -1,
	// indicating that the container should wait indefinitely until the component has been initialized. If no value is set
	// i.e value is 0, it would default to whatever timeout is considered by the container.
	Timeout int
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

func (d *SimpleComponent) setState(s state) {
	d.state = s
}

func (d *SimpleComponent) String() string {
	return d.cURI
}

func (d *SimpleComponent) State() state {
	return d.state
}

func (d *SimpleComponent) Init(ctx context.Context) {}

func (d *SimpleComponent) Start(ctx context.Context) {}

func (d *SimpleComponent) Stop(ctx context.Context) {}

func (d *SimpleComponent) GetContainer() *Container {
	return d.container
}

func (d *SimpleComponent) Send(interface{}) {}

func (d *SimpleComponent) GetNotificationCh() chan interface{} {
	return d.notificationCh
}

func (d *SimpleComponent) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("{ok}"))
}
