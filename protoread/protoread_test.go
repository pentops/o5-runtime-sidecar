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
	t.Skip("Currently broken")
	grpcPair := flowtest.NewGRPCPair(t)

	service := &Service{}
	testpb.RegisterFooServiceServer(grpcPair.Server, service)
	reflection.Register(grpcPair.Server)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	grpcPair.ServeUntilDone(t, ctx)

	desc, err := FetchServices(ctx, grpcPair.Client)
	if err != nil {
		t.Fatal(err)
	}

	t.Log(desc)

	if len(desc) != 1 {
		t.Fatal("expected one service")
	}

	if desc[0].Name() != "FooService" {
		t.Fatal("expected FooService")
	}
}
