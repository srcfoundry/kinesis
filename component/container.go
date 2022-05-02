package component

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/mitchellh/hashstructure/v2"
)

var runOnce sync.Once

type Container struct {
	SimpleComponent
	signalCh chan os.Signal

	// slice to maintain the order of components being added
	components []string
	// map of indices to which components are found in the components slice
	componentsIndices map[string]int

	cComponents map[string]cComponent
	cHandlers   map[string]func(http.ResponseWriter, *http.Request)
}

func (c *Container) Init(ctx context.Context) error {
	c.cComponents = make(map[string]cComponent)
	c.componentsIndices = make(map[string]int)
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
		case _, ok := <-c.signalCh:
			if !ok {
				log.Println("signalCh closed for", c.GetName())
				return nil
			}

			shutdownErrCh := make(chan error, 1)
			notification := func() (context.Context, interface{}, chan<- error) {
				// TODO: later change it to a context with timeout
				log.Println("proceed to shutdown container")
				return context.TODO(), Shutdown, shutdownErrCh
			}

			c.Notify(notification)

			shutdownErr, isErrChOpen := <-shutdownErrCh
			if shutdownErr == nil && !isErrChOpen {
				log.Println("Shutdown notification was successfully sent to", c.GetName())
				return nil
			}
		case <-c.inbox:
		}
	}
}

func (c *Container) Add(comp Component) error {
	if comp != nil && len(comp.GetName()) <= 0 {
		log.Fatalln("Component name cannot be empty")
	}

	c.componentLifecycleFSM(context.TODO(), comp)

	cComm := cComponent{}
	err := c.toCanonical(comp, &cComm)
	if err != nil {
		log.Println(comp, "failed to convert to canonical type due to", err.Error())
		return err
	}

	if _, found := c.cComponents[comp.GetName()]; found {
		err := fmt.Errorf("%v already activated", comp.GetName())
		log.Println("failed to add component due to", err.Error())
		return err
	}
	c.cComponents[comp.GetName()] = cComm
	c.components = append(c.components, comp.GetName())
	c.componentsIndices[comp.GetName()] = len(c.components) - 1

	return nil
}

