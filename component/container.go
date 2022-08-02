package component

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

var runOnce sync.Once

type Container struct {
	SimpleComponent
	signalCh chan os.Signal

	// slice to maintain the order of components being added
	components []string

	// channel maintained as a queue for adding & activating Components sequentially.
	// Addition of components is made possible through a dedicated go routine which reads from compActivationQueue channel, thereby unblocking the caller to
	// perform other activities like subscribing to state change notifications or just continuing with remaining execution.
	compActivationQueue chan Component

	cComponents map[string]cComponent
	cHandlers   map[string]func(http.ResponseWriter, *http.Request)
}

func (c *Container) Init(ctx context.Context) error {
	c.cComponents = make(map[string]cComponent)
	c.cHandlers = make(map[string]func(http.ResponseWriter, *http.Request))

	runOnce.Do(func() {
		c.signalCh = make(chan os.Signal, 1)
		signal.Notify(c.signalCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	})
	return nil
}

func (c *Container) Start(ctx context.Context) error {
	for {
		select {
		// proceed to shutdown container if system interrupt is received.
		case osSignal, ok := <-c.signalCh:
			if !ok {
				log.Println("signalCh closed for", c.GetName())
				return nil
			}

			log.Println(c.GetName(), "received", osSignal)

			// TODO: later change it to a context with timeout
			log.Println("proceed to shutdown", c.GetName())
			err := c.Notify(context.TODO(), ControlMsgId, map[MsgClassifierId]interface{}{ControlMsgId: Shutdown}, nil)
			if err == nil {
				log.Println("Shutdown notification was successfully sent to", c.GetName())
				return nil
			}
		case <-c.inbox:
		}
	}
}

func (c *Container) Add(comp Component) error {
	err := validateName(comp)
	if err != nil {
		return err
	}

	if !c.Matches(comp) && comp.GetName() == c.GetName() {
		return fmt.Errorf("Component name could not have same name as container")
	}

	if comp != nil && comp.GetRWLock() == nil {
		log.Fatalf("RW mutex has not been initialized for %s", comp.GetName())
	}

	if c.compActivationQueue == nil && c.GetStage() < Stopping {
		c.compActivationQueue = make(chan Component, 1)

		// Even though components could be added asynchronously, it is activated sequentially since its being read off the compActivationQueue channel, thereby maintaining order.
		go func() {
			for nextComp := range c.compActivationQueue {
				c.componentLifecycleFSM(context.TODO(), nextComp)

				cComm := cComponent{}
				err := c.toCanonical(nextComp, &cComm)
				if err != nil {
					log.Println(nextComp, "failed to convert to canonical type due to", err.Error())
				}

				if _, found := c.cComponents[nextComp.GetName()]; found {
					err := fmt.Errorf("%v already activated", nextComp.GetName())
					log.Println("failed to add component due to", err.Error())
				}
				c.cComponents[nextComp.GetName()] = cComm
				c.GetRWLock().Lock()
				c.components = append(c.components, nextComp.GetName())
				c.GetRWLock().Unlock()
			}
		}()
	}

	c.compActivationQueue <- comp
	return nil
}

// componentLifecycleFSM is a FSM for handling various stages of a component.
func (c *Container) componentLifecycleFSM(ctx context.Context, comp Component) error {
	pc, _, _, ok := runtime.Caller(1)
	details := runtime.FuncForPC(pc)
	if ok && details != nil {
		log.Printf("componentLifecycleFSM() called from %s by %s\n", details.Name(), comp.GetName())
	}
	comp.GetRWLock().Lock()

	log.Printf("componentLifecycleFSM() called from %s by %s in stage: %s \n", details.Name(), comp.GetName(), comp.GetStage())
	log.Printf("componentLifecycleFSM() called from %s by %s in state: %s \n", details.Name(), comp.GetName(), comp.GetState())

	switch comp.GetStage() {
	case Submitted:
		comp.setStage(Preinitializing)
		fallthrough
	case Preinitializing:
		comp.preInit()
		_ = comp.Callback(true, stateChangeCallbacker(comp))
		ctx := context.Background()

		// add default message handler if not added
		if comp.getMessageHandler(comp.GetName()) == nil {
			comp.SetMessageHandler(comp.GetName(), comp.DefaultMessageHandler)
		}

		go c.startMmux(ctx, comp)
		comp.setStage(Preinitialized)
		fallthrough
	case Preinitialized:
		comp.setStage(Initializing)
		ctx := context.Background()
		defer func(comp Component) {
			go c.componentLifecycleFSM(context.TODO(), comp)
		}(comp)

		// TODO: test for init new component from within init of a component
		err := comp.Init(ctx)
		if err != nil {
			comp.setStage(Aborting)
			comp.setStage(Tearingdown)
		} else {
			comp.setStage(Initialized)
		}
	case Initialized, Restarting, Starting:
		comp.setStage(Starting)
		ctx := context.Background()
		go c.startComponent(ctx, comp)
		comp.setStage(Started)
	case Started:
		// do nothing
	case Stopping:
		err := comp.Stop(ctx)
		if err != nil {
			return err
		}
		c.removeHttpHandlers(comp)
		comp.setStage(Stopped)
		fallthrough
	case Stopped:
		fallthrough
	case Tearingdown:
		comp.tearDown()
		comp.setStage(Teareddown)
		c.removeComponent(comp.GetName())
	}

	comp.GetRWLock().Unlock()
	log.Printf("return componentLifecycleFSM() called from %s by %s\n", details.Name(), comp.GetName())

	return nil
}

// startMmux is the main routine which blocks and receives all types of messages which are sent to a component. Components which have been
// preinitialized successfully, would each have its own message handlers started as separate go routines.
func (c *Container) startMmux(ctx context.Context, comp Component) {
	defer func(ctx context.Context, comp Component) {
		if comp.GetState() != Active {
			return
		}
		go c.startMmux(ctx, comp)
	}(ctx, comp)

	for msgFunc := range comp.getMmux() {
		msgCtx, msgClassId, msgClassLookup, msg, errCh := msgFunc()
		msgClass := msgClassLookup[msgClassId]

		switch msgClassId {
		case ControlMsgId:
			switch msgClass {
			//additional control handling cases goes here
			case Shutdown:
				comp.setStage(Stopping)
				err := c.componentLifecycleFSM(msgCtx, comp)
				log.Println("degug,,,", comp.GetName(), "sending error", err)
				errCh <- err
				if err == nil {
					return
				}
			}
		case ComponentMsgId:
			switch msgClass {
			case comp.GetName():
				// check if message is a copy of the same underlying concrete component type
				msgCompType, compConcreteType, msgCompConcreteType := msg.(Component), reflect.TypeOf(comp).Elem(), reflect.TypeOf(msg).Elem()
				if compConcreteType == msgCompConcreteType {
					// validate if etag of the copy is current or not
					if comp.GetEtag() != msgCompType.GetEtag() {
						errCh <- errors.New("component copy is not current due to mismatched etag")
						break
					}
				}
				goto forwardMessage
			default:
				goto forwardMessage
			}

		forwardMessage:
			log.Println("startMmux forwardMessage tag before getting handler for", comp.GetName())
			handler := comp.getMessageHandler(msgClass.(string))
			log.Println("startMmux forwardMessage tag before calling handler for", comp.GetName())
			err := handler(msgCtx, msg)
			log.Println("startMmux forwardMessage tag for", comp.GetName(), "sending back error", err)
			errCh <- err
		}
	}
}

func (c *Container) startComponent(ctx context.Context, comp Component) {
	defer func(ctx context.Context, comp Component) {
		if comp.GetState() != Active {
			return
		}
		comp.setStage(Restarting)
		go c.componentLifecycleFSM(ctx, comp)
	}(ctx, comp)

	err := comp.Start(ctx)
	if err != nil {
		log.Println(comp, "encountered following error while starting.", err)
		isRestartable, delay := comp.IsRestartableWithDelay()
		if !isRestartable {
			log.Println(comp, "configured not to restart")
			return
		}
		if delay <= time.Duration(0) {
			delay = 5 * time.Second
		}
		log.Println(comp, "to restart after", delay)
		time.Sleep(delay)
		return
	}
}

// compare if the passed component matches this container type
func (c *Container) Matches(comp Component) bool {
	containerAddress, compAddress := fmt.Sprintf("%p", Component(c)), fmt.Sprintf("%p", comp)
	return containerAddress == compAddress
}

// toCanonical enhances the component and assigns it to the passed cComponent type
func (c *Container) toCanonical(comp Component, cComp *cComponent) error {
	// assign reference of container to the component only if its not the current container being bootstrapped.
	if !c.Matches(comp) {
		comp.setContainer(c)
	}

	cComp.cName = comp.GetName()
	cComp.comp = comp

	handlers := deriveHttpHandlers(comp)
	if len(handlers) <= 0 {
		goto returnnoerror
	}

	for cURI, httpHandlerFunc := range handlers {
		c.cHandlers[cURI] = httpHandlerFunc
		log.Println("added URI", cURI)
	}

returnnoerror:
	return nil
}

// 1) A container (which is also a component) could be added and maintained within its own data structures to bootstrap itself. 2) It could also be added to a
// parent container and maintained within that container. 3) There ia a rare possibility that the container which is boostraping itself would appear later in
// the LIFO order, than a component it holds.
//
// So would have to factor the afore mentioned cases while "Stopping" a container in order that it has clean and consise logic.
// Note that the method sends Shutdown notification to the encompassed components and relies on removeComponent method to properly update the data structures.
// Both methods maintain clear separation of responsibilities.
func (c *Container) Stop(ctx context.Context) error {
	// c.GetRWLock().Lock()
	// defer c.GetRWLock().Unlock()

	// proceed to stop components in LIFO order
	last := len(c.components) - 1

	for last >= 0 {
		cName := c.components[last]
		cComp, found := c.cComponents[cName]

		if !found || cComp.comp == nil {
			log.Println(cName, "component no longer found within container", c.GetName())
		} else if !c.Matches(cComp.comp) {
			log.Println("sending", Shutdown, "signal to", cName)

			err := cComp.comp.Notify(context.TODO(), ControlMsgId, map[MsgClassifierId]interface{}{ControlMsgId: Shutdown}, nil)
			if err != nil {
				return err
			}
		}

		// return if the last remaining component is the same container as this. The following check would be sufficient to cover cases 1 & 3.
		// For case 3 scenario, last would != 0, thereby it skips the bootstrapped container and continues with shutting down all other components
		// until the skipped container is the only component left. At that point last would be == 0, and the check would be true (which matches case 1)
		if last == 0 && c.Matches(cComp.comp) {
			return nil
		}

		last = len(c.components) - 1
	}

	defer func() {
		if c.compActivationQueue != nil {
			close(c.compActivationQueue)
			c.compActivationQueue = nil
		}

		if c.signalCh != nil {
			signal.Stop(c.signalCh)
			close(c.signalCh)
			c.signalCh = nil
		}
	}()

	return nil
}

// GetComponent returns a copy of the component within a container. Only exported field values would be copied over. Advisable not to mark pointer
// fields within a component as an exported field. Reference types such as slice, map, channel, interface, and function types which are exported would
// be copied over.
func (c *Container) GetComponent(name string) (Component, error) {
	c.GetRWLock().RLock()
	defer c.GetRWLock().RUnlock()

	var (
		cComp cComponent
		found bool
	)

	if cComp, found = c.cComponents[name]; !found {
		return nil, fmt.Errorf("unable to find component %v within %v", name, c.GetName())
	}

	comp := cComp.comp
	if comp == nil {
		return nil, errors.New("unable to find component type within cComponent")
	}

	return getComponentCopy(comp)
}

// GetHttpHandler returns the longest matching URI prefix handler
func (c *Container) GetHttpHandler(URI string) func(w http.ResponseWriter, r *http.Request) {
	var httpHandler func(w http.ResponseWriter, r *http.Request)

	for len(URI) > 0 {
		httpHandler = c.cHandlers[URI]
		if httpHandler != nil {
			return httpHandler
		}
		lastSeparatorIndx := strings.LastIndex(URI, "/")
		URI = URI[:lastSeparatorIndx]
	}
	return nil
}

func (c *Container) removeHttpHandlers(comp Component) {
	// obtain all the handlers for the component
	handlers := deriveHttpHandlers(comp)
	if len(handlers) <= 0 {
		log.Println("unable to find any http handlers to remove for", comp.GetName())
	}

	for cURI, _ := range handlers {
		delete(c.cHandlers, cURI)
		log.Println("removed URI", cURI)
	}
}

// removeComponent removes a component from a container in the event its being shutdown
func (c *Container) removeComponent(name string) {
	c.GetRWLock().Lock()
	defer c.GetRWLock().Unlock()

	delete(c.cComponents, name)

	indx, found := len(c.components)-1, false
	for ; indx >= 0; indx-- {
		if c.components[indx] == name {
			found = true
			break
		}
	}

	if !found {
		log.Println("unable to find", name, "to remove, within", c.GetName())
		return
	}

	// removing component name from silce at indx
	c.components = append(c.components[:indx], c.components[indx+1:]...)
}

func validateName(comp Component) error {
	if comp == nil {
		return fmt.Errorf("passed nil Component reference")
	}

	if len(comp.GetName()) <= 0 {
		return fmt.Errorf("Component name cannot be empty")
	}

	// validation for allowed unreserved charaters https://datatracker.ietf.org/doc/html/rfc3986#section-2.3
	allowedChars := ".a-zA-Z0-9~_-"
	foundInvalidChars, err := regexp.MatchString("[^\\"+allowedChars+"]+", comp.GetName())
	if err != nil {
		return nil
	}

	if foundInvalidChars {
		return fmt.Errorf("%s contains invalid characters. Allowed characters are: %s", comp.GetName(), allowedChars)
	}

	return nil
}

func stateChangeCallbacker(comp Component) func(context.Context, int, interface{}) {
	return func(ctx context.Context, cbIndx int, notification interface{}) {
		switch notification {
		case Active, Inactive:
			log.Println("proceeding to set new ETag for", comp.GetName())
			SetComponentEtag(comp)
		case Stopping:
			log.Println("stateChangeCallbacker: case Stopping")
			err := comp.RemoveCallback(cbIndx)
			if err != nil {
				log.Println(err)
			}
		default:
		}
	}
}

func deriveHttpHandlers(comp Component) map[string]func(w http.ResponseWriter, r *http.Request) {
	// proceeding to assign URI for the component. for that we would need to travserse all the way to top container.
	rootC := comp.GetContainer()
	paths := []string{}
	for rootC != nil {
		paths = append(paths, rootC.GetName())
		rootC = rootC.GetContainer()
	}

	cURI := "/"
	if len(paths) > 0 {
		for i := len(paths) - 1; i >= 0; i-- {
			cURI += paths[i] + "/"
		}
	}
	cURI += comp.GetName()

	cType := reflect.TypeOf(comp)
	cTypeVal := reflect.ValueOf(comp)

	handlers := map[string]func(w http.ResponseWriter, r *http.Request){}

	//derive the http routes within the component
	for i := 0; i < cTypeVal.NumMethod(); i++ {
		methodVal := cTypeVal.Method(i)

		// additionally check for any exported methods matching http handler func signature
		if httpHandlerFunc, ok := methodVal.Interface().(func(http.ResponseWriter, *http.Request)); ok {
			methodName := cType.Method(i).Name
			// ServeHTTP becomes the default URI to the component
			if methodName == "ServeHTTP" {
				handlers[cURI] = httpHandlerFunc
				// set the component URI
				comp.setURI(cURI)
			} else {
				handlerURI := cURI + "/" + methodName
				handlers[handlerURI] = httpHandlerFunc
			}
		}
	}

	return handlers
}

// canonical Component which enhances the component
type cComponent struct {
	cName string
	comp  Component
}
