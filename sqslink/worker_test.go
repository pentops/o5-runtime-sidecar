package sqslink

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/pentops/o5-runtime-sidecar/testproto/gen/testpb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestWorker(t *testing.T) {

	ww := NewWorker(nil, "https://test.com/queue")

	fd := testpb.File_test_v1_test_proto.Services().Get(1).Methods().Get(0)
	if err := ww.registerMethod(fd, nil); err != nil {
		t.Fatal(err.Error())
	}

	handler, msg, err := ww.parseMessage(types.Message{
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
		Body: aws.String(`{"name": "test", "id": "asdf"}`),
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
	if !proto.Equal(want, msg) {
		t.Log(protojson.Format(msg))
		t.Fatalf("Messages do not match")
	}

}
