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
	Inactive state = iota
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

	// Init method should include logic which finishes with reasonable amount of time, since it blocks initializing other components for the application
	// unless this has finished executing. Exceptions to this could be infra components which handle database or messaging requirements, which other
	// components might be dependent for proper functioning.
	Init(context.Context) error
	Start(context.Context) error
	Stop(context.Context) error

	// Notify could be used as a primary means to asynchronously send any type of message to a component, wrapped as a function. Once the message is processed
	// by the receiving component, proceed to close the error channel within the receiving component, to indicate to the sender that it had finished processing
	// the message. In event of any error, the receiving component could pass along the error on the error channel before closing the channel.
	Notify(func() (context.Context, interface{}, chan<- error))

	// Subscribers could pass a channel to receive state/stage notifications of components it might be interested
	Subscribe(subscriber string, subscriberCh chan<- interface{}) error

	// A component could assign a channel by which it could receive contexed messages/notifications wrapped as a function. The component would continue to
	// receive messages until the returned notification channel is closed.
	SetInbox(chan func() (context.Context, interface{}, chan<- error)) (<-chan struct{}, error)
	getInbox() chan func() (context.Context, interface{}, chan<- error)

	getMmux() chan func() (context.Context, interface{}, chan<- error)

	// IsRestartableWithDelay indicates if component is to be restarted if Start() fails with error. The method could include logic for exponential backoff
	// to return the delay duration between restarts.
	IsRestartableWithDelay() (bool, time.Duration)

	setContainer(*Container)
	GetContainer() *Container

	GetLock() *sync.Mutex

	setHash(uint64)
	Hash() uint64

	setEtag(string)
	GetEtag() string

	fmt.Stringer
	http.Handler
}

type SimpleComponent struct {
	Etag string
	hash uint64

	Name      string
	container *Container
	stage     stage
	state     state

	// message mux
	mmux chan func() (context.Context, interface{}, chan<- error)

	inbox              chan func() (context.Context, interface{}, chan<- error)
	isMessagingStopped chan struct{}
	mutex              *sync.Mutex

	subscribers map[string]chan<- interface{}
}

func (d *SimpleComponent) GetName() string {
	return d.Name
}

func (d *SimpleComponent) preInit() {
	d.mutex = &sync.Mutex{}
	d.mmux = make(chan func() (context.Context, interface{}, chan<- error), 1)
}

func (d *SimpleComponent) tearDown() {
	d.GetLock().Lock()
	defer d.GetLock().Unlock()

	if d.isMessagingStopped != nil {
		close(d.isMessagingStopped)
	}

	if d.mmux != nil {
		close(d.mmux)
		d.mmux = nil
	}
}

func (d *SimpleComponent) String() string {
	return d.GetName()
}

func (d *SimpleComponent) setStage(s stage) {
	log.Println(d, s)
	d.stage = s
	d.notifySubscribers(s)

	if d.stage >= Initialized && d.stage <= Started {
		d.setState(Active)
	} else {
		d.setState(Inactive)
	}
}

func (d *SimpleComponent) State() state {
	return d.state
}

func (d *SimpleComponent) setState(s state) {
	if d.state != s {
		log.Println(d, s)
		d.notifySubscribers(s)
	}
	d.state = s
}

func (d *SimpleComponent) Stage() stage {
	return d.stage
}

func (d *SimpleComponent) setHash(hash uint64) {
	d.hash = hash
}

func (d *SimpleComponent) Hash() uint64 {
	return d.hash
}

func (d *SimpleComponent) setEtag(etag string) {
	d.Etag = etag
}

func (d *SimpleComponent) GetEtag() string {
	return d.Etag
}

func (d *SimpleComponent) Subscribe(subscriber string, subscriberCh chan<- interface{}) error {
	if _, found := d.subscribers[subscriber]; found {
		return errors.New("found " + subscriber + " already subscribed to " + d.GetName())
	}
	if d.subscribers == nil {
		d.subscribers = make(map[string]chan<- interface{})
	}
	d.subscribers[subscriber] = subscriberCh
	log.Println(subscriber, "successfully subscribed to", d.GetName())
	return nil
}

func (d *SimpleComponent) notifySubscribers(notification interface{}) {
	for _, subsCh := range d.subscribers {
		subsCh <- notification
	}
}

// components which use SimpleComponent as embedded type could override this method to have custom implementation.
// Refer proper guidelines for implementing the method within Component interface.
func (d *SimpleComponent) Init(context.Context) error { return nil }

// components which use SimpleComponent as embedded type could override this method to have custom implementation.
// Refer proper guidelines for implementing the method within Component interface.
func (d *SimpleComponent) Start(context.Context) error {
	inbox := d.getInbox()
	if inbox == nil {
		return nil
	}

	for msgFunc := range inbox {
		// process msgFunc
		_, msg, _ := msgFunc()
		fmt.Println(msg)
	}

	return nil
}

// components which use SimpleComponent as embedded type could override this method to have custom implementation.
// Refer proper guidelines for implementing the method within Component interface.
func (d *SimpleComponent) IsRestartableWithDelay() (bool, time.Duration) {
	return true, 3 * time.Second
}

// components which use SimpleComponent as embedded type could override this method to have custom implementation.
// Refer proper guidelines for implementing the method within Component interface.
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

func (d *SimpleComponent) GetLock() *sync.Mutex {
	return d.mutex
}

func (d *SimpleComponent) Notify(notification func() (context.Context, interface{}, chan<- error)) {
	d.GetLock().Lock()
	defer d.GetLock().Unlock()

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
