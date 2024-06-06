package awsmsg

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/pentops/o5-go/messaging/v1/messaging_pb"
	"github.com/stretchr/testify/assert"
)

func TestParser(t *testing.T) {

	for _, tc := range []struct {
		name  string
		input types.Message
		want  *messaging_pb.Message
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
		want: &messaging_pb.Message{
			GrpcService: "test.v1.FooTopic",
			GrpcMethod:  "Foo",
			Body: &messaging_pb.Any{
				Encoding: messaging_pb.WireEncoding_PROTOJSON,
				Value:    []byte(`{"name": "test", "id": "asdf"}`),
			},
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
			Body: aws.String(`{
				"Message": "{\"name\": \"test\", \"id\": \"asdf\"}",
				"MessageId": "asdf",
				"Type": "Notification",
				"TopicArn": "arn:aws:sns:us-west-2:123456789012:MyTopic"
			}`),
		},
		want: &messaging_pb.Message{
			GrpcService: "test.v1.FooTopic",
			GrpcMethod:  "Foo",
			Body: &messaging_pb.Any{
				Encoding: messaging_pb.WireEncoding_PROTOJSON,
				Value:    []byte(`{"name": "test", "id": "asdf"}`),
			},
		},
	}, {
		name: "sns-raw",
		input: types.Message{
			MessageId: aws.String("asdf"),
			Body: aws.String(`{
				"Message": "Hello World!",
				"MessageId": "asdf",
				"Type": "Notification",
				"TopicArn": "arn:aws:sns:us-west-2:123456789012:MyTopic",
				"Timestamp": "2012-03-29T05:12:16.901Z"
			}`),
		},
		want: &messaging_pb.Message{
			GrpcService:      "o5.messaging.v1.topic.RawMessageTopic",
			GrpcMethod:       "Raw",
			DestinationTopic: "MyTopic",
			Body: &messaging_pb.Any{
				Encoding: messaging_pb.WireEncoding_RAW,
				Value:    []byte(`Hello World!`),
			},
		},
	}, {
		name: "o5-wrapped",
		input: types.Message{
			MessageAttributes: map[string]types.MessageAttributeValue{},
			MessageId:         aws.String("asdf"),
			Body: aws.String(`{
				"detail-type": "o5-message/v1",
				"detail": {
					"grpcService": "test.v1.FooTopic",
					"grpcMethod": "Foo",
					"body": {
						"typeUrl": "type.googleapis.com/o5.deployer.v1.topic.CloudFormationRequestMessage",
						"value": "Rk9PQkFS"
					}
				}
			}`),
		},
		want: &messaging_pb.Message{
			GrpcService: "test.v1.FooTopic",
			GrpcMethod:  "Foo",
			Body: &messaging_pb.Any{
				Encoding: messaging_pb.WireEncoding_UNSPECIFIED,
				TypeUrl:  "type.googleapis.com/o5.deployer.v1.topic.CloudFormationRequestMessage",
				Value:    []byte(`FOOBAR`),
			},
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {

			msg, err := ParseSQSMessage(tc.input)
			if err != nil {
				t.Fatal(err.Error())
			}

			assert.Equal(t, tc.want.Body.Encoding.String(), msg.Body.Encoding.String(), "Encoding")
			assert.Equal(t, tc.want.Body.TypeUrl, msg.Body.TypeUrl)
			assert.Equal(t, tc.want.GrpcService, msg.GrpcService)
			assert.Equal(t, tc.want.GrpcMethod, msg.GrpcMethod)
			assert.Equal(t, string(tc.want.Body.Value), string(msg.Body.Value))

			if tc.want.MessageId != "" {
				assert.Equal(t, tc.want.MessageId, msg.MessageId)
			}

		})
	}
}
