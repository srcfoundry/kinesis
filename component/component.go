package component

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

type (
	stage  int
	state  int
	signal int
)

//go:generate stringer -type=stage
const (
	Submitted stage = iota
	Preinitializing
	Preinitialized
	Initializing
	Initialized
	Starting
	Restarting
	Started
	Stopping
	Stopped
	Tearingdown
	Teareddown
	// Error stages follow
	Aborting
)

//go:generate stringer -type=state
const (
	InActive state = iota
	Active
)

//go:generate stringer -type=signal
const (
	EnablePeerMessaging signal = iota
	DisablePeerMessaging
	Shutdown
	ShutdownAfter
	Cancel
	CancelAfter
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

	// A component could assign a channel by which it could receive contexed messages/notifications wrapped as a function. The component would continue
	// to receive messages until the returned notification channel is closed.
	SetInbox(chan func() (context.Context, interface{})) (<-chan struct{}, error)
	getInbox() chan func() (context.Context, interface{})

	getMmux() chan func() (context.Context, interface{})

	// IsRestartableWithDelay indicates if component is to be restarted if Start() fails with error. The method could include logic
	// for exponential backoff to return the delay duration between restarts.
	IsRestartableWithDelay() (bool, time.Duration)

	GetContainer() *Container

	fmt.Stringer
	http.Handler
}

type SimpleComponent struct {
	Name      string
	cURI      string
	container *Container
	stage     stage
	state     state

	// message mux
	mmux chan func() (context.Context, interface{})

	inbox              chan func() (context.Context, interface{})
	isMessagingStopped chan struct{}

	mutex *sync.Mutex
}

func (d *SimpleComponent) preInit() {
	d.mutex = &sync.Mutex{}
	d.mmux = make(chan func() (context.Context, interface{}), 1)
}

func (d *SimpleComponent) tearDown() {
	if d.isMessagingStopped != nil {
		close(d.isMessagingStopped)
	}
	close(d.mmux)
}

func (d *SimpleComponent) String() string {
	return d.cURI
}

func (d *SimpleComponent) setStage(s stage) {
	log.Println(d, s)
	d.stage = s

	if d.stage >= Initialized && d.stage <= Started {
		d.setState(Active)
	} else {
		d.setState(InActive)
	}
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

func (d *SimpleComponent) Start(context.Context) error { return nil }

func (d *SimpleComponent) IsRestartableWithDelay() (bool, time.Duration) {
	return true, 3 * time.Second
}

func (d *SimpleComponent) Stop(context.Context) error { return nil }

func (d *SimpleComponent) GetContainer() *Container {
	return d.container
}

func (d *SimpleComponent) getMmux() chan func() (context.Context, interface{}) {
	return d.mmux
}

func (d *SimpleComponent) SetInbox(inbox chan func() (context.Context, interface{})) (<-chan struct{}, error) {
	if inbox == nil {
		return nil, errors.New("SetMessageReader was passed an empty messaging channel")
	}

	d.inbox = inbox
	d.isMessagingStopped = make(chan struct{})
	return d.isMessagingStopped, nil
}

func (d *SimpleComponent) getInbox() chan func() (context.Context, interface{}) {
	return d.inbox
}

func (d *SimpleComponent) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("{ok}"))
}
