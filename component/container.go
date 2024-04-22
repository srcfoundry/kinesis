package component

import (
	"bytes"

	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"regexp"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"
)

func init() {
	// initialize logger
	logger = createLogger()
	defer logger.Sync()

	// init primarily deals with initializing the default root container component
	rootContainer = new(Container)
	rootContainer.Name = "root"
	AttachComponent(true, rootContainer)

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM, syscall.SIGABRT, syscall.SIGUSR1)

	go func() {
		osSignal := <-signalCh
		rootContainer.GetLogger().Info("received interrupt signal", zap.String("signal", osSignal.String()))

		signal.Stop(signalCh)
		close(signalCh)
		signalCh = nil

		subscribe := make(chan interface{}, 1)
		defer close(subscribe)
		rootContainer.Subscribe("root.init", subscribe)

		// if system interrupt is received, proceed to shutdown all top level components except root container.
		for i := len(topLevelComponents) - 1; i > 0; i-- {
			nxtTopLvlComp := topLevelComponents[i]
			rootContainer.GetLogger().Info("sending SyncMessage", zap.String("targetcomponent", nxtTopLvlComp.GetName()), zap.Any(string(ControlMsgType), Shutdown))
			err := nxtTopLvlComp.SendSyncMessage(5*time.Second, ControlMsgType, map[interface{}]interface{}{ControlMsgType: Shutdown})
			if err == nil {
				rootContainer.GetLogger().Debug("SendSyncMessage", zap.String("targetcomponent", nxtTopLvlComp.GetName()), zap.Any(string(ControlMsgType), Shutdown), zap.Bool("success", true))
			}
		}

		// once all top level components are stopped, proceed to stop root container
		go func() {
			err := topLevelComponents[0].SendSyncMessage(5*time.Second, ControlMsgType, map[interface{}]interface{}{ControlMsgType: Shutdown})
			if err == nil {
				rootContainer.GetLogger().Debug("SendSyncMessage", zap.String("targetcomponent", topLevelComponents[0].GetName()), zap.Any(string(ControlMsgType), Shutdown), zap.Bool("success", true))
			}
		}()

		for notification := range subscribe {
			if notification == Stopped {
				rootContainer.GetLogger().Sugar().Info("exiting")
				// syscall.SIGUSR1 is used for testing
				if osSignal != syscall.SIGUSR1 {
					os.Exit(0)
				}
			}
		}
	}()
}

var (
	// the root container component
	rootContainer *Container
	// add-on components intended to attach with root container. deferred until root container is initialized
	addons []Component
	// components (including containers) which dont have an explicit parent container are included as top level components
	topLevelComponents []Component
)

// AttachComponent used for activating add-on components which attaches to the root container. Add-on components
// could be optionally included by means of golang build tags. Refer 'addons' package for all available add-on components.
func AttachComponent(isHead bool, addon Component) {
	if rootContainer == nil {
		if isHead {
			addons = append([]Component{addon}, addons...)
		} else {
			addons = append(addons, addon)
		}
	} else {
		err := rootContainer.Add(addon)
		if err != nil {
			rootContainer.GetLogger().Error("error while attaching", zap.String("component", addon.GetName()), zap.Error(err))
			os.Exit(1)
		}
	}
}

type Container struct {
	SimpleComponent

	// slice to maintain the order of components being added
	components []string

	// channel maintained as a queue for adding & activating Components sequentially.
	// Addition of components is made possible through a dedicated go routine which reads from compActivationQueue channel, thereby unblocking the caller to
	// perform other activities like subscribing to state change notifications or just continuing with remaining execution.
	compActivationQueue  chan Component
	compActivationNotify chan error

	cComponents map[string]cComponent
	cHandlers   map[string]func(http.ResponseWriter, *http.Request)

	persistence *Persistence
}

func (c *Container) Init(ctx context.Context) error {
	c.cComponents = make(map[string]cComponent)
	c.cHandlers = make(map[string]func(http.ResponseWriter, *http.Request))
	return nil
}

