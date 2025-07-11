package grpcreflect

import (
	"testing"

	"github.com/pentops/flowtest"
	"github.com/pentops/o5-runtime-sidecar/testproto/gen/test/v1/test_spb"
	"google.golang.org/grpc/reflection"
)

type Service struct {
	test_spb.UnimplementedFooServiceServer
}

func TestProtoReadHappy(t *testing.T) {
	grpcPair := flowtest.NewGRPCPair(t)

	service := &Service{}
	test_spb.RegisterFooServiceServer(grpcPair.Server, service)
	reflection.Register(grpcPair.Server)

	ctx := t.Context()

	grpcPair.ServeUntilDone(t, ctx)

	cl := NewClient(grpcPair.Client)

	desc, err := cl.FetchServices(ctx)
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
