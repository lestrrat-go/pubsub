package pubsub

import (
	"context"
	"fmt"
	"sync"
)

// Service is responsible for accepting a payload, and
// pubsubing it to all data sinks. It is expected that
// the data sinks do not block more than absolutely necessary
type Service struct {
	// "shared" variables. These can be accessed from the user-facing API
	running bool
	mu      sync.RWMutex
	cond    *sync.Cond
	pending []pubsubCmd

	// "private" variables. Can only be accessed from the internal structures.
	control     chan pubsubCmd
	data        chan pubsubCmd
	subscribers []Subscriber

	// note: The zero value should "work" (i.e. not blow up)
}

type cmdType int

const (
	cmdSubscribe cmdType = iota + 1
	cmdUnsubscribe
	cmdReceive
)

type pubsubCmd struct {
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
	svc.pending = append(svc.pending, pubsubCmd{
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

// Subscribe registers a subscriber to receive pubsub messages
func (svc *Service) Subscribe(s Subscriber, options ...CommandOption) error {
	return svc.sendCmd(cmdSubscribe, s, options...)
}

// Unsubscribe unregisters a previously registered subscriber
func (svc *Service) Unsubscribe(s Subscriber, options ...CommandOption) error {
	return svc.sendCmd(cmdUnsubscribe, s, options...)
}

// Receive should only be used by whatever ingress service.
// When there is new data coming in from the ingress,
// this method can be used to pubsub the data to the subscribers
func (svc *Service) Receive(v interface{}, options ...CommandOption) error {
	return svc.sendCmd(cmdReceive, v, options...)
}

// defines the maximum number of commands that can be processed in one
// batch withing draingPending().
const bufferProcessingSize = 32

func (svc *Service) drainPending(ctx context.Context, drained chan struct{}) {
	defer func() {
		close(drained)
	}()
	pending := make([]pubsubCmd, 0, bufferProcessingSize)
	for {
		svc.cond.L.Lock()
		for len(svc.pending) <= 0 {
			// if nothing is pending, test if we're done and bail
			select {
			case <-ctx.Done():
				svc.cond.L.Unlock()
				return
			default:
			}
			// nothing to process... wait for incoming work
			svc.cond.Wait()
		}

		// copy over the pending queue so we can release the lock
		if l := len(svc.pending); l < bufferProcessingSize {
			pending = pending[:l]
		} else {
			pending = pending[:bufferProcessingSize]
		}
		n := copy(pending, svc.pending)

		// reduce the pending queue
		svc.pending = svc.pending[n:]

		// after this unlock, users can add more commands
		svc.cond.L.Unlock()

		// work on the local buffer. This needs no locking
		for _, v := range pending {
			switch v.kind {
			case cmdSubscribe, cmdUnsubscribe:
				select {
				case <-ctx.Done():
					return
				case svc.control <- v:
				}
			case cmdReceive:
				select {
				case <-ctx.Done():
					return
				case svc.data <- v:
				}
			}
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

func (svc *Service) Run(ctx context.Context) error {
	const commandBufferSize = 16
	svc.mu.Lock()
	svc.cond = sync.NewCond(&svc.mu)
	svc.control = make(chan pubsubCmd, commandBufferSize)
	svc.data = make(chan pubsubCmd, commandBufferSize)
	svc.running = true
	svc.mu.Unlock()

	drained := make(chan struct{})
	go svc.drainPending(ctx, drained)
	for done := false; !done; {
		done = svc.process(ctx.Done())
	}

	// if we got here, <-ctx.Done() returned. Make sure to draing
	// the remaining commands before we call it quits
	for len(svc.data) > 0 && len(svc.control) > 0 {
		_ = svc.process(nil)
	}

	// Cleanup
	svc.mu.Lock()
	svc.running = false
	close(svc.data)
	close(svc.control)
	svc.data = nil
	svc.control = nil
	svc.mu.Unlock()

	// Signal the cond var one last time. This will make sure that the
	// drainPending() goroutine wakes up and notices that the code path
	// for <-ctx.Done()
	svc.cond.Signal()

	// Make absolutely sure we have cleaned up the drainPending
	// goroutine by waiting on this channel
	<-drained
	return nil
}

func (svc *Service) process(done <-chan struct{}) bool {
	select {
	case <-done:
		return true
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
		}
	case v := <-svc.data:
		switch v.kind {
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
	return false
}
