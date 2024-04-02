package sqslink

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/pentops/o5-runtime-sidecar/outbox"
	"github.com/pentops/o5-runtime-sidecar/testproto/gen/testpb"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type deadletterHandler struct {
	msg outbox.RoutableMessage
}

func (d *deadletterHandler) SendOne(ctx context.Context, ob outbox.RoutableMessage) error {
	d.msg = ob
	return nil
}

type fakeInvoker struct{}

func (i *fakeInvoker) Invoke(ctx context.Context, f string, q interface{}, e interface{}, z ...grpc.CallOption) error {
	return nil
}

type fakeSQS struct{}

func (i *fakeSQS) ReceiveMessage(ctx context.Context, input *sqs.ReceiveMessageInput, opts ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	return nil, nil
}
func (i *fakeSQS) DeleteMessage(ctx context.Context, input *sqs.DeleteMessageInput, opts ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error) {
	return nil, nil
}

func TestDeadletter(t *testing.T) {
	dlhandler := deadletterHandler{}
	fakeSqs := fakeSQS{}
	// 100% chance of sending a dead letter
	ww := NewWorker(&fakeSqs, "https://test.com/queue", &dlhandler, 0, 100)

	fd := testpb.File_test_v1_test_proto.Services().Get(1).Methods().Get(0)
	if err := ww.registerMethod(context.Background(), fd, nil); err != nil {
		t.Fatal(err.Error())
	}

	ww.RegisterService(context.Background(), testpb.File_test_v1_test_proto.Services().Get(1), &fakeInvoker{})

	m := sqstypes.Message{
		MessageAttributes: map[string]sqstypes.MessageAttributeValue{
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
	}

	ww.handleMessage(context.Background(), m)

	if dlhandler.msg == nil {
		t.Fatal("Expected a dead letter, got none")
	}
	t.Logf("msg is %v", dlhandler.msg)
	t.Fatal()
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

	msg, handler, err := ww.parseMessage(sqstypes.Message{
		MessageAttributes: map[string]sqstypes.MessageAttributeValue{
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