func (c *Container) Add(comp Component) error {
	err := validateName(comp)
	if err != nil {
		return err
	}

	currCompName := comp.GetName()
	if !c.Matches(comp) && currCompName == c.GetName() {
		return fmt.Errorf("Component name could not have same name as container")
	}

	var parentLogger *zap.Logger
	if parentLogger = c.GetLogger(); parentLogger == nil {
		parentLogger = logger
	}

	childLogger := parentLogger.With(zap.String("component", comp.GetName()))
	comp.setLogger(childLogger)

	if c.compActivationQueue == nil && c.GetStage() < Stopping {
		c.compActivationQueue = make(chan Component, 1)
		c.compActivationNotify = make(chan error)

		// Even though components could be added asynchronously, it is activated sequentially since its being read off the compActivationQueue channel, thereby maintaining order.
		go func(notifyActivation chan error) {
			for nextComp := range c.compActivationQueue {

				// proceed to initialize component
				err = c.componentLifecycleFSM(context.TODO(), nextComp)
				if err != nil {
					c.GetLogger().Error("addon activation failure", zap.String("component", nextComp.GetName()), zap.Error(err))
					notifyActivation <- err
					continue
				}

				cComm := cComponent{}
				err = c.toCanonical(nextComp, &cComm)
				if err != nil {
					c.GetLogger().Error("failed Canonical conversion", zap.String("component", nextComp.GetName()), zap.Error(err))
					notifyActivation <- err
					continue
				}

				if _, found := c.cComponents[nextComp.GetName()]; found {
					err := fmt.Errorf("%v already activated", nextComp.GetName())
					notifyActivation <- err
					continue
				}
				c.cComponents[nextComp.GetName()] = cComm
				c.getMutatingLock().Lock()
				c.components = append(c.components, nextComp.GetName())
				c.getMutatingLock().Unlock()

				// after successfull initilaization, proceed to start the component
				err = c.componentLifecycleFSM(context.TODO(), nextComp)
				if err != nil {
					notifyActivation <- err
					continue
				}

				// register infra add-ons like Persistence within root container, for persisting components
				if c == rootContainer {
					c.registerAddon(nextComp)
				}

				notifyActivation <- nil
			}
		}(c.compActivationNotify)
	}
	c.compActivationQueue <- comp
	err = <-c.compActivationNotify
	if err != nil {
		return err
	}

	// the following is for root container to activate any deferred add-on components that is yet to initialized
	if rootContainer != nil && len(addons) > 0 {
		// copy over addons to a new slice and mark original addons as empty otherwise rootContainer.Add() would
		// result in a recursive call within this condition, to add the add-on component again, unless addons is emptied.
		addonsCopy := make([]Component, len(addons))
		copy(addonsCopy, addons)
		addons = nil

		for _, addon := range addonsCopy {
			c.GetLogger().Info("proceed to activate", zap.String("component", addon.GetName()))
			err = rootContainer.Add(addon)
			if err != nil {
				c.GetLogger().Fatal("addon activation failure", zap.String("component", addon.GetName()), zap.Error(err))
			}
		}
	}

	return nil
}

// used for register infra add-ons like Persistence within root container, for persisting components
func (c *Container) registerAddon(comp Component) {
	switch addOn := comp.(type) {
	case *Persistence:
		c.persistence = addOn
	}
}

