package broadcast

import (
	"context"
	"fmt"
	"sync"
)

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

	egress Egress
	// note: The zero value should "work" (i.e. not blow up)
}

type cmdType int

const (
	cmdSubscribe cmdType = iota + 1
	cmdUnsubscribe
	cmdSend
	cmdReceive
)

type broadcastCmd struct {
	kind    cmdType
	payload interface{}
	reply   chan error
}

func (svc *Service) sendCmd(k cmdType, v interface{}, options ...CommandOption) error {
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

	var ack bool
	for _, option := range options {
		//nolint:forcetypeassert
		switch option.Ident() {
		case identAck{}:
			ack = option.Value().(bool)
		}
	}

	svc.mu.RLock()
	defer svc.mu.RUnlock()

	var reply chan error
	if ack {
		reply = make(chan error, 1)
	}
	svc.pending = append(svc.pending, broadcastCmd{
		kind:    k,
		payload: v,
		reply:   reply,
	})
	if svc.running {
		svc.cond.Signal()
	}

	if ack {
		return <-reply
	}
	return nil
}

// Send puts the payload `v` to be broadcast to all subscribers.
func (svc *Service) Send(v interface{}, options ...CommandOption) error {
	return svc.sendCmd(cmdSend, v, options...)
}

// Subscribe registers a subscriber to receive broadcast messages
func (svc *Service) Subscribe(s Subscriber, options ...CommandOption) error {
	return svc.sendCmd(cmdSubscribe, s, options...)
}

// Unsubscribe unregisters a previously registered subscriber
func (svc *Service) Unsubscribe(s Subscriber, options ...CommandOption) error {
	return svc.sendCmd(cmdUnsubscribe, s, options...)
}

// Receive should only be used by whatever ingress service.
// When there is new data coming in from the ingress,
// this method can be used to broadcast the data to the subscribers
func (svc *Service) Receive(v interface{}, options ...CommandOption) error {
	return svc.sendCmd(cmdReceive, v, options...)
}

func (svc *Service) drainPending(ctx context.Context) {
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

// This exists to allow function based subscribers -- functions can't be
// compared using ==
func compareSubscribers(a, b Subscriber) bool {
	switch a := a.(type) {
	case equaler:
		return a.Equal(b)
	default:
		return a == b
	}
}

func (svc *Service) Run(ctx context.Context, options ...RunOption) error {
	var egress Egress
	//nolint:forcetypeassert
	for _, option := range options {
		switch option.Ident() {
		case identEgress{}:
			egress = option.Value().(Egress)
		}
	}

	svc.mu.Lock()
	if egress == nil {
		egress = nilEgress{}
	}
	svc.egress = egress
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
				if v.reply != nil && !found {
					v.reply <- fmt.Errorf(`could not find subscription`)
				}
			case cmdSend:
				_ = svc.egress.Send(v.payload)
			case cmdReceive:
				var errCount int
				for _, sub := range svc.subscribers {
					if err := sub.Receive(v.payload); err != nil {
						errCount++
					}
				}
				if v.reply != nil && errCount > 0 {
					v.reply <- fmt.Errorf(`some receivers failed to receive payload`)
				}
			}
			if v.reply != nil {
				close(v.reply)
			}
		}
	}
}