// componentLifecycleFSM is a FSM for handling various stages of a component.
func (c *Container) componentLifecycleFSM(ctx context.Context, comp Component) error {
	switch comp.GetStage() {
	case Submitted:
		comp.setStage(Preinitializing)
		fallthrough
	case Preinitializing:
		comp.preInit()
		ctx := context.Background()
		go c.startMmux(ctx, comp)
		comp.setStage(Preinitialized)
		fallthrough
	case Preinitialized:
		comp.setStage(Initializing)
		ctx := context.Background()
		defer func(comp Component) {
			c.componentLifecycleFSM(context.TODO(), comp)
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
	return nil
}

// startMmux is the main routine which blocks and receives all types of messages which are sent to a component. Components which have been
// preinitialized successfully, would each have its own message handlers started as separate go routines.
func (c *Container) startMmux(ctx context.Context, comp Component) {
	defer func(ctx context.Context, comp Component) {
		if comp.GetState() != Active {
			return
		}
		c.startMmux(ctx, comp)
	}(ctx, comp)

	for msgFunc := range comp.getMmux() {
		msgCtx, msg, errCh := msgFunc()

		switch msgType := msg.(type) {
		case controlMsg:
			switch msgType {
			case Shutdown:
				comp.setStage(Stopping)
				err := c.componentLifecycleFSM(msgCtx, comp)
				if err != nil {
					errCh <- err
				}
				close(errCh)
				if err == nil {
					return
				}
			}
		case Component:
			// check if message is a copy of the same underlying concrete component type
			compConcreteType, msgCompConcreteType := reflect.TypeOf(comp).Elem(), reflect.TypeOf(msg).Elem()
			if compConcreteType == msgCompConcreteType {
				// validate if etag of the copy is current or not
				if comp.GetEtag() != msgType.GetEtag() {
					errCh <- errors.New("component copy is not current due to mismatched etag")
					close(errCh)
					break
				}
			}
			goto sendToInbox
		default:
			goto sendToInbox
		}

	sendToInbox:
		inbox := comp.getInbox()
		if inbox == nil {
			close(errCh)
			continue
		}
		inbox <- msgFunc

	}
}

func (c *Container) startComponent(ctx context.Context, comp Component) {
	defer func(comp Component) {
		c.componentLifecycleFSM(ctx, comp)
	}(comp)

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
		comp.setStage(Restarting)
		log.Println(comp, "to restart after", delay)
		time.Sleep(delay)
		return
	}
}

// toCanonical enhances the component and assigns it to the passed cComponent type
func (c *Container) toCanonical(comp Component, cComp *cComponent) error {
	// assign reference of container to the component getting added only if the names do not match.
	if comp.GetName() != c.GetName() {
		comp.setContainer(c)
	}

	cName := comp.GetName()
	if len(cName) <= 0 {
		log.Fatalln("Component name cannot be empty")
	}
	cName = strings.ToLower(cName)
	cComp.cName = cName
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

func (c *Container) Stop(ctx context.Context) error {
	// proceed to stop components in LIFO order
	last := len(c.components) - 1

	for last >= 0 {
		cName := c.components[last]
		var (
			cComp cComponent
			found bool
		)

		if cComp, found = c.cComponents[cName]; !found || cComp.comp == nil {
			log.Println(cName, "component no longer found within container", c.GetName())
			// after iterating in LIFO order, the current container would be one left in the list. ignore sending Shutdown to itself.
			// so maintaining check if the current component cComp.comp.GetName() != c.GetName()
		} else if cComp.comp.GetName() != c.GetName() {
			log.Println("sending", Shutdown, "signal to", cName)
			errCh := make(chan error)
			cComp.comp.Notify(func() (context.Context, interface{}, chan<- error) {
				return context.TODO(), Shutdown, errCh
			})
			err := <-errCh
			if err != nil {
				return err
			}
		}

		c.components = c.components[:last]
		last = len(c.components) - 1
		if found {
			delete(c.cComponents, cName)
			delete(c.componentsIndices, cName)
		}
	}

	defer func() {
		signal.Stop(c.signalCh)
		if c.signalCh != nil {
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
	var (
		cComp cComponent
		found bool
	)

	c.GetLock().Lock()
	if cComp, found = c.cComponents[name]; !found {
		return nil, fmt.Errorf("unable to find component %v within %v", name, c.GetName())
	}
	c.GetLock().Unlock()

	comp := cComp.comp
	if comp == nil {
		return nil, errors.New("unable to find component type within cComponent")
	}

	return GetComponentCopy(comp)
}

func (c *Container) GetHttpHandler(URI string) func(w http.ResponseWriter, r *http.Request) {
	return c.cHandlers[URI]
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
	c.mutex.Lock()
	defer c.mutex.Unlock()

	delete(c.cComponents, name)

	indx, found := -1, false
	if indx, found = c.componentsIndices[name]; !found {
		return
	}

	// removing component name from silce at indx
	c.components = append(c.components[:indx], c.components[indx+1:]...)
	delete(c.componentsIndices, name)
}

func GetComponentCopy(comp Component) (Component, error) {
	comp.GetLock().Lock()
	defer comp.GetLock().Unlock()

	// compute hash of the component object and compare with existing hash
	newHash, err := hashstructure.Hash(comp, hashstructure.FormatV2, nil)
	if err != nil {
		return nil, err
	}

	// assign new etag if component instance hash has changed. this is required in order to resolve conflicts arising
	// due to concurrent updates through messages.
	if newHash != comp.Hash() {
		comp.setHash(newHash)
		etag := uuid.New()
		comp.setEtag(etag.String())
	}

	compType := reflect.TypeOf(comp).Elem()
	compVal := reflect.ValueOf(comp).Elem().Interface()

	newCTypeValue := reflect.New(compType)
	newVal := newCTypeValue.Interface()

	copyStruct(newVal, compVal)
	return newVal.(Component), nil
}

func copyStruct(dest, src interface{}) {
	buf := new(bytes.Buffer)
	gob.NewEncoder(buf).Encode(src)
	gob.NewDecoder(buf).Decode(dest)
}

func deriveHttpHandlers(comp Component) map[string]func(w http.ResponseWriter, r *http.Request) {
	cName := strings.ToLower(comp.GetName())
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
	cURI += cName

	cType := reflect.TypeOf(comp)
	cTypeVal := reflect.ValueOf(comp)

	handlers := map[string]func(w http.ResponseWriter, r *http.Request){}

	//derive the http routes within the component
	for i := 0; i < cTypeVal.NumMethod(); i++ {
		methodVal := cTypeVal.Method(i)

		// check for methods matching func signature of http handler
		if httpHandlerFunc, ok := methodVal.Interface().(func(http.ResponseWriter, *http.Request)); ok {
			methodName := cType.Method(i).Name
			// ServeHTTP becomes the default URI to the component
			if methodName == "ServeHTTP" {
				handlers[cURI] = httpHandlerFunc
			} else {
				handlerURI := cURI + "/" + strings.ToLower(methodName)
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
