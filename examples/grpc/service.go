package grpcexample

import (
	"context"
	"net"

	"github.com/lestrrat-go/broadcast"
	pb "github.com/lestrrat-go/broadcast/examples/grpc/pb"
	"google.golang.org/grpc"
)

type Service struct {
	svc *broadcast.Service
	pb.UnimplementedBroadcasterServer
}

func (s *Service) Broadcast(ctx context.Context, req *pb.BroadcastRequest) (*pb.BroadcastResponse, error) {
	err := s.svc.Receive(req.Message)
	return &pb.BroadcastResponse{Success: err == nil}, nil
}

func (s *Service) Run(ctx context.Context, svc *broadcast.Service, l net.Listener) {
	s.svc = svc
	server := grpc.NewServer()

	pb.RegisterBroadcasterServer(server, s)
	go server.Serve(l)

	<-ctx.Done()

	server.GracefulStop()
}
