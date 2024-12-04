package component

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

func shutdownTestContainer(c *Container, delay time.Duration) {
	subscribe, errCh := make(chan ChangeObject, 1), make(chan error, 1)
	defer func() {
		close(errCh)
		close(subscribe)
	}()

	c.Subscribe("testS", subscribe)

	go func() {
		time.Sleep(delay)
		logger.Info("shutdownTestContainer....")
		c.SendSyncMessage(5*time.Second, ControlMsgType, map[interface{}]interface{}{ControlMsgType: Shutdown})
	}()

outer:
	for {
		select {
		case notification := <-subscribe:
			logger.Info("received notification :::::  " + notification.String())
			switch changeObj := notification.(type) {
			case StageChangeObject:
				if changeObj.GetCurrentObject() == Stopped {
					break outer
				}
			}
		case err := <-errCh:
			fmt.Println("obtained error", err)
		}
	}
}

func TestContainer_Add(t *testing.T) {
	c := &Container{}
	c.Name = "testContainer"
	c.Add(c)

	type args struct {
		comp Component
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "Add_SimpleComponent_test2",
			args: args{
				comp: &SimpleComponent{Name: "simpleTestComponent"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			if err := c.Add(tt.args.comp); (err != nil) != tt.wantErr {
				t.Errorf("Container.Add() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
	shutdownTestContainer(c, 5*time.Second)
}

func TestContainer_GetComponentCopy(t *testing.T) {
	c := &Container{}
	c.Name = "testContainer"
	c.Add(c)

	type TestSimpleType struct {
		SimpleComponent
		String1   string
		Int1      int
		MapValues map[string]bool
	}

	var (
		intVal    int    = 7753
		stringVal string = "simpleStringValue"
		mapVals          = map[string]bool{"key1": true, "key2": false}
	)
	testComp := &TestSimpleType{}
	testComp.Name = "simpleTestComponent"
	testComp.String1 = stringVal
	testComp.Int1 = intVal
	testComp.MapValues = mapVals
	c.Add(testComp)

	time.Sleep(1 * time.Second)

	type args struct {
		name string
	}
	tests := []struct {
		name           string
		args           args
		wantName       string
		wantString1    string
		wantIntValue1  int
		wantMapValues1 map[string]bool
		wantErr        bool
	}{
		{
			name: "GetValidComponentCopy",
			args: args{
				name: "simpleTestComponent",
			},
			wantName:       "simpleTestComponent",
			wantIntValue1:  intVal,
			wantString1:    stringVal,
			wantMapValues1: mapVals,
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := c.GetComponentCopy(tt.args.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("Container.GetComponentCopy() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if got.GetName() != tt.wantName {
				t.Errorf("Container.GetComponentCopy() = %v, want %v", got.GetName(), tt.wantName)
			}

			gotSimpleType := got.(*TestSimpleType)
			if gotSimpleType.String1 != tt.wantString1 {
				t.Errorf("Container.GetComponentCopy() = %v, want %v", gotSimpleType.String1, tt.wantString1)
			}
			if gotSimpleType.Int1 != tt.wantIntValue1 {
				t.Errorf("Container.GetComponentCopy() = %v, want %v", gotSimpleType.Int1, tt.wantIntValue1)
			}
			if !reflect.DeepEqual(gotSimpleType.MapValues, tt.wantMapValues1) {
				t.Errorf("Container.GetComponentCopy() = %v, want %v", gotSimpleType.MapValues, tt.wantMapValues1)
			}
		})
	}

	shutdownTestContainer(c, 2*time.Second)
}

func TestContainer_NotifyValidComponentCopy(t *testing.T) {
	c := &Container{}
	c.Name = "testContainer"
	c.Add(c)

	type TestSimpleType struct {
		SimpleComponent
		String1   string
		Int1      int
		MapValues map[string]bool
	}

	var (
		testComponentName string = "simpleTestComponent"
		intVal            int    = 7753
		stringVal         string = "simpleStringValue"
		mapVals                  = map[string]bool{"key1": true, "key2": false}
	)
	testComp := &TestSimpleType{}
	testComp.Name = testComponentName
	testComp.String1 = stringVal
	testComp.Int1 = intVal
	testComp.MapValues = mapVals
	c.Add(testComp)

	time.Sleep(1 * time.Second)

	newCompCopy, _ := c.GetComponentCopy(testComponentName)

	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "NotifyValidComponentCopy",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := testComp.SendSyncMessage(5*time.Second, ComponentMsgType, map[interface{}]interface{}{ComponentMsgType: newCompCopy})
			if (err != nil) != tt.wantErr {
				t.Errorf("NotifyValidComponentCopy  error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}

	shutdownTestContainer(c, 2*time.Second)
}

func TestContainer_NotifyInvalidComponentCopy(t *testing.T) {
	c := &Container{}
	c.Name = "testContainer"
	c.Add(c)

	type TestSimpleType struct {
		SimpleComponent
		String1   string
		Int1      int
		MapValues map[string]bool
	}

	var (
		testComponentName string = "simpleTestComponent"
		intVal            int    = 7753
		stringVal         string = "simpleStringValue"
		mapVals                  = map[string]bool{"key1": true, "key2": false}
	)
	testComp := &TestSimpleType{}
	testComp.Name = testComponentName
	testComp.String1 = stringVal
	testComp.Int1 = intVal
	testComp.MapValues = mapVals
	c.Add(testComp)

	time.Sleep(1 * time.Second)

	newCompCopy, _ := c.GetComponentCopy(testComponentName)
	// overwrite Etag
	newCompCopy.setEtag("cafebeet")

	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "NotifyInvalidComponentCopy",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := testComp.SendSyncMessage(5*time.Second, ComponentMsgType, map[interface{}]interface{}{ComponentMsgType: newCompCopy})
			if (err != nil) != tt.wantErr {
				t.Errorf("NotifyInvalidComponentCopy  error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}

	shutdownTestContainer(c, 2*time.Second)
}

func TestContainer_VerifyComponentInitOrder(t *testing.T) {
	c := &Container{}
	c.Name = "testContainer"
	c.Add(c)

	type TestSimpleType struct {
		SimpleComponent
	}

	testComp1 := &TestSimpleType{}
	testComp1.Name = "simpleTestComponent1"

	testComp2 := &TestSimpleType{}
	testComp2.Name = "simpleTestComponent2"

	testComp3 := &TestSimpleType{}
	testComp3.Name = "simpleTestComponent3"

	type args struct {
		comp Component
	}
	tests := []struct {
		name      string
		args      args
		wantErr   bool
		wantOrder []string
	}{
		{
			name: "Add_simpleTestComponent1",
			args: args{
				comp: testComp1,
			},
			wantErr:   false,
			wantOrder: []string{"testContainer", "simpleTestComponent1"},
		},
		{
			name: "Add_simpleTestComponent2",
			args: args{
				comp: testComp2,
			},
			wantErr:   false,
			wantOrder: []string{"testContainer", "simpleTestComponent1", "simpleTestComponent2"},
		},
		{
			name: "Add_simpleTestComponent3",
			args: args{
				comp: testComp3,
			},
			wantErr:   false,
			wantOrder: []string{"testContainer", "simpleTestComponent1", "simpleTestComponent2", "simpleTestComponent3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := c.Add(tt.args.comp); (err != nil) != tt.wantErr {
				t.Errorf("VerifyComponentInitOrder add component error = %v, wantErr %v", err, tt.wantErr)
			}

			time.Sleep(1 * time.Second)

			if strings.Join(c.components, ",") != strings.Join(tt.wantOrder, ",") {
				t.Errorf("VerifyComponentInitOrder component order = %v, wantOrder %v", c.components, tt.wantOrder)
			}
		})
	}
	shutdownTestContainer(c, 2*time.Second)

}

// TODO: failing test case. investigate later
// func TestContainer_Invalid_GetComponentCopy(t *testing.T) {
// 	c := &Container{}
// 	c.Name = "testContainer"
// 	c.Add(c)

// 	type args struct {
// 		name string
// 	}
// 	tests := []struct {
// 		name          string
// 		args          args
// 		wantComponent Component
// 		wantErr       bool
// 	}{
// 		{
// 			name: "GetInvalidComponentCopy",
// 			args: args{
// 				name: "invalidTestComponent",
// 			},
// 			wantComponent: nil,
// 			wantErr:       true,
// 		},
// 	}

// 	subscribe, errCh := make(chan interface{}, 1), make(chan error, 1)
// 	c.Subscribe("testS", subscribe)

// 	go func() {
// 		time.Sleep(3 * time.Second)
// 		c.Notify(func() (context.Context, interface{}, chan<- error) {
// 			return nil, Shutdown, errCh
// 		})
// 	}()

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			got, err := c.GetComponentCopy(tt.args.name)
// 			if (err != nil) != tt.wantErr {
// 				t.Errorf("Container.GetComponentCopy() error = %v, wantErr %v", err, tt.wantErr)
// 				return
// 			}
// 			if got != tt.wantComponent {
// 				t.Errorf("Container.GetComponentCopy() = %v, want %v", got, tt.wantComponent)
// 			}
// 		})
// 	}

// outer:
// 	for {
// 		select {
// 		case notification := <-subscribe:
// 			if notification == Stopped {
// 				break outer
// 			}
// 		case <-errCh:
// 		}
// 	}
// }

func Test_validateName(t *testing.T) {
	type args struct {
		comp Component
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "validateName_validLowerCaseChars",
			args:    args{comp: &SimpleComponent{Name: "lowercase"}},
			wantErr: false,
		},
		{
			name:    "validateName_validUpperCaseChars",
			args:    args{comp: &SimpleComponent{Name: "UPPERCASE"}},
			wantErr: false,
		},
		{
			name:    "validateName_validCamelCaseChars",
			args:    args{comp: &SimpleComponent{Name: "camelCase"}},
			wantErr: false,
		},
		{
			name:    "validateName_validNumerals",
			args:    args{comp: &SimpleComponent{Name: "98765430"}},
			wantErr: false,
		},
		{
			name:    "validateName_validNumeralsAndUpperLowerChars",
			args:    args{comp: &SimpleComponent{Name: "789lowerUPPER4512"}},
			wantErr: false,
		},
		{
			name:    "validateName_validCharsWithPeriod",
			args:    args{comp: &SimpleComponent{Name: "789lower.UPPER4512"}},
			wantErr: false,
		},
		{
			name:    "validateName_validCharsWithHyphen",
			args:    args{comp: &SimpleComponent{Name: "789lower-UPPER4512"}},
			wantErr: false,
		},
		{
			name:    "validateName_validCharsWithUnderscore",
			args:    args{comp: &SimpleComponent{Name: "789lower_UPPER4512"}},
			wantErr: false,
		},
		{
			name:    "validateName_validCharsWithTilde",
			args:    args{comp: &SimpleComponent{Name: "789lower~UPPER4512"}},
			wantErr: false,
		},
		{
			name:    "validateName_invalidPercentChar",
			args:    args{comp: &SimpleComponent{Name: "789lower%UPPER4512"}},
			wantErr: true,
		},
		{
			name:    "validateName_invalidSpaceChar",
			args:    args{comp: &SimpleComponent{Name: "789lower UPPER4512"}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateName(tt.args.comp); (err != nil) != tt.wantErr {
				t.Errorf("validateName() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

type testNoSQLDB struct {
	persistBytes []byte
}

func (s *testNoSQLDB) Connect(ctx context.Context, options ...interface{}) error {
	return nil
}
func (s *testNoSQLDB) Disconnect(ctx context.Context, options ...interface{}) error {
	return nil
}
func (s *testNoSQLDB) Insert(ctx context.Context, collection string, document interface{}, args ...interface{}) error {
	return nil
}
func (s *testNoSQLDB) Update(ctx context.Context, collection string, filter interface{}, update interface{}, args ...interface{}) error {
	s.persistBytes, _ = json.Marshal(update)
	return nil
}
func (s *testNoSQLDB) Delete(ctx context.Context, collection string, filter interface{}, args ...interface{}) error {
	return nil
}
func (s *testNoSQLDB) FindOne(ctx context.Context, collection string, filter interface{}, result interface{}, args ...interface{}) error {
	_ = json.Unmarshal(s.persistBytes, result)
	return nil
}
func (s *testNoSQLDB) Find(ctx context.Context, collection string, filter interface{}, args ...interface{}) ([]interface{}, error) {
	return nil, nil
}

func TestContainer_Persistence(t *testing.T) {
	c := &Container{}
	c.Name = "testContainer"
	c.Add(c)

	os.Setenv("KINESIS_DB_CONNECTION", "/home")
	defer os.Unsetenv("KINESIS_DB_CONNECTION")

	os.Setenv("KINESIS_DB_SYMMETRIC_ENCRYPT_KEY", "CAFEBEA")
	defer os.Unsetenv("KINESIS_DB_CONNECTION")

	persistence := new(Persistence)
	persistence.Name = "testFileDB"
	persistence.DB = &testNoSQLDB{}
	AttachComponent(false, persistence)

	type TestPersistComponent struct {
		SimpleComponent
		String1 string `persistable:"native"`
		Int1    int    `persistable:"encrypt"`
	}

	// assign values to TestPersistComponent persistable fields, followed by adding the component
	tc1 := TestPersistComponent{String1: "test string", Int1: 77}
	tc1.Name = "TestPersist"
	c.Add(&tc1)

	_ = tc1.SendSyncMessage(5*time.Second, ControlMsgType, map[interface{}]interface{}{ControlMsgType: Shutdown})

	tc2 := TestPersistComponent{}
	tc2.Name = "TestPersist"
	err := c.Add(&tc2)
	if err != nil {
		t.Error(err)
		t.Fail()
	}

	if tc2.String1 != tc1.String1 {
		t.Errorf("TestContainer_Persistence TestPersistComponent observed String1 = %v, want %v", tc2.String1, tc1.String1)
	}

	if tc2.Int1 != tc1.Int1 {
		t.Errorf("TestContainer_Persistence TestPersistComponent observed Int1 = %v, want %v", tc2.Int1, tc1.Int1)
	}

	shutdownTestContainer(c, 2*time.Second)

}

type TestRestartableSimpleType struct {
	SimpleComponent
	toggleStart bool
	ch          chan struct{}
}

func (d *TestRestartableSimpleType) Start(context.Context) error {
	if !d.toggleStart {
		d.toggleStart = true
		return errors.New("simulating error to start TestRestartableSimpleType")
	}

	d.ch = make(chan struct{})
	<-d.ch
	logger.Info("returning from TestRestartableSimpleType Start()")
	return nil
}

func (d *TestRestartableSimpleType) Stop(context.Context) error {
	close(d.ch)
	logger.Info("closed TestRestartableSimpleType ch")
	return nil
}

func (d *TestRestartableSimpleType) IsRestartableWithDelay() (bool, time.Duration) {
	return true, 1 * time.Second
}

func TestContainer_AddRestartableComponent(t *testing.T) {
	c := &Container{}
	c.Name = "testContainer"
	c.Add(c)

	restartableComponent := &TestRestartableSimpleType{}
	restartableComponent.Name = "restartableTestComponent"

	type args struct {
		comp Component
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "AddRestartableComponent",
			args: args{
				comp: restartableComponent,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := c.Add(tt.args.comp); (err != nil) != tt.wantErr {
				t.Errorf("Container.Add() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
	shutdownTestContainer(c, 5*time.Second)
}

type TestRestartableSimpleType2 struct {
	TestRestartableSimpleType
}

func TestContainer_MonitorRestartableComponentStages(t *testing.T) {
	c := &Container{}
	c.Name = "testContainer"
	c.Add(c)

	restartableComponent := &TestRestartableSimpleType2{}
	restartableComponent.Name = "restartableTestComponentToo"

	changeObjectCh := make(chan ChangeObject, 1)
	restartableComponent.Subscribe("monitorStages", changeObjectCh)
	defer restartableComponent.Unsubscribe("monitorStages")

	// initialize a slice to record the stages
	stages := []stage{}

	go func() {
		for changeObj := range changeObjectCh {
			if stageChangeObject, ok := changeObj.(StageChangeObject); ok {
				prevStage, currStage := stageChangeObject.prevObj, stageChangeObject.currObj
				if len(stages) <= 0 {
					stages = append(stages, prevStage)
				}
				prevStoredStage := stages[len(stages)-1]
				if prevStoredStage == prevStage {
					stages = append(stages, currStage)
				}
			}
		}
	}()

	c.Add(restartableComponent)

	time.Sleep(3 * time.Second)

	type args struct {
		stages []stage
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "MonitorRestartableComponentStages",
			args: args{
				stages: []stage{Submitted, Preinitializing, Preinitialized, Initializing, Initialized, Starting, Started, Restarting, Starting, Started},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i, nxtStage := range tt.args.stages {
				if nxtStage != stages[i] {
					t.Errorf("Wanted stage %v, got %v", nxtStage, stages[i])
				}
			}
		})
	}

	shutdownTestContainer(c, 5*time.Second)
}

func TestContainer_HandleInterrupt(t *testing.T) {
	c := &Container{}
	c.Name = "testContainer"
	c.Add(c)

	type args struct {
		comp Component
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "SimpleComponent_HandleInterrupt",
			args: args{
				comp: &SimpleComponent{Name: "simpleTestComponent"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			if err := c.Add(tt.args.comp); (err != nil) != tt.wantErr {
				t.Errorf("Container.Add() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	time.Sleep(2 * time.Second)
	pid := syscall.Getpid()
	currProcess, _ := os.FindProcess(pid)
	currProcess.Signal(syscall.SIGUSR1)
}

type TestInitiatedSimpleType struct {
	SimpleComponent
}

type TestInitiatingSimpleType struct {
	SimpleComponent
}

func (d *TestInitiatingSimpleType) PreStart(context.Context) error {
	tc := new(TestInitiatedSimpleType)
	tc.Name = "YetAnotherTestSimpleType"
	return d.GetContainer().Add(tc)
}

func TestComponentInitiatingOtherComponent(t *testing.T) {
	c := &Container{}
	c.Name = "testContainer"
	c.Add(c)

	type args struct {
		comp Component
	}

	initiatingComponent := new(TestInitiatingSimpleType)
	initiatingComponent.Name = "InitiatingComponent"

	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "SimpleComponent_HandleInterrupt",
			args: args{
				comp: initiatingComponent,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			if err := c.Add(tt.args.comp); (err != nil) != tt.wantErr {
				t.Errorf("Container.Add() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	shutdownTestContainer(c, 5*time.Second)
}

type TestCallingSimpleType struct {
	SimpleComponent
	RandomNonce int
}

type TestCallerSimpleType struct {
	SimpleComponent
	callBackIndx    int
	randomNonce     int
	callbackInvoked bool
}

func (t *TestCallerSimpleType) testCallback(ctx context.Context, cbIndx int, notification ChangeObject) {
	//assign to callBackIndx, so that it could be removed later
	t.callBackIndx = cbIndx
	// check if callback obtained copy of TestCallingSimpleType.RandomNonce == what had been passed
	switch changeType := notification.(type) {
	case ComponentChangeObject:
		if sType, ok := changeType.GetCurrentObject().(*TestCallingSimpleType); ok && sType.RandomNonce == t.randomNonce {
			t.callbackInvoked = true
		}
	}
}

func (t *TestCallerSimpleType) IsCallbackInvoked() bool {
	return t.callbackInvoked
}

func TestComponentNotifyCallbackListeners(t *testing.T) {
	c := &Container{}
	c.Name = "testContainer"
	c.Add(c)

	type args struct {
		duration   time.Duration
		msgType    interface{}
		msgsLookup map[interface{}]interface{}
	}

	randomNonce := rand.Intn(1000)

	cc1 := new(TestCallingSimpleType)
	cc1.Name = "CallingComponent"
	cc1.RandomNonce = randomNonce
	c.Add(cc1)

	cc2 := new(TestCallerSimpleType)
	cc2.Name = "CallerComponent"
	cc2.randomNonce = randomNonce
	c.Add(cc2)

	// register cc2 for callback from cc1
	cc1.Callback(false, cc2.testCallback)

	tests := []struct {
		name              string
		args              args
		wantErr           bool
		isCallbackSuccess bool
	}{
		{
			name: "SimpleComponent_NotifyCallbackListeners",
			args: args{
				duration:   2 * time.Second,
				msgType:    ControlMsgType,
				msgsLookup: map[interface{}]interface{}{ControlMsgType: NotifyPeers},
			},
			wantErr:           false,
			isCallbackSuccess: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := cc1.SendSyncMessage(tt.args.duration, tt.args.msgType, tt.args.msgsLookup); (err != nil) != tt.wantErr {
				t.Errorf("cc1.SendSyncMessage error = %v, wantErr %v", err, tt.wantErr)
			}

			cc1.RemoveCallback(cc2.callBackIndx)

			if tt.isCallbackSuccess != cc2.IsCallbackInvoked() {
				t.Errorf("cc2.IsCallbackInvoked observed Incc2.IsCallbackInvoked() = %v, want %v", cc2.IsCallbackInvoked(), tt.isCallbackSuccess)
			}
		})
	}

	shutdownTestContainer(c, 5*time.Second)
}

type TestCallerVersionCheckSimpleType struct {
	SimpleComponent
	callBackIndx    int
	prevRandomNonce int
	currRandomNonce int
	callbackInvoked bool
}

func (t *TestCallerVersionCheckSimpleType) testCallback(ctx context.Context, cbIndx int, notification ChangeObject) {
	//assign to callBackIndx, so that it could be removed later
	t.callBackIndx = cbIndx
	// check if callback obtained copy of TestCallingSimpleType.RandomNonce == what had been passed
	switch changeType := notification.(type) {
	case ComponentChangeObject:
		sTypePrev, ok1 := changeType.GetPreviousObject().(*TestCallingSimpleType)
		if !ok1 {
			return
		}
		sTypeCurrent, ok2 := changeType.GetCurrentObject().(*TestCallingSimpleType)
		if !ok2 {
			return
		}
		if sTypePrev.RandomNonce == t.prevRandomNonce && sTypeCurrent.RandomNonce == t.currRandomNonce {
			t.callbackInvoked = true
		}
	}
}

func (t *TestCallerVersionCheckSimpleType) IsCallbackInvoked() bool {
	return t.callbackInvoked
}
func TestNotifyCallbackListenersWithVersionedObject(t *testing.T) {
	c := &Container{}
	c.Name = "testContainer"
	c.Add(c)

	type args struct {
		duration   time.Duration
		msgType    interface{}
		msgsLookup map[interface{}]interface{}
	}

	cc1 := new(TestCallingSimpleType)
	cc1.Name = "CallingComponent"
	c.Add(cc1)

	cc2 := new(TestCallerVersionCheckSimpleType)
	cc2.Name = "CallerComponent"
	c.Add(cc2)

	// register cc2 for callback from cc1
	cc1.Callback(false, cc2.testCallback)

	tests := []struct {
		name              string
		args              args
		wantErr           bool
		isCallbackSuccess bool
	}{
		{
			name: "SimpleComponent_NotifyCallbackListeners_with_init_versioned_object",
			args: args{
				duration:   2 * time.Second,
				msgType:    ControlMsgType,
				msgsLookup: map[interface{}]interface{}{ControlMsgType: NotifyPeers},
			},
			wantErr:           false,
			isCallbackSuccess: true,
		},
		{
			name: "SimpleComponent_NotifyCallbackListeners_with_init_versioned_object",
			args: args{
				duration:   2 * time.Second,
				msgType:    ControlMsgType,
				msgsLookup: map[interface{}]interface{}{ControlMsgType: NotifyPeers},
			},
			wantErr:           false,
			isCallbackSuccess: true,
		},
	}

	// assign nonce to cc1
	randomNonce := rand.Intn(1000)
	cc1.RandomNonce = randomNonce

	// init cc2 prevRandomNonce to cc1's nonce
	cc2.prevRandomNonce = cc1.RandomNonce
	cc2.currRandomNonce = cc1.RandomNonce

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			if err := cc1.SendSyncMessage(tt.args.duration, tt.args.msgType, tt.args.msgsLookup); (err != nil) != tt.wantErr {
				t.Errorf("cc1.SendSyncMessage error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.isCallbackSuccess != cc2.IsCallbackInvoked() {
				t.Errorf("cc2.IsCallbackInvoked observed Incc2.IsCallbackInvoked() = %v, want %v", cc2.IsCallbackInvoked(), tt.isCallbackSuccess)
			}

			// reset for next test case
			tt.isCallbackSuccess = false
			cc2.prevRandomNonce = cc1.RandomNonce

			randomNonce := rand.Intn(1000)
			cc1.RandomNonce = randomNonce
			cc2.currRandomNonce = cc1.RandomNonce
		})
	}

	cc1.RemoveCallback(cc2.callBackIndx)
	shutdownTestContainer(c, 5*time.Second)
}
