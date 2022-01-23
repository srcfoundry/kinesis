package component

import (
	"context"
	"reflect"
	"sync"
	"testing"
)

func TestSimpleComponent_GetNotificationCh(t *testing.T) {
	type fields struct {
		Name           string
		cURI           string
		container      *Container
		stage          stage
		ctx            context.Context
		notificationCh chan interface{}
		MessageCh      chan interface{}
		mutex          *sync.Mutex
	}
	tests := []struct {
		name   string
		fields fields
		want   chan interface{}
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := SimpleComponent{
				Name:           tt.fields.Name,
				cURI:           tt.fields.cURI,
				container:      tt.fields.container,
				stage:          tt.fields.stage,
				notificationCh: tt.fields.notificationCh,
				MessageCh:      tt.fields.MessageCh,
				mutex:          tt.fields.mutex,
			}
			if got := d.GetNotificationCh(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SimpleComponent.GetNotificationCh() = %v, want %v", got, tt.want)
			}
		})
	}
}
