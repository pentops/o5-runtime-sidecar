package sqslink

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/pentops/o5-runtime-sidecar/testproto/gen/testpb"
	"github.com/stretchr/testify/assert"
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

func TestParser(t *testing.T) {

	for _, tc := range []struct {
		name  string
		input types.Message
		want  O5Message
	}{{
		name: "simple",
		input: types.Message{

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
		},
		want: O5Message{
			ContentType:  "application/json",
			GrpcService:  "test.v1.FooTopic",
			GrpcMethod:   "Foo",
			SQSMessageID: "asdf",
			Message:      []byte(`{"name": "test", "id": "asdf"}`),
		},
	}, {
		name: "sns-wrapped",
		input: types.Message{
			MessageAttributes: map[string]types.MessageAttributeValue{
				serviceAttribute: {
					DataType:    aws.String("String"),
					StringValue: aws.String("/test.v1.FooTopic/Foo"),
				},
			},
			MessageId: aws.String("asdf"),
			Body:      aws.String(`{"Message": "{\"name\": \"test\", \"id\": \"asdf\"}"}`),
		},
		want: O5Message{
			ContentType:  "application/json",
			GrpcService:  "test.v1.FooTopic",
			GrpcMethod:   "Foo",
			SQSMessageID: "asdf",
			Message:      []byte(`{"name": "test", "id": "asdf"}`),
		},
	}, {
		name: "o5-wrapped",
		input: types.Message{
			MessageAttributes: map[string]types.MessageAttributeValue{},
			MessageId:         aws.String("asdf"),
			Body: aws.String(`{
				"detail-type": "o5-message",
				"detail": {
					"grpc-service": "test.v1.FooTopic",
					"grpc-method": "Foo",
					"content-type": "application/proto",
					"body": "Rk9PQkFS"
				}
			}`),
		},
		want: O5Message{
			ContentType:  "application/proto",
			GrpcService:  "test.v1.FooTopic",
			GrpcMethod:   "Foo",
			SQSMessageID: "asdf",
			Message:      []byte(`FOOBAR`),
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {

			msg, err := parseMessage(tc.input)
			if err != nil {
				t.Fatal(err.Error())
			}

			assert.Equal(t, tc.want.ContentType, msg.ContentType)
			assert.Equal(t, tc.want.GrpcService, msg.GrpcService)
			assert.Equal(t, tc.want.GrpcMethod, msg.GrpcMethod)
			assert.Equal(t, tc.want.SQSMessageID, msg.SQSMessageID)
			assert.Equal(t, string(tc.want.Message), string(msg.Message))

			if tc.want.ID != "" {
				assert.Equal(t, tc.want.ID, msg.ID)
			} else {
				assert.NotEqual(t, msg.SQSMessageID, msg.ID)
			}

		})
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

	asProto, err := handler.parseMessageBody("application/json", []byte(`{"name": "test", "id": "asdf"}`))
	if err != nil {
		t.Fatal(err.Error())
	}

	if !proto.Equal(want, asProto) {
		t.Log(protojson.Format(asProto))
		t.Fatalf("Messages do not match")
	}

}
