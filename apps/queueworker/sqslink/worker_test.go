package sqslink

import (
	"context"
	"testing"

	"github.com/pentops/j5/gen/j5/messaging/v1/messaging_j5pb"
	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_pb"
	"github.com/pentops/o5-messaging/o5msg"
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

	handler, ok := ww.handlers["/test.v1.FooTopic/Foo"].(*service)
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

func TestRequestMetadata(t *testing.T) {
	ww := NewWorker(nil, "https://test.com/queue", nil, 0)

	fd := testpb.File_test_v1_test_proto.Services().ByName("RequestTopic")

	if err := ww.RegisterService(context.Background(), fd, nil); err != nil {
		t.Fatal(err.Error())
	}

	handler, ok := ww.handlers["/test.v1.RequestTopic/Request"].(*service)
	if !ok || handler == nil {
		t.Fatal("handler is nil")
	}

	input := &testpb.RequestMessage{
		Request: &messaging_j5pb.RequestMetadata{
			ReplyTo: "prior",
			Context: []byte("value"),
		},
	}

	// emulate outbox.Send()
	wrapper, err := o5msg.WrapMessage(input)
	if err != nil {
		t.Fatal(err.Error())
	}

	// sidecar sender
	reqExt := wrapper.GetRequest()
	if reqExt == nil {
		t.Fatal("no request extension")
	}
	reqExt.ReplyTo = "injected"

	asProto, err := handler.parseMessageBody(wrapper)
	if err != nil {
		t.Fatal(err.Error())
	}

	want := &testpb.RequestMessage{
		Request: &messaging_j5pb.RequestMetadata{
			ReplyTo: "injected",
			Context: []byte("value"),
		},
	}
	if !proto.Equal(want, asProto) {
		t.Log(protojson.Format(want))
		t.Log(protojson.Format(asProto))
		t.Fatalf("Messages do not match")
	}
}
