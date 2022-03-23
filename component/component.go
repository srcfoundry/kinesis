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
	stage      int
	state      int
	controlMsg int
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

//go:generate stringer -type=controlMsg
const (
	EnablePeerMessaging controlMsg = iota
	DisablePeerMessaging
	Shutdown
	ShutdownAfter
	Cancel
	CancelAfter
)

type Component interface {
	GetName() string

	setStage(stage)
	Stage() stage

	setState(state)
	State() state

	preInit()
	tearDown()

	Init(context.Context) error
	Start(context.Context) error
	Stop(context.Context) error

	// Use Notify as a primary means to asynchronously send any type of message to the component, wrapped as a function.
	Notify(func() (context.Context, interface{}, chan<- error))

	// A component could assign a channel by which it could receive contexed messages/notifications wrapped as a function. The component would continue
	// to receive messages until the returned notification channel is closed.
	SetInbox(chan func() (context.Context, interface{}, chan<- error)) (<-chan struct{}, error)
	getInbox() chan func() (context.Context, interface{}, chan<- error)

	getMmux() chan func() (context.Context, interface{}, chan<- error)

	// IsRestartableWithDelay indicates if component is to be restarted if Start() fails with error. The method could include logic
	// for exponential backoff to return the delay duration between restarts.
	IsRestartableWithDelay() (bool, time.Duration)

	setContainer(*Container)
	GetContainer() *Container

	fmt.Stringer
	http.Handler
}

type SimpleComponent struct {
	Hash      uint64 `hash:"ignore"`
	Name      string
	container *Container
	stage     stage
	state     state

	// message mux
	mmux chan func() (context.Context, interface{}, chan<- error)

	inbox              chan func() (context.Context, interface{}, chan<- error)
	isMessagingStopped chan struct{}
	mutex              *sync.Mutex
}

func (d *SimpleComponent) GetName() string {
	return d.Name
}

func (d *SimpleComponent) preInit() {
	d.mutex = &sync.Mutex{}
	d.mmux = make(chan func() (context.Context, interface{}, chan<- error), 1)
}

func (d *SimpleComponent) tearDown() {
	if d.isMessagingStopped != nil {
		close(d.isMessagingStopped)
	}
	close(d.mmux)
}

func (d *SimpleComponent) String() string {
	return d.GetName()
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
	if d.state != s {
		log.Println(d, s)
	}
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

func (d *SimpleComponent) setContainer(c *Container) {
	d.container = c
}

func (d *SimpleComponent) GetContainer() *Container {
	return d.container
}

func (d *SimpleComponent) getMmux() chan func() (context.Context, interface{}, chan<- error) {
	return d.mmux
}

func (d *SimpleComponent) SetInbox(inbox chan func() (context.Context, interface{}, chan<- error)) (<-chan struct{}, error) {
	if inbox == nil {
		return nil, errors.New("SetMessageReader was passed an empty messaging channel")
	}

	d.inbox = inbox

	// close any existing message stop channel before creating one
	if d.isMessagingStopped != nil {
		close(d.isMessagingStopped)
	}
	d.isMessagingStopped = make(chan struct{})
	return d.isMessagingStopped, nil
}

func (d *SimpleComponent) getInbox() chan func() (context.Context, interface{}, chan<- error) {
	return d.inbox
}

func (d *SimpleComponent) Notify(notification func() (context.Context, interface{}, chan<- error)) {
	mMux := d.getMmux()
	if mMux == nil {
		err := errors.New("message mux not initialized for " + d.GetName())
		log.Println("failed to process notification due to", err.Error())
		_, _, errCh := notification()

		defer func() {
			if errCh != nil {
				errCh <- err
				close(errCh)
			}
		}()
		return
	}

	mMux <- notification
}

func (d *SimpleComponent) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("{\"status\":\"ok\"}"))
}
