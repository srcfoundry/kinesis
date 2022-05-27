package component

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mitchellh/hashstructure/v2"
	"github.com/mohae/deepcopy"
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

	setURI(string)
	GetURI() string

	setStage(stage)
	GetStage() stage

	setState(state)
	GetState() state

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

	// Callback could be used to register a callback function to receive state/stage notifications from a component. All registered callback functions would be
	// maintained within a function slice. Any time a callback function is registered with isHead = false would get appended to end of the function slice. While
	// a registration made with isHead = true, would result in the callback function getting added to index 0 (head) of the function slice and shifting any existing
	// functions by 1 index. Repeated adds with isHead = true would be like adding to head of a LIFO stack.
	//
	// Note that the registered callback functions would be sent notifications prior to any subscribers, by iterating across the function slice and executing each
	// function sequentially with a timeout conveyed within the context parameter. function slice index is also passed as a parameter which could be used to
	// de-register the callback function.
	// Callback functions could be registered in this manner, thereby maintaining order of execution to process a notification across components.
	Callback(isHead bool, callback func(ctx context.Context, cbIndx int, notification interface{})) error

	RemoveCallback(cbIndx int) error

	// Subscribers could pass a channel to receive state/stage notifications of components it might be interested.
	// Note that the callback functions registered using the Callback method would be sent notifications prior to any subscribers.
	Subscribe(subscriber string, subscriberCh chan<- interface{}) error

	Unsubscribe(subscriber string) error

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

	GetRWLock() *sync.RWMutex

	setHash(uint64)
	Hash() uint64

	setEtag(string)
	GetEtag() string

	fmt.Stringer
	http.Handler
}

type SimpleComponent struct {
	Etag string `json:"etag" hash:"ignore"`
	hash uint64

	Name      string `json:"name"`
	uri       string
	container *Container
	Stage     stage `json:"stage"`
	State     state `json:"state"`

	// message mux
	mmux chan func() (context.Context, interface{}, chan<- error)

	inbox              chan func() (context.Context, interface{}, chan<- error)
	isMessagingStopped chan struct{}
	RWMutex            *sync.RWMutex `json:"-" hash:"ignore"`

	subscribers map[string]chan<- interface{}
	callbacks   []func(context.Context, int, interface{})
}

func (d *SimpleComponent) GetName() string {
	return d.Name
}

func (d *SimpleComponent) setURI(uri string) {
	d.uri = uri
}

func (d *SimpleComponent) GetURI() string {
	return d.uri
}

func (d *SimpleComponent) preInit() {
	d.mmux = make(chan func() (context.Context, interface{}, chan<- error), 1)
}

func (d *SimpleComponent) tearDown() {
	if d.isMessagingStopped != nil {
		close(d.isMessagingStopped)
	}

	if d.mmux != nil {
		close(d.mmux)
		d.mmux = nil
	}

	if len(d.callbacks) > 0 {
		for cbIndx, _ := range d.callbacks {
			d.RemoveCallback(cbIndx)
		}
	}

	if len(d.subscribers) > 0 {
		for subscriber, _ := range d.subscribers {
			d.Unsubscribe(subscriber)
		}
	}
}

func (d *SimpleComponent) String() string {
	return d.GetName()
}

func (d *SimpleComponent) setStage(s stage) {
	log.Println(d, s)
	d.Stage = s
	d.invokeCallbacks(s)
	d.notifySubscribers(s)

	if d.Stage >= Initialized && d.Stage <= Started {
		d.setState(Active)
	} else {
		d.setState(Inactive)
	}
}

func (d *SimpleComponent) GetState() state {
	return d.State
}

func (d *SimpleComponent) setState(s state) {
	prevState := d.State
	d.State = s

	if d.State != prevState {
		log.Println(d, d.State)
		d.invokeCallbacks(d.State)
		d.notifySubscribers(d.State)
	}
}

