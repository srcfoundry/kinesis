package component

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func shutdownTestContainer(c *Container, delay time.Duration) {
	subscribe, errCh := make(chan interface{}, 1), make(chan error, 1)
	c.Subscribe("testS", subscribe)

	go func() {
		time.Sleep(delay)
		c.Notify(func() (context.Context, interface{}, chan<- error) {
			return nil, Shutdown, errCh
		})
	}()

outer:
	for {
		select {
		case notification := <-subscribe:
			if notification == Stopped {
				break outer
			}
		case <-errCh:
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
	shutdownTestContainer(c, 2*time.Second)
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
			got, err := c.GetComponent(tt.args.name)
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

	newCompCopy, _ := c.GetComponent(testComponentName)
	errCh := make(chan error, 1)

	testComp.Notify(func() (context.Context, interface{}, chan<- error) {
		return nil, newCompCopy, errCh
	})

	type args struct {
		name string
	}

	tests := []struct {
		name            string
		wantErr         bool
		wantIsErrChOpen bool
	}{
		{
			name:            "NotifyValidComponentCopy",
			wantErr:         false,
			wantIsErrChOpen: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err, isOpen := <-errCh
			if (err != nil) != tt.wantErr {
				t.Errorf("NotifyValidComponentCopy  error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if isOpen != tt.wantIsErrChOpen {
				t.Errorf("NotifyValidComponentCopy  isOpen = %v, wantIsErrChOpen %v", isOpen, tt.wantIsErrChOpen)
				return
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
