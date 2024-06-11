package sqslink

import (
	"context"
	"testing"

	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_pb"
	"github.com/pentops/o5-runtime-sidecar/testproto/gen/testpb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestChance(t *testing.T) {
	ctx := context.Background()
	if randomlySelected(ctx, 0) == true {
		t.Fatal("No chance but it happened")
	}
	if randomlySelected(ctx, 100) == false {
		t.Fatal("Guaranteed but didn't happen")
	}
}

func TestDynamicParse(t *testing.T) {
	ww := NewWorker(nil, "https://test.com/queue", nil, 0)

	fd := testpb.File_test_v1_test_proto.Services().Get(1)
	if err := ww.RegisterService(context.Background(), fd, nil); err != nil {
		t.Fatal(err.Error())
	}

	handler, ok := ww.services["/test.v1.FooTopic/Foo"].(*service)
	if !ok || handler == nil {
		t.Fatal("handler is nil")
	}

	want := &testpb.FooMessage{
		Name: "test",
		Id:   "asdf",
	}

	asProto, err := handler.parseMessageBody(&messaging_pb.Message{
		Body: &messaging_pb.Any{
			Encoding: messaging_pb.WireEncoding_PROTOJSON,
			Value:    []byte(`{"name": "test", "id": "asdf"}`),
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	if !proto.Equal(want, asProto) {
		t.Log(protojson.Format(asProto))
		t.Fatalf("Messages do not match")
	}

}
