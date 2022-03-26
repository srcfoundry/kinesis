package component

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"reflect"
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
	components  []string
	cComponents map[string]cComponent
}

func (c *Container) Init(ctx context.Context) error {
	c.cComponents = make(map[string]cComponent)
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
		case <-c.signalCh:
			shutdownErrCh := make(chan error, 1)
			notification := func() (context.Context, interface{}, chan<- error) {
				// TODO: later change it to a context with timeout
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
		err := errors.New("activating component with same name " + comp.GetName())
		log.Println("failed to add component due to", err.Error())
		return err
	}
	c.cComponents[comp.GetName()] = cComm
	c.components = append(c.components, comp.GetName())

	return nil
}

// componentLifecycleFSM is a FSM for handling various stages of a component.
func (c *Container) componentLifecycleFSM(ctx context.Context, comp Component) error {
	switch comp.Stage() {
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
		comp.setStage(Stopped)
		fallthrough
	case Stopped:
		fallthrough
	case Tearingdown:
		comp.tearDown()
		comp.setStage(Teareddown)
	}

	return nil
}

// startMmux is the main routine which blocks and receives all types of messages which are sent to a component. Components which have been
// preinitialized successfully, would each have its own message handlers started as separate go routines.
func (c *Container) startMmux(ctx context.Context, comp Component) {
	defer func(ctx context.Context, comp Component) {
		if comp.State() != Active {
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
			}
		default:
			inbox := comp.getInbox()
			if inbox == nil {
				continue
			}
			inbox <- msgFunc
		}
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
	// assign reference of container to the component getting added.
	if comp != c {
		comp.setContainer(c)
	}

	cName := comp.GetName()
	if len(cName) <= 0 {
		log.Fatalln("Component name cannot be empty")
	}
	cName = strings.ToLower(cName)
	cComp.cName = cName

	// proceeding to assign URI for the component. for that we would need to travserse all the way to top container.
	rootC := c.GetContainer()
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

	handlers := map[string]func(http.ResponseWriter, *http.Request){}
	cType := reflect.TypeOf(comp)
	cTypeVal := reflect.ValueOf(comp)

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

	cComp.comp = comp
	if len(handlers) > 0 {
		cComp.cHandlers = handlers
	}

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
			// so maintaining check if the current component cComp.comp != c
		} else if cComp.comp != c {
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
		}
	}

	return nil
}

func (c *Container) GetComponent() {

}

// canonical Component which enhances the component
type cComponent struct {
	cName     string
	comp      Component
	cHandlers map[string]func(http.ResponseWriter, *http.Request)
}
