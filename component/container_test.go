package component

import (
	"context"
	"testing"
	"time"
)

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
			name: "Add_SimpleComponent",
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

	subscribe, errCh := make(chan interface{}, 1), make(chan error, 1)
	c.Subscribe("testS", subscribe)

	go func() {
		time.Sleep(2 * time.Second)
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
