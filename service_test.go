package pubsub_test

import (
	"context"
	"testing"
	"time"

	"github.com/lestrrat-go/pubsub"
	"github.com/stretchr/testify/assert"
)

func TestService(t *testing.T) {
	t.Run("Receive before looping should not be an error", func(t *testing.T) {
		var svc pubsub.Service

		if !assert.NoError(t, svc.Receive(`Hello`, pubsub.WithAck(false)), `Sending before subscribing should not be an error`) {
			return
		}
	})
	t.Run("Multiple messages, multiple subscribers", func(t *testing.T) {
		var svc pubsub.Service

		sendMsgs := []interface{}{
			"Hello", 1, 'W', 'o', 'r', 'l', 'd', 3.14,
		}
		var msgs1, msgs2 []interface{}

		sub1 := pubsub.SubscribeFunc(func(v interface{}) error {
			msgs1 = append(msgs1, v)
			return nil
		})
		sub2 := pubsub.SubscribeFunc(func(v interface{}) error {
			msgs2 = append(msgs2, v)
			return nil
		})

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		ready := make(chan struct{})
		go func(ready chan struct{}) {
			defer close(ready)
			_ = svc.Run(ctx)
		}(ready)

		_ = svc.Subscribe(sub1)
		_ = svc.Subscribe(sub2)

		time.Sleep(100 * time.Millisecond)

		l := pubsub.NewLoopback(&svc)
		for i := 0; i < len(sendMsgs); i++ {
			_ = l.Send(sendMsgs[i])
		}

		<-ready

		_ = svc.Unsubscribe(sub2)
		_ = svc.Unsubscribe(sub1)

		if !assert.Equal(t, msgs1, sendMsgs, `msgs1 should contain all messages in order`) {
			t.Logf("%#v", msgs1)
			return
		}

		if !assert.Equal(t, msgs2, sendMsgs, `msgs2 should contain all messages in order`) {
			t.Logf("%#v", msgs2)
			return
		}
	})
}
