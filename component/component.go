package component

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/mitchellh/hashstructure/v2"
	"github.com/mohae/deepcopy"
	"go.uber.org/zap"
)

// MsgType could be used by Components' to define its own set of message types. Used in conjunction with a lookup map in determining the
// message classification associated to the MsgType, and invoking the appropriate handler function registered to process the message type.
type MsgType string

const (
	ControlMsgType   MsgType = "ControlMsgType"   // ControlMsgType to classify controlMsg
	ComponentMsgType MsgType = "ComponentMsgType" // ComponentMsgType to classify any Component type being passed
)

type ContextType struct{}

var (
	TraceID ContextType
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
	NotifyPeers
	Persist
	Restart
	RestartAfter
	RestartMmux
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

	// components which are containers, that embed Container type, should make use of PostInit to initialize aspects specific to it. Same limitations that
	// apply to the Init method, in terms of what all logic could be included, would be applicable to PostInit method as well.
	PostInit(context.Context) error

	// The PreStart stage is suitable for components that initiate other components, as it occurs after all prerequisites, such as converting to canonical form
	// and container assignment, have been completed.
	PreStart(context.Context) error

	Start(context.Context) error
	Stop(context.Context) error

	// SendSyncMessage is a blocking call for sending any type of message/s to a component.
	// msgType could be used by Components' to define its own set of message types. Used in conjunction with msgsLookup in determining the
	// message type associated to the MsgType, and invoking the appropriate handler function registered to process the message type.
	// If handler functions are not registered for specific message types, all messages would be forwarded to the DefaultSyncMessageHandler.
	// Message types could be batched as a slice to process the messages in-order, thereby facilitating atomic transaction processing.
	//
	// Changes made to component fields outside of expected channels or methods could be persisted (if persistence addon is enabled), by sending
	// a "ControlMsgType:Persist" message. In the same way out-of-band component changes could be notified to subscribers by sending "ControlMsgType:NotifyPeers" message
	SendSyncMessage(timeout time.Duration, msgType interface{}, msgsLookup map[interface{}]interface{}) error

	// To set blocking message handler functions for any message types. Components could define its own message types.
	SetSyncMessageHandler(msgType interface{}, msgTypeHandler func(context.Context, interface{}) error)
	getSyncMessageHandler(msgType interface{}) func(context.Context, interface{}) error

	// DefaultSyncMessageHandler is a blocking call which synchronously handles all messages except ControMsgType types. Messages gets routed by invoking SendSyncMessage.
	DefaultSyncMessageHandler(context.Context, interface{}) error

	// Callback could be used to register a callback function to receive ChangeObject notifications from a component. Notifications could be State, Stage or immutable Component
	// copies. All registered callback functions would be maintained within a function slice. Any time a callback function is registered with isHead = false would get appended
	// to end of the function slice. While a registration made with isHead = true, would result in the callback function getting added to index 0 (head) of the function slice
	// and shifting any existing functions by 1 index. Repeated adds with isHead = true would be like adding to head of a LIFO stack.
	//
	// Note that the registered callback functions would be sent notifications prior to any subscribers, by iterating across the function slice and executing each
	// function sequentially with a timeout conveyed within the context parameter. function slice index is also passed as a parameter which could be used to
	// de-register the callback function.
	// Callback functions could be registered in this manner, thereby maintaining order of execution to process a notification across components.
	Callback(isHead bool, callback func(ctx context.Context, cbIndx int, changeObject ChangeObject)) error
	RemoveCallback(cbIndx int) error
	invokeCallbacks(changeObject ChangeObject)

	// Subscribers could pass a channel to receive ChangeObject notifications of components it might be interested. Notifications could be State, Stage or immutable Component
	// copies.
	//
	// Note that the callback functions registered using the Callback method would be sent notifications prior to any subscribers.
	Subscribe(subscriber string, subscriberCh chan<- ChangeObject) error
	Unsubscribe(subscriber string) error
	notifySubscribers(notification ChangeObject)

	getMmux() chan func() (ctx context.Context, msgType interface{}, msgsLookup map[interface{}]interface{}, errCh chan<- error)

	// IsRestartableWithDelay indicates if component is to be restarted if Start() fails with error. The method could include logic for exponential backoff
	// to return the delay duration between restarts.
	IsRestartableWithDelay() (bool, time.Duration)

	setContainer(*Container)
	GetContainer() *Container

	// lock to acquire prior to component mutation
	getMutatingLock() *sync.RWMutex

	// lock to acquire prior to component stage/state transition
	getstateTransitionLock() *sync.RWMutex

	setHash(uint64)
	Hash() uint64

	setEtag(string)
	GetEtag() string

	setPersistanceCache([]byte)
	getPersistanceCache() []byte

	setComponentCache(Component)
	getComponentCache() Component

	fmt.Stringer
	http.Handler

	SetAsNonRestEntity(bool)
	IsNonRestEntity() bool

	GetLogger() *zap.Logger
	setLogger(*zap.Logger)
}

