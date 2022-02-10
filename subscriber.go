package pubsub

import "reflect"

type Subscriber interface {
	Receive(interface{}) error
}

type SubscribeFunc func(interface{}) error

func (s SubscribeFunc) Receive(v interface{}) error {
	return s(v)
}

func (s SubscribeFunc) Equal(another Subscriber) bool {
	rv1 := reflect.ValueOf(s)
	rv2 := reflect.ValueOf(another)

	if rv1.Kind() != rv2.Kind() {
		return false
	}

	switch rv1.Kind() {
	case reflect.Func:
		return rv1.Pointer() == rv2.Pointer()
	default:
		return false
	}
}