// componentLifecycleFSM is a FSM for handling various stages of a component.
func (c *Container) componentLifecycleFSM(ctx context.Context, comp Component) error {
	comp.getMutatingLock().Lock()
	defer func() {
		// persist persistable component fields to DB
		if rootContainer != nil && rootContainer.persistence != nil {
			err := rootContainer.persist(context.Background(), comp)
			if err != nil {
				c.GetLogger().Error("persistence failure", zap.String("component", comp.GetName()), zap.Error(err))
			}
		}
		comp.getMutatingLock().Unlock()
	}()

	switch comp.GetStage() {
	case Submitted:
		comp.setStage(Preinitializing)
		fallthrough
	case Preinitializing:
		comp.preInit()
		_ = comp.Callback(true, stateChangeCallbacker(comp))
		ctx := context.Background()

		// add default message handler if not added
		if comp.getSyncMessageHandler(comp.GetName()) == nil {
			comp.SetSyncMessageHandler(comp.GetName(), comp.DefaultSyncMessageHandler)
		}

		go c.startMmux(ctx, comp)
		comp.setStage(Preinitialized)
		fallthrough
	case Preinitialized:
		comp.setStage(Initializing)
		ctx := context.Background()

		// load persistable fields from DB prior to initilization of component
		if rootContainer != nil && rootContainer.persistence != nil {
			err := rootContainer.load(context.Background(), comp)
			if err != nil {
				c.GetLogger().Error("failed persistence loading", zap.String("component", comp.GetName()), zap.Error(err))
			}
		}

		err := comp.Init(ctx)
		if err != nil {
			comp.setStage(Aborting)
			comp.setStage(Tearingdown)
			return err
		}
		err = comp.PostInit(ctx)
		if err != nil {
			comp.setStage(Aborting)
			comp.setStage(Tearingdown)
			return err
		}
		comp.setStage(Initialized)
	case Initialized, Restarting:
		// PreStart is initiated in a separate goroutine to ensure that startComponent proceeds even if an error occurs during prestart
		go func() {
			err := comp.PreStart(ctx)
			if err != nil {
				c.GetLogger().Error("error during prestart", zap.String("component", comp.GetName()), zap.Error(err))
			}
		}()
		comp.setStage(Starting)
		ctx := context.Background()
		go c.startComponent(ctx, comp)
		comp.setStage(Started)
	case Started:
		ctrlMsg := ctx.Value(ControlMsgType)
		switch ctrlMsg {
		case RestartMmux:
			c.GetLogger().Warn("restarting mmux", zap.String("component", comp.GetName()))
			go c.startMmux(ctx, comp)
		case RestartAfter:
			comp.setStage(Restarting)
			restartAfter := ctx.Value(RestartAfter)
			delay, found := restartAfter.(time.Duration)
			if !found {
				delay = 5 * time.Second
			}
			c.GetLogger().Warn("component restart", zap.String("component", comp.GetName()), zap.Any("delay", delay))
			time.Sleep(delay)
			return nil
		case Shutdown:
			comp.setStage(Stopping)
		default:
			return nil
		}
	case Stopping:
		err := comp.Stop(ctx)
		if err != nil {
			c.GetLogger().Error("failed to stop", zap.String("component", comp.GetName()), zap.Error(err))
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
		// making sure panics are caught while processing messages
		if r := recover(); r != nil {
			c.GetLogger().Warn("mmux recovering from panic", zap.String("component", comp.GetName()), zap.Any("recover", r), zap.String("stacktrace", string(debug.Stack())))
		}
		// mmux would only be restarted if the component is already Started & still Active
		ctx = context.WithValue(ctx, ControlMsgType, RestartMmux)
		go c.componentLifecycleFSM(ctx, comp)
	}(ctx, comp)

	for msgFunc := range comp.getMmux() {
		msgCtx, msgType, msgsLookup, errCh := msgFunc()
		var err error

		// performing sync message validation
		if msgType == nil {
			err = fmt.Errorf("encountered nil msgType for %s", comp.GetName())
		} else if msgsLookup == nil {
			err = fmt.Errorf("encountered nil msgsLookup for %s", comp.GetName())
		}

		if err != nil {
			c.GetLogger().Error("error reading message", zap.String("component", comp.GetName()), zap.Error(err))
			if errCh != nil {
				errCh <- err
			}
			continue
		}

		// read message from the lookup & take appropriate actions based on the message type.
		msg := msgsLookup[msgType]

		switch msgType {
		case ControlMsgType:
			msgCtx = context.WithValue(msgCtx, ControlMsgType, msg)
			switch msg {
			//additional control handling cases goes here
			case Shutdown:
				_ = c.componentLifecycleFSM(msgCtx, comp)
				// once component stage is stopping, invoke componentLifecycleFSM again to actually stop the component
				if comp.GetStage() == Stopping {
					err = c.componentLifecycleFSM(msgCtx, comp)
					if err != nil {
						c.GetLogger().Error("failed to stop", zap.String("component", comp.GetName()), zap.Error(err))
						errCh <- err
						continue
					}
				}
				errCh <- err
				return
			}
		case ComponentMsgType:
			switch msg.(Component).GetName() {
			case comp.GetName():
				// check if message is a copy of the same underlying concrete component type
				msgCompType, compConcreteType, msgCompConcreteType := msg.(Component), reflect.TypeOf(comp).Elem(), reflect.TypeOf(msg).Elem()
				if compConcreteType == msgCompConcreteType {
					// validate if etag of the copy is current or not
					if comp.GetEtag() != msgCompType.GetEtag() {
						errCh <- errors.New("component copy is not current due to mismatched etag")
						continue
					}
				}
				goto forwardMessage
			default:
				goto forwardMessage
			}
		}

	forwardMessage:
		handler := comp.getSyncMessageHandler(msgType)
		if handler == nil {
			c.GetLogger().Debug("passing message to default message handler", zap.String("component", comp.GetName()), zap.Any("msgType", msgType))
			handler = comp.getSyncMessageHandler(comp.GetName())
		}
		err = handler(msgCtx, msg)
		if err != nil {
			errCh <- err
			continue
		}

		// if persistence add-on is enabled, persist the resulting state of the component after message processing
		if rootContainer != nil && rootContainer.persistence != nil {
			err = rootContainer.persist(msgCtx, comp)
			if err != nil {
				errCh <- err
				continue
			}
		}

		cCopy, err := createCopy(comp)
		if err != nil {
			errCh <- err
			continue
		}

		comp.invokeCallbacks(cCopy)
		comp.notifySubscribers(cCopy)

		errCh <- err
	}
}

// persist Component fields' tagged as `persistable`, within the selected Database
func (c *Container) persist(ctx context.Context, comp Component) error {
	if c.persistence == nil {
		return fmt.Errorf("%s does not have persistence add-on enabled", c)
	}

	pTypes := c.persistable(comp, c.persistence.getSymmetricKey())
	if len(pTypes) <= 0 {
		c.GetLogger().Debug("unable to find any persistable fields", zap.String("component", comp.GetName()))
		return nil
	}

	pTypesBytes, err := json.Marshal(pTypes)
	if err != nil {
		return fmt.Errorf("unable to convert PTypes to []byte while persisting, due to: %s", err)
	}

	// compare existing component type cache before persisting
	if bytes.Equal(pTypesBytes, comp.getTypeCache()) {
		return nil
	}

	comp.setTypeCache(pTypesBytes)

	ctx = ContextWithLogger(ctx, comp.GetLogger())
	if noSqlDB, isNoSqlDB := interface{}(c.persistence.DB).(NoSQLDB); isNoSqlDB {
		return noSqlDB.Update(ctx, comp.GetName(), nil, pTypes)
	} else if relationalDB, isRelationalDB := interface{}(c.persistence.DB).(RelationalDB); isRelationalDB {
		// implement for RelationalDB here
		_, err := relationalDB.Query(ctx, "TODO", nil)
		return err
	}

	return fmt.Errorf("unable to determine persistence DB type for %s", c)
}

// read last persisted value of Component fields' tagged as `persistable`,from Database during Preinitialization
func (c *Container) load(ctx context.Context, comp Component) error {
	if c.persistence == nil {
		return fmt.Errorf("%s does not have persistence add-on enabled", c)
	}

	var pTypes PTypes
	ctx = ContextWithLogger(ctx, comp.GetLogger())

	if noSqlDB, isNoSqlDB := interface{}(c.persistence.DB).(NoSQLDB); isNoSqlDB {
		err := noSqlDB.FindOne(ctx, comp.GetName(), nil, &pTypes, nil)
		if err != nil {
			return err
		}
	} else if relationalDB, isRelationalDB := interface{}(c.persistence.DB).(RelationalDB); isRelationalDB {
		// implement for RelationalDB here
		_, _ = relationalDB.Query(ctx, "TODO", nil)
	}

	if len(pTypes) <= 0 {
		return nil
	}

	pTypeMap := make(map[string]PType, len(pTypes))
	for _, pType := range pTypes {
		pTypeMap[pType.Name] = pType
	}

	cValue := reflect.ValueOf(comp)
	actualValue := reflect.Indirect(cValue)

	ctx = ContextWithLogger(ctx, comp.GetLogger())
	unmarshalToType(ctx, c.persistence.getSymmetricKey(), actualValue, actualValue.Type().Name(), pTypeMap)
	return nil
}

// determine persistable fields of a component as PTypes
func (c *Container) persistable(comp Component, encryptKey string) PTypes {
	cValue := reflect.ValueOf(comp)
	v := reflect.Indirect(cValue)

	pTypes := PTypes{}
	ctx := context.Background()
	ctx = ContextWithLogger(ctx, comp.GetLogger())
	marshalType(ctx, encryptKey, v, v.Type().Name(), &pTypes)
	return pTypes
}

func (c *Container) startComponent(ctx context.Context, comp Component) {
	defer func(ctx context.Context, comp Component) {
		// making sure panics are caught while starting a component
		if r := recover(); r != nil {
			c.GetLogger().Warn("startComponent recovering from panic", zap.String("component", comp.GetName()), zap.Any("recover", r), zap.String("stacktrace", string(debug.Stack())))
		}
		if comp.GetStage() >= Stopping {
			return
		}
		isRestartable, delay := comp.IsRestartableWithDelay()
		if !isRestartable {
			return
		}
		if delay <= time.Duration(0) {
			delay = 5 * time.Second
		}
		ctx = context.WithValue(ctx, ControlMsgType, RestartAfter)
		ctx = context.WithValue(ctx, RestartAfter, delay)
		err := c.componentLifecycleFSM(ctx, comp)
		if err != nil {
			c.GetLogger().Error("failed to restart", zap.String("component", comp.GetName()), zap.Error(err))
			return
		}
		// proceed to start the component
		c.componentLifecycleFSM(ctx, comp)
	}(ctx, comp)

	err := comp.Start(ctx)
	if err != nil {
		c.GetLogger().Error("failed to start", zap.String("component", comp.GetName()), zap.Error(err))
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
	if !c.Matches(comp) {
		comp.setContainer(c)
	} else {
		topLevelComponents = append(topLevelComponents, comp)
	}

	cComp.cName = comp.GetName()
	cComp.comp = comp

	handlers := deriveHttpHandlers(comp)
	if len(handlers) <= 0 {
		goto returnnoerror
	}

	for cURI, httpHandlerFunc := range handlers {
		rootContainer.cHandlers[cURI] = httpHandlerFunc
		c.GetLogger().Info("adding URI", zap.String("component", comp.GetName()), zap.String("URI", cURI))
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
	// proceed to stop components in LIFO order
	last := len(c.components) - 1

	for last >= 0 {
		cName := c.components[last]
		cComp, found := c.cComponents[cName]

		if !found || cComp.comp == nil {
			c.GetLogger().Warn("component no longer found within container", zap.String("component", cName))
		} else if !c.Matches(cComp.comp) {
			c.GetLogger().Debug("sending ControlMsgType", zap.String("container", c.GetName()), zap.String("component", cName), zap.Any("ControlMsgType", Shutdown))
			err := cComp.comp.SendSyncMessage(5*time.Second, ControlMsgType, map[interface{}]interface{}{ControlMsgType: Shutdown})
			if err != nil {
				c.GetLogger().Error("SendSyncMessage failed", zap.String("component", cName), zap.Any("ControlMsgType", Shutdown), zap.Error(err))
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

	return nil
}

// GetComponent returns a copy of the component within a container. Only exported field values would be copied over. Advisable not to mark pointer
// fields within a component as an exported field. Reference types such as slice, map, channel, interface, and function types which are exported would
// be copied over.
func (c *Container) GetComponent(name string) (Component, error) {
	c.getMutatingLock().RLock()
	defer c.getMutatingLock().RUnlock()

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

	return createCopy(comp)
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
		return
	}

	for cURI, _ := range handlers {
		delete(c.cHandlers, cURI)
		c.GetLogger().Info("removed URI", zap.String("component", comp.GetName()), zap.String("URI", cURI))
	}
}

// removeComponent removes a component from a container in the event its being shutdown
func (c *Container) removeComponent(name string) {
	delete(c.cComponents, name)

	indx, found := len(c.components)-1, false
	for ; indx >= 0; indx-- {
		if c.components[indx] == name {
			found = true
			break
		}
	}

	if !found {
		c.GetLogger().Warn("unable to find component within container", zap.String("component", name))
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
		comp.GetLogger().Debug("stateChangeCallbacker", zap.Any("notification", notification))
		switch notification {
		case Active, Inactive:
			comp.GetLogger().Debug("proceeding to set new ETag")
			UpdateComponentEtag(comp)
		case Stopping:
			err := comp.RemoveCallback(cbIndx)
			if err != nil {
				comp.GetLogger().Error("error while removing callback", zap.Error(err))
			}
		default:
		}
	}
}

func deriveHttpHandlers(comp Component) map[string]func(w http.ResponseWriter, r *http.Request) {
	if comp.IsNonRestEntity() {
		return nil
	}
	// proceeding to assign URI for the component. for that we would need to travserse all the way to top container.
	rootC := comp.GetContainer()
	paths := []string{}
	for rootC != nil {
		paths = append(paths, strings.ToLower(rootC.GetName()))
		rootC = rootC.GetContainer()
	}

	cURI := "/"
	if len(paths) > 0 {
		for i := len(paths) - 1; i >= 0; i-- {
			cURI += paths[i] + "/"
		}
	}
	cURI += strings.ToLower(comp.GetName())

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