type SimpleComponent struct {
	Etag string `json:"etag" hash:"ignore"`
	hash uint64
	// type cache maintained for persistence
	persistanceCache []byte
	// component cache maintained for component message processing
	componentCache Component

	Name            string `json:"name"`
	uri             string
	isNonRestEntity bool
	container       *Container
	Stage           stage `json:"stage"`
	State           state `json:"state"`

	// message mux
	mmux            chan func() (ctx context.Context, msgType interface{}, msgsLookup map[interface{}]interface{}, errCh chan<- error)
	messageHandlers map[interface{}]func(context.Context, interface{}) error

	isMessagingStopped chan struct{}
	// mutatingLock maintained as generic type to prevent direct access to RWMutex and to use the getMutatingLock() method instead
	mutatingLock interface{} `json:"-" hash:"ignore"`
	// simple type to enforce atomic access within functions.primarily used in getMutatingLock function
	mutatingAtomicCAS atomic.Bool `json:"-" hash:"ignore"`

	// stateTransitionLock maintained as generic type to prevent direct access to RWMutex and to use the getStateTransitionLock() method instead
	stateTransitionLock interface{} `json:"-" hash:"ignore"`
	// simple type to enforce atomic access within functions.primarily used in getStateTransitionLock function
	stateAtomicCAS atomic.Bool `json:"-" hash:"ignore"`

	subscribers map[string]chan<- ChangeObject
	callbacks   []func(context.Context, int, ChangeObject)

	logger *zap.Logger
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

func (d *SimpleComponent) setPersistanceCache(persistType []byte) {
	d.persistanceCache = persistType
}

func (d *SimpleComponent) getPersistanceCache() []byte {
	return d.persistanceCache
}

func (d *SimpleComponent) setComponentCache(compCopy Component) {
	d.componentCache = compCopy
}

func (d *SimpleComponent) getComponentCache() Component {
	return d.componentCache

}

func (d *SimpleComponent) preInit() {
	d.mmux = make(chan func() (ctx context.Context, msgType interface{}, msgsLookup map[interface{}]interface{}, errCh chan<- error), 1)
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
	d.getstateTransitionLock().Lock()
	stageChange := StageChangeObject{}
	stageChange.prevObj, stageChange.currObj = d.Stage, s
	d.GetLogger().Debug("stage transition", zap.Any("stage", stageChange))
	d.Stage = s
	d.getstateTransitionLock().Unlock()

	d.invokeCallbacks(stageChange)
	d.notifySubscribers(stageChange)

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
		stateChange := StateChangeObject{}
		stateChange.prevObj, stateChange.currObj = prevState, s
		d.GetLogger().Info("state transition", zap.Any("state", stateChange))
		d.invokeCallbacks(stateChange)
		d.notifySubscribers(stateChange)
	}
}

