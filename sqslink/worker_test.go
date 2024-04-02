package sqslink

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/pentops/o5-runtime-sidecar/testproto/gen/testpb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestDeadletter(t *testing.T) {
	// 100% chance of sending a dead letter
	ww := NewWorker(nil, "https://test.com/queue", nil, 0, 100)

	fd := testpb.File_test_v1_test_proto.Services().Get(1).Methods().Get(0)
	if err := ww.registerMethod(context.Background(), fd, nil); err != nil {
		t.Fatal(err.Error())
	}

	msg, handler, err := ww.parseMessage(types.Message{
		MessageAttributes: map[string]types.MessageAttributeValue{
			contentTypeAttribute: {
				DataType:    aws.String("String"),
				StringValue: aws.String("application/json"),
			},
			serviceAttribute: {
				DataType:    aws.String("String"),
				StringValue: aws.String("/test.v1.FooTopic/Foo"),
			},
		},
		Body:      aws.String(`{"name": "test", "id": "asdf"}`),
		MessageId: aws.String("asdf"),
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	if handler == nil {
		t.Fatal("handler is nil")
	}

	want := &testpb.FooMessage{
		Name: "test",
		Id:   "asdf",
	}
	if !proto.Equal(want, msg.Proto) {
		t.Log(protojson.Format(msg.Proto))
		t.Fatalf("Messages do not match")
	}
	// check on deadletter
}

func TestChance(t *testing.T) {
	ctx := context.Background()
	if randomlySelected(ctx, 0) == true {
		t.Fatal("No chance but it happened")
	}
	if randomlySelected(ctx, 100) == false {
		t.Fatal("Guaranteed but didn't happen")
	}
}

func TestWorker(t *testing.T) {
	ww := NewWorker(nil, "https://test.com/queue", nil, 0, 0)

	fd := testpb.File_test_v1_test_proto.Services().Get(1).Methods().Get(0)
	if err := ww.registerMethod(context.Background(), fd, nil); err != nil {
		t.Fatal(err.Error())
	}

	msg, handler, err := ww.parseMessage(types.Message{
		MessageAttributes: map[string]types.MessageAttributeValue{
			contentTypeAttribute: {
				DataType:    aws.String("String"),
				StringValue: aws.String("application/json"),
			},
			serviceAttribute: {
				DataType:    aws.String("String"),
				StringValue: aws.String("/test.v1.FooTopic/Foo"),
			},
		},
		Body:      aws.String(`{"name": "test", "id": "asdf"}`),
		MessageId: aws.String("asdf"),
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	if handler == nil {
		t.Fatal("handler is nil")
	}

	want := &testpb.FooMessage{
		Name: "test",
		Id:   "asdf",
	}
	if !proto.Equal(want, msg.Proto) {
		t.Log(protojson.Format(msg.Proto))
		t.Fatalf("Messages do not match")
	}
}
