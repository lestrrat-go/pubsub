package broadcast_test

import (
	"context"
	"testing"
	"time"

	"github.com/lestrrat-go/broadcast"
	"github.com/stretchr/testify/assert"
)

func TestService(t *testing.T) {
	var svc broadcast.Service

	sendMsgs := []interface{}{
		"Hello", 1, 'W', 'o', 'r', 'l', 'd', 3.14,
	}
	var msgs1, msgs2 []interface{}

	sub1 := broadcast.SubscribeFunc(func(v interface{}) {
		msgs1 = append(msgs1, v)
	})
	sub2 := broadcast.SubscribeFunc(func(v interface{}) {
		msgs2 = append(msgs2, v)
	})
	svc.Subscribe(sub1)
	svc.Send(sendMsgs[0])

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	ready := make(chan struct{})
	go func(ready chan struct{}) {
		defer close(ready)
		svc.Run(ctx)
	}(ready)

	svc.Subscribe(sub2)
	for i := 1; i < len(sendMsgs)-1; i++ {
		svc.Send(sendMsgs[i])
	}
	svc.Unsubscribe(sub2)
	svc.Send(sendMsgs[len(sendMsgs)-1])

	<-ready

	if !assert.Equal(t, msgs1, sendMsgs, `msgs1 should contain all messages in order`) {
		t.Logf("%#v", msgs1)
		return
	}

	if !assert.Equal(t, msgs2, sendMsgs[1:len(sendMsgs)-1], `msgs2 should contain everything except for first and last`) {
		t.Logf("%#v", msgs2)
		return
	}

}