func (d *SimpleComponent) GetStage() stage {
	d.getstateTransitionLock().RLock()
	defer d.getstateTransitionLock().RUnlock()
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

func (d *SimpleComponent) SetAsNonRestEntity(set bool) {
	d.isNonRestEntity = set
}

func (d *SimpleComponent) IsNonRestEntity() bool {
	return d.isNonRestEntity
}

func (d *SimpleComponent) Callback(isHead bool, callback func(ctx context.Context, cbIndx int, changeObj ChangeObject)) error {
	if d.Stage > Started {
		return fmt.Errorf("unable to add callback since %v is %v", d.GetName(), d.Stage)
	}

	if isHead {
		d.callbacks = append([]func(ctx context.Context, cbIndx int, change ChangeObject){callback}, d.callbacks...)
	} else {
		d.callbacks = append(d.callbacks, callback)
	}
	return nil
}

func (d *SimpleComponent) RemoveCallback(cbIndx int) error {
	if cbIndx >= len(d.callbacks) {
		return fmt.Errorf("unable to find callback function at index %v within %v", cbIndx, d.GetName())
	}

	d.callbacks = append(d.callbacks[:cbIndx], d.callbacks[cbIndx+1:]...)
	d.GetLogger().Debug("removed callback function", zap.Int("index", cbIndx))
	return nil
}

func (d *SimpleComponent) Subscribe(subscriber string, subscriberCh chan<- ChangeObject) error {
	if d.subscribers == nil {
		d.subscribers = make(map[string]chan<- ChangeObject)
	}

	if _, found := d.subscribers[subscriber]; found {
		return fmt.Errorf("%v already subscribed to %v", subscriber, d.GetName())
	}

	d.subscribers[subscriber] = subscriberCh
	d.GetLogger().Info("subscription", zap.String("subscriber", subscriber))
	return nil
}

func (d *SimpleComponent) Unsubscribe(subscriber string) error {
	if d.subscribers == nil {
		return fmt.Errorf("unable to find %v subscribed to %v", subscriber, d.GetName())
	}

	if _, found := d.subscribers[subscriber]; !found {
		return fmt.Errorf("unable to find %v subscribed to %v", subscriber, d.GetName())
	}

	delete(d.subscribers, subscriber)
	d.GetLogger().Info("unsubscription", zap.String("subscriber", subscriber))
	return nil
}

func (d *SimpleComponent) invokeCallbacks(changeObject ChangeObject) {
	if len(d.callbacks) <= 0 {
		return
	}

	// make a copy of callback functions and iterate over the copied function slice, in case the callback function removes itself from the callback function slice
	callbackFuncs := make([]func(context.Context, int, ChangeObject), len(d.callbacks))
	copy(callbackFuncs, d.callbacks)

	for cbIndx, callbackFunc := range callbackFuncs {
		// each callback is executed with a max timeout of 2 seconds
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)

		go func() {
			callbackFunc(ctx, cbIndx, changeObject)
			cancel()
		}()

		// once context is cancelled, check ctx.Err() to see if context got cancelled due to executing cancel() after the callback or due to context timeout.
		// Hint from https://stackoverflow.com/a/52799874/8952645
		<-ctx.Done()
		switch ctx.Err() {
		case context.DeadlineExceeded:
			d.GetLogger().Info("callback exceeded notification deadline", zap.Int("index", cbIndx), zap.Any("notification", changeObject))
		case context.Canceled:
			d.GetLogger().Debug("callback executed", zap.Int("index", cbIndx), zap.Any("notification", changeObject))
		}
	}
}

func (d *SimpleComponent) notifySubscribers(notification ChangeObject) {
	for _, subsCh := range d.subscribers {
		subsCh <- notification
	}
}

// components which use SimpleComponent as embedded type could override this method to have custom implementation.
// Refer notes within Component interface for implementing the method.
func (d *SimpleComponent) Init(context.Context) error { return nil }

// components which use Container as embedded type could override this method to have custom implementation.
// Refer notes within Component interface for implementing the method.
func (d *SimpleComponent) PostInit(context.Context) error { return nil }

// components that initiate other components could override this method to have custom implementation.
// Refer notes within Component interface for implementing the method.
func (d *SimpleComponent) PreStart(context.Context) error { return nil }

// components which use SimpleComponent as embedded type could override this method to have custom implementation.
// Refer notes within Component interface for implementing the method.
func (d *SimpleComponent) Start(context.Context) error { return nil }

// components which use SimpleComponent as embedded type could override this method to have custom implementation.
// Refer notes within Component interface for implementing the method.
func (d *SimpleComponent) IsRestartableWithDelay() (bool, time.Duration) {
	return false, 0 * time.Second
}

// components which use SimpleComponent as embedded type could override this method to have custom implementation.
// Refer notes within Component interface for implementing the method.
func (d *SimpleComponent) Stop(context.Context) error { return nil }

func (d *SimpleComponent) setContainer(c *Container) {
	d.getMutatingLock().Lock()
	defer d.getMutatingLock().Unlock()
	d.container = c
}

