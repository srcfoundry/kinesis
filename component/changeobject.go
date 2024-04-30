package component

import "fmt"

// Immutable copies of the previous and current objects of any type could be encapsulated as ChangeObject interface,
// which serves as the data format for message processing. The main ChangeObjects are StateChangeObject, StageChangeObject
// & ComponentChangeObject
type ChangeObject interface {
	GetPreviousObject() interface{}
	GetCurrentObject() interface{}
	fmt.Stringer
}

// inner changeObject struct is implemented to generically handle any type and provide the methods required by the ChangeObject interface.
type changeObject[T any] struct {
	prevObj T
	currObj T
}

// for string marshalling
type changeObjStrFmt struct {
	Type     string
	Previous struct {
		Value interface{}
	}
	Current struct {
		Value interface{}
	}
}

func (c changeObject[T]) String() string {
	changeObjFmt := changeObjStrFmt{
		Type:     fmt.Sprintf("%T", c.currObj),
		Previous: struct{ Value interface{} }{c.prevObj},
		Current:  struct{ Value interface{} }{c.currObj},
	}

	return fmt.Sprintf("%+v", changeObjFmt)
}

func (c changeObject[T]) GetPreviousObject() interface{} {
	return c.prevObj
}

func (c changeObject[T]) GetCurrentObject() interface{} {
	return c.currObj
}

type StateChangeObject struct {
	changeObject[state]
}

type StageChangeObject struct {
	changeObject[stage]
}

type ComponentChangeObject struct {
	changeObject[Component]
}
