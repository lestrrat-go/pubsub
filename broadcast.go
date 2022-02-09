package broadcast

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"sync"
)

// Subscriber defines the interface to
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

// Service is responsible for accepting a payload, and
// broadcasting it to all data sinks. It is expected that
// the data sinks do not block more than absolutely necessary
type Service struct {
	running     bool
	mu          sync.RWMutex
	cond        *sync.Cond
	control     chan broadcastCmd
	pending     []broadcastCmd
	subscribers []Subscriber
}

type cmdType int

const (
	cmdSubscribe cmdType = iota + 1
	cmdUnsubscribe
	cmdSend
)

type broadcastCmd struct {
	kind    cmdType
	payload interface{}
	reply   chan error
}

func (svc *Service) sendCmd(k cmdType, v interface{}) chan error {
	// The commands are not processed until Run() is called. Instead
	// they are buffered in the "pending" slice.
	//
	// In order to reduce locking contention, actual modification
	// of the Service object (other than .pending and .running variables)
	// is _ONLY_ done within the Run() method.
	//
	// There's an intermediary whose sole purpose is to drain the
	// .pending queue, such that Run() can accept new commands in
	// the Run() method.
	//
	// Within Run(), the Service object just sits and waits until it's
	// notified by the condition variable -- once it gets a notification
	// it processes the commands one by one
	svc.mu.RLock()
	defer svc.mu.RUnlock()

	reply := make(chan error, 1)
	svc.pending = append(svc.pending, broadcastCmd{
		kind:    k,
		payload: v,
		reply:   reply,
	})
	if svc.running {
		svc.cond.Signal()
	}
	return reply
}

// Send puts the payload `v` to be broadcast to all subscribers
func (svc *Service) Send(v interface{}) error {
	return <-svc.sendCmd(cmdSend, v)
}

// Subscribe registers a subscriber to receive broadcast messages
func (svc *Service) Subscribe(s Subscriber) error {
	return <-svc.sendCmd(cmdSubscribe, s)
}

// Unsubscribe unregisters a previously registered subscriber
func (svc *Service) Unsubscribe(s Subscriber) error {
	return <-svc.sendCmd(cmdUnsubscribe, s)
}

func (svc *Service) drainPending(ctx context.Context) {
	defer log.Printf("stop draining...")
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		svc.cond.L.Lock()
		for len(svc.pending) <= 0 {
			svc.cond.Wait()
		}

		v := svc.pending[0]
		svc.pending = svc.pending[1:]
		svc.cond.L.Unlock()

		select {
		case <-ctx.Done():
			return
		case svc.control <- v:
		}
	}
}

type equaler interface {
	Equal(Subscriber) bool
}

func compareSubscribers(a, b Subscriber) bool {
	switch a := a.(type) {
	case equaler:
		return a.Equal(b)
	default:
		return a == b
	}
}

func (svc *Service) Run(ctx context.Context) error {
	svc.mu.Lock()
	svc.cond = sync.NewCond(&svc.mu)
	svc.control = make(chan broadcastCmd)
	svc.running = true
	svc.mu.Unlock()

	defer func() {
		svc.mu.Lock()
		svc.running = false
		svc.mu.Unlock()
	}()

	go svc.drainPending(ctx)

	for {
		select {
		case <-ctx.Done():
			return nil
		case v := <-svc.control:
			switch v.kind {
			case cmdSubscribe:
				svc.subscribers = append(svc.subscribers, v.payload.(Subscriber))
			case cmdUnsubscribe:
				var found bool
				for i, sub := range svc.subscribers {
					if compareSubscribers(sub, v.payload.(Subscriber)) {
						found = true
						svc.subscribers = append(svc.subscribers[:i], svc.subscribers[i+1:]...)
						break
					}
				}
				if !found {
					v.reply <- fmt.Errorf(`could not find subscription`)
				}
			case cmdSend:
				var errCount int
				for _, sub := range svc.subscribers {
					if err := sub.Receive(v.payload); err != nil {
						errCount++
					}
				}
				v.reply <- fmt.Errorf(`some receivers failed to receive payload`)
			}
			close(v.reply)
		}
	}
}
