package grpcexample_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/lestrrat-go/pubsub"
	grpcexample "github.com/lestrrat-go/pubsub/examples/grpc"
	pb "github.com/lestrrat-go/pubsub/examples/grpc/pb"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
)

func TestService(t *testing.T) {
	var svc pubsub.Service
	var ingress grpcexample.Service

	const bufSize = 1024 * 1024

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	l := bufconn.Listen(bufSize)
	go ingress.Run(ctx, &svc, l)
	go svc.Run(ctx)

	var msgs1 []interface{}

	sub1 := pubsub.SubscribeFunc(func(v interface{}) error {
		msgs1 = append(msgs1, v)
		return nil
	})
	_ = svc.Subscribe(sub1)

	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		return l.DialContext(ctx)
	}), grpc.WithInsecure())
	if !assert.NoError(t, err, `grpc.DialContext should succeed`) {
		return
	}

	defer conn.Close()

	client := pb.NewBroadcasterClient(conn)

	sendMsgs := []string{`Hello!`, `World`}
	var expected []interface{}

	for _, payload := range sendMsgs {
		resp, err := client.Broadcast(ctx, &pb.BroadcastRequest{Message: payload})
		if !assert.NoError(t, err, `client.Broadcast should succeed`) {
			return
		}

		if !assert.True(t, resp.GetSuccess(), `resp.Success should be true`) {
			return
		}

		expected = append(expected, payload)
	}

	time.Sleep(time.Second)

	if !assert.Equal(t, msgs1, expected) {
		return
	}

}
