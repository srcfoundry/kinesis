package component

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"strings"
	"time"
)

type Container struct {
	SimpleComponent
	cComponents map[string]cComponent
}

func (c *Container) Init(ctx context.Context) {
	c.cComponents = make(map[string]cComponent)
}

func (c *Container) Add(comp Component) error {
	cComm := cComponent{}
	err := c.toCanonical(comp, &cComm)
	if err != nil {
		log.Println(comp, "failed to convert to canonical type due to", err.Error())
		return err
	}
	c.componentLifecycleFSM(comp)
	return nil
}

// componentLifecycleFSM is a fsm for handling various stages of a component.
func (c *Container) componentLifecycleFSM(comp Component) {
	switch comp.Stage() {
	case Submitted:
		comp.setStage(Preinitializing)
		fallthrough
	case Preinitializing:
		comp.preInit()
		comp.setStage(Preinitialized)
		fallthrough
	case Preinitialized:
		comp.setStage(Initializing)
		ctx := context.Background()
		defer func(comp Component) {
			c.componentLifecycleFSM(comp)
		}(comp)

		// TODO: test for init new component from within init of a component
		err := comp.Init(ctx)
		if err != nil {
			comp.setStage(Aborting)
			comp.setStage(Tearingdown)
		} else {
			comp.setStage(Initialized)
		}
	case Initialized:
		ctx := context.Background()
		go c.startMessageReceiver(ctx, comp)
		comp.setStage(MessageReceiverStarted)
	case MessageReceiverStarted, Restarting, Starting:
		comp.setStage(Starting)
		ctx := context.Background()
		go c.startComponent(ctx, comp)
		comp.setStage(Started)
	case Started:
		// do nothing
	case Tearingdown:
		comp.tearDown()
		comp.setStage(Teareddown)
	}
}

func (c *Container) startMessageReceiver(ctx context.Context, comp Component) {
	defer func(comp Component) {
		c.componentLifecycleFSM(comp)
	}(comp)

	err := comp.MessageReceiver(ctx)
	if err != nil {
		comp.setStage(Aborting)
		comp.setStage(Tearingdown)
	}
}

func (c *Container) startComponent(ctx context.Context, comp Component) {
	defer func(comp Component) {
		c.componentLifecycleFSM(comp)
	}(comp)

	err := comp.Start(ctx)
	if err != nil {
		isRestartable, delay := comp.IsRestartableWithDelay()
		if isRestartable {
			comp.setStage(Restarting)
			log.Println(comp, "to restart after", delay)
			time.Sleep(delay)
		}
	}
}

// toCanonical enhances the component and assigns it to the passed cComponent type
func (c *Container) toCanonical(comp Component, cComp *cComponent) error {

	cType := reflect.TypeOf(comp)
	if cType == nil {
		log.Fatalln("Cannot determine type for", comp)
	}

	cTypeVal := reflect.ValueOf(comp)

	var (
		cName, cURI    string
		ok             bool
		simpleCompType *SimpleComponent
	)

	if simpleCompType, ok = comp.(*SimpleComponent); !ok {
		return errors.New(fmt.Sprintln("Cannot initialize non SimpleComponent type", comp))
	}

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

	return nil
}

// canonical Component which enhances the component
type cComponent struct {
	comp      Component
	cHandlers map[string]func(http.ResponseWriter, *http.Request)
}
