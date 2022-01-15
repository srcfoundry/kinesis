package component

import (
	"context"
	"errors"
	"log"
	"net/http"
	"reflect"
	"strings"
	"time"
)

const (
	DefaultTimeout = 3000 // ms
)

type Container struct {
	SimpleComponent
	cComponents map[string]cComponent
}

func (c *Container) Init(ctx context.Context) {
	c.cComponents = make(map[string]cComponent)
}

func (c *Container) Add(comp Component) {
	cComm := cComponent{}
	c.toCanonical(comp, &cComm)

	// stages of initializing and starting a component
	comp.setState(Preinitializing)
	comp.preInit()
	comp.setState(Preinitialized)

	c.initComponent(comp)

	comp.setState(Starting)
	//comp.Start(c.Context())
	comp.setState(Started)
}

// initComponent avoids using its mutex lock since it would lead to a deadlock, in the event another component were to be
// initialized from within the Init method of the current component.
// TODO: test for init new component from within init of a component
func (c *Container) initComponent(comp Component) {
	comp.setState(Initializing)

	var sType *SimpleComponent
	var ok bool

	if sType, ok = comp.(*SimpleComponent); !ok {
		log.Fatalln("Cannot initialize non SimpleComponent type", comp)
	}

	timeout := sType.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	ctx := context.Background()

	// if timeout > 0, initiate a time bound Init() call else initiate a blocking call (probably needed for Infra components requiring a hard requirement)
	if timeout > 0 {
		tCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
		defer cancel()

		comp.Init(tCtx)

		// Since there is no way for the caller to know whether the callee had timed out, we would have to explicity check the context if it has any DeadlineExceeded error.
		// we're using errors.Is since it checks all the errors in the wrap chain, and returns true if any of them matches. Reason is because some mechanisms like
		// http client tend to wrap context.DeadlineExceeded with extra information.
		// So in the event the callee had timed out, prevent the component from proceeding further and prepare to teardown.
		if errors.Is(tCtx.Err(), context.DeadlineExceeded) {
			log.Println(comp, "encountered", tCtx.Err())
			comp.setState(Aborting)
			comp.setState(Tearingdown)
			comp.tearDown()
			comp.setState(Teareddown)
		}
	} else {
		comp.Init(ctx)
	}

	comp.setState(Initialized)
}

func (c *Container) toCanonical(comp Component, cComp *cComponent) {

	cType := reflect.TypeOf(comp)
	if cType == nil {
		log.Fatalln("Cannot determine type", comp)
	}

	cTypeVal := reflect.ValueOf(comp)

	var cName, cURI string

	if simpleCompType, ok := comp.(*SimpleComponent); ok {
		// assign reference of container to the component getting added.
		simpleCompType.container = c

		cName = simpleCompType.Name
		if len(cName) <= 0 {
			// if component name is empty, derive it from the component type
			cName = cType.Name()
		}
		cName = strings.ToLower(cName)

		// proceeding to assign URI for the component
		cURI = c.cURI + "/" + cName
		simpleCompType.cURI = cURI
	}

	handlers := map[string]func(http.ResponseWriter, *http.Request){}

	// derive the http routes within the component
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
}

// canonical Component which enhances the component
type cComponent struct {
	comp      Component
	cHandlers map[string]func(http.ResponseWriter, *http.Request)
}