func (d *SimpleComponent) GetStage() stage {
	return d.Stage
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

func (d *SimpleComponent) Callback(isHead bool, callback func(ctx context.Context, cbIndx int, notification interface{})) error {
	d.GetRWLock().Lock()
	defer d.GetRWLock().Unlock()

	if d.Stage > Started {
		return fmt.Errorf("unable to add callback since %v is %v", d.GetName(), d.Stage)
	}

	if isHead {
		d.callbacks = append([]func(ctx context.Context, cbIndx int, notification interface{}){callback}, d.callbacks...)
	} else {
		d.callbacks = append(d.callbacks, callback)
	}
	return nil
}

func (d *SimpleComponent) RemoveCallback(cbIndx int) error {
	d.GetRWLock().Lock()
	defer d.GetRWLock().Unlock()

	if cbIndx >= len(d.callbacks) {
		return fmt.Errorf("unable to find callback function at index %v within %v", cbIndx, d.GetName())
	}

	d.callbacks = append(d.callbacks[:cbIndx], d.callbacks[cbIndx+1:]...)
	log.Println(fmt.Sprintf("successfully removed callback function at index %v within %v", cbIndx, d.GetName()))
	return nil
}

func (d *SimpleComponent) Subscribe(subscriber string, subscriberCh chan<- interface{}) error {
	d.GetRWLock().Lock()
	defer d.GetRWLock().Unlock()

	if d.Stage > Started {
		return fmt.Errorf("unable to subscribe since %v is %v", d.GetName(), d.Stage)
	}

	if d.subscribers == nil {
		d.subscribers = make(map[string]chan<- interface{})
	}

	if _, found := d.subscribers[subscriber]; found {
		return fmt.Errorf("%v already subscribed to %v", subscriber, d.GetName())
	}

	d.subscribers[subscriber] = subscriberCh
	log.Println(subscriber, "successfully subscribed to", d.GetName())
	return nil
}

func (d *SimpleComponent) Unsubscribe(subscriber string) error {
	d.GetRWLock().Lock()
	defer d.GetRWLock().Unlock()

	if d.subscribers == nil {
		return fmt.Errorf("unable to find %v subscribed to %v", subscriber, d.GetName())
	}

	if _, found := d.subscribers[subscriber]; !found {
		return fmt.Errorf("unable to find %v subscribed to %v", subscriber, d.GetName())
	}

	delete(d.subscribers, subscriber)
	log.Println(subscriber, "successfully unsubscribed from", d.GetName())
	return nil
}

func (d *SimpleComponent) invokeCallbacks(notification interface{}) {
	if len(d.callbacks) <= 0 {
		return
	}

	// make a copy of callback functions and iterate over the copied function slice, in case the callback function removes itself from the callback function slice
	d.GetRWLock().RLock()
	callbackFuncs := make([]func(context.Context, int, interface{}), len(d.callbacks))
	copy(callbackFuncs, d.callbacks)
	d.GetRWLock().RUnlock()

	for cbIndx, callbackFunc := range callbackFuncs {
		// each callback is executed with a max timeout of 2 seconds
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)

		go func() {
			callbackFunc(ctx, cbIndx, notification)
			cancel()
		}()

		// once context is cancelled, check ctx.Err() to see if context got cancelled due to executing cancel() after the callback or due to context timeout.
		// Hint from https://stackoverflow.com/a/52799874/8952645
		<-ctx.Done()
		switch ctx.Err() {
		case context.DeadlineExceeded:
			log.Println(d.GetName(), "callback at index", cbIndx, "exceeded deadline")
		case context.Canceled:
			//log.Println(d.GetName(), "callback at index", cbIndx, "executed successfully")
		}
	}
}

func (d *SimpleComponent) notifySubscribers(notification interface{}) {
	d.GetRWLock().RLock()
	defer d.GetRWLock().RUnlock()

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
		return nil, fmt.Errorf("%v SetInbox was passed an empty messaging channel", d.GetName())
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

func (d *SimpleComponent) GetRWLock() *sync.RWMutex {
	return d.RWMutex
}

func (d *SimpleComponent) Notify(notification func() (context.Context, interface{}, chan<- error)) {
	d.GetRWLock().RLock()
	defer d.GetRWLock().RUnlock()

	mMux := d.getMmux()
	if mMux == nil {
		err := fmt.Errorf("message mux not initialized for %v", d.GetName())
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
	var comp Component

	// if requested URI path suffix does not match component URI, then return http.StatusNotFound
	if r.URL.Path != d.GetURI() {
		http.NotFound(w, r)
		return
	}

	container := d.GetContainer()
	if container != nil {
		comp, _ = container.GetComponent(d.GetName())
	} else {
		comp, _ = getComponentCopy(d)
	}

	MarshallToHttpResponseWriter(w, comp)
}

// SetComponentEtag calculates the component hash and sets an Entity Tag(ETag) to indicate a version of the component.
// this function is not thread safe. caller should ensure that the function is called within a critical section or
// call hiearchy traces back to one.
func SetComponentEtag(comp Component) error {
	// compute hash of the component object and compare with existing hash
	newHash, err := hashstructure.Hash(comp, hashstructure.FormatV2, nil)
	if err != nil {
		return err
	}

	// assign new etag if component instance hash has changed. this is required in order to resolve conflicts arising
	// due to concurrent updates through messages.
	if newHash != comp.Hash() {
		comp.setHash(newHash)
		etag := uuid.New()
		comp.setEtag(etag.String())
	}
	return nil
}

// getComponentCopy returns copy of component passed. this function is not thread safe. caller should ensure that the function
// is called within a critical section or call hiearchy traces back to one.
func getComponentCopy(comp Component) (Component, error) {
	SetComponentEtag(comp)
	cCopy := deepcopy.Copy(comp)
	return cCopy.(Component), nil
}

func MarshallToHttpResponseWriter(w http.ResponseWriter, comp Component) {
	if comp == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", comp.GetEtag())

	compBytes, err := json.MarshalIndent(comp, "", "  ")
	if err != nil {
		compBytes = []byte(`{"status":"undefined"}"`)
	}
	w.Write(compBytes)
}