func (d *SimpleComponent) GetContainer() *Container {
	return d.container
}

func (d *SimpleComponent) getMmux() chan func() (ctx context.Context, msgType interface{}, msgsLookup map[interface{}]interface{}, errCh chan<- error) {
	return d.mmux
}

func (d *SimpleComponent) SetSyncMessageHandler(msgType interface{}, msgTypeHandler func(context.Context, interface{}) error) {
	if d.messageHandlers == nil {
		d.messageHandlers = map[interface{}]func(context.Context, interface{}) error{}
	}
	d.messageHandlers[msgType] = msgTypeHandler
}

func (d *SimpleComponent) getSyncMessageHandler(msgType interface{}) func(context.Context, interface{}) error {
	if d.messageHandlers == nil {
		return nil
	}
	return d.messageHandlers[msgType]
}

func (d *SimpleComponent) DefaultSyncMessageHandler(context.Context, interface{}) error {
	return nil
}

func (d *SimpleComponent) getMutatingLock() *sync.RWMutex {
	d.mutatingAtomicCAS.CompareAndSwap(d.mutatingAtomicCAS.Load(), !d.mutatingAtomicCAS.Load())
	if d.mutatingLock == nil {
		d.mutatingLock = &sync.RWMutex{}
	}
	return d.mutatingLock.(*sync.RWMutex)
}

func (d *SimpleComponent) getstateTransitionLock() *sync.RWMutex {
	d.stateAtomicCAS.CompareAndSwap(d.stateAtomicCAS.Load(), !d.stateAtomicCAS.Load())
	if d.stateTransitionLock == nil {
		d.stateTransitionLock = &sync.RWMutex{}
	}
	return d.stateTransitionLock.(*sync.RWMutex)
}

func (d *SimpleComponent) SendSyncMessage(timeout time.Duration, msgType interface{}, msgsLookup map[interface{}]interface{}) error {
	if d.GetStage() >= Stopping {
		return fmt.Errorf("cannot process message since %s is %s", d.GetName(), d.GetStage())
	}

	const retryUnits = 100
	retryWait := timeout / retryUnits
	var mMux chan func() (ctx context.Context, msgType interface{}, msgsLookup map[interface{}]interface{}, errCh chan<- error)

	// wait each retryWait until mmux has been initialized or max retries have been reached
	for retries := 0; retries <= retryUnits; retries++ {
		if mMux = d.getMmux(); mMux == nil {
			time.Sleep(retryWait)
		} else {
			// calculate remaining timeout after the retries if any
			timeout -= (time.Duration(retries) * retryWait)
			break
		}
	}

	errCh := make(chan error)
	defer close(errCh)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	mMux <- func() (context.Context, interface{}, map[interface{}]interface{}, chan<- error) {
		return ctx, msgType, msgsLookup, errCh
	}

	var err error

	select {
	case <-time.After(75 * time.Second):
		err = errors.New("notification max timeout")
	case <-ctx.Done():
		err = ctx.Err()
	case err = <-errCh:
	}

	return err
}

func (d *SimpleComponent) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var comp Component

	// if requested URI path prefix does not match component URI, then return http.StatusNotFound
	if r.URL.Path != d.GetURI() {
		http.NotFound(w, r)
		return
	}

	container := d.GetContainer()
	if container != nil {
		comp, _ = container.GetComponent(d.GetName())
	} else {
		comp, _ = createCopy(d)
	}

	MarshallToHttpResponseWriter(w, comp)
}

func (d *SimpleComponent) GetLogger() *zap.Logger {
	if d.logger == nil {
		return logger
	}
	return d.logger
}

func (d *SimpleComponent) setLogger(logger *zap.Logger) {
	d.logger = logger
}

// UpdateComponentEtag calculates the component hash and sets an Entity Tag(ETag) to indicate a version of the component.
// this function is not thread safe. caller should ensure that the function is called within a critical section or
// call hiearchy traces back to one.
func UpdateComponentEtag(comp Component) error {
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

// createCopy returns copy of component passed. this function is not thread safe. caller should ensure that the function
// is called within a critical section or call hierarchy traces back to one.
func createCopy(comp Component) (Component, error) {
	UpdateComponentEtag(comp)
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
