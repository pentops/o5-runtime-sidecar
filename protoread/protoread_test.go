package protoread

import (
	"context"
	"testing"

	"github.com/pentops/flowtest"
	"github.com/pentops/o5-runtime-sidecar/testproto/gen/testpb"
	"google.golang.org/grpc/reflection"
)

type Service struct {
	testpb.UnimplementedFooServiceServer
}

func TestProtoReadHappy(t *testing.T) {
	ctx := context.Background()

	grpcPair := flowtest.NewGRPCPair(t)

	service := &Service{}
	testpb.RegisterFooServiceServer(grpcPair.Server, service)
	reflection.Register(grpcPair.Server)

}
