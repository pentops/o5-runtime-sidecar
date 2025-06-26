package eventbridge

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
	"github.com/google/uuid"
	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_pb"
	"github.com/stretchr/testify/assert"
)

type MockEventBridgeAPI struct {
	putEvents func(params *eventbridge.PutEventsInput) (*eventbridge.PutEventsOutput, error)
}

func (mock *MockEventBridgeAPI) PutEvents(ctx context.Context, params *eventbridge.PutEventsInput, optFns ...func(*eventbridge.Options)) (*eventbridge.PutEventsOutput, error) {
	return mock.putEvents(params)
}

func TestEventBridge(t *testing.T) {
	gotEntries := make([]types.PutEventsRequestEntry, 0)

	eventbridgeClient := &MockEventBridgeAPI{
		putEvents: func(params *eventbridge.PutEventsInput) (*eventbridge.PutEventsOutput, error) {
			events := make([]types.PutEventsResultEntry, len(params.Entries))
			for i, entry := range params.Entries {
				gotEntries = append(gotEntries, entry)
				events[i] = types.PutEventsResultEntry{
					EventId: aws.String(uuid.NewString()),
				}
			}
			return &eventbridge.PutEventsOutput{
				Entries: events,
			}, nil
		},
	}

	publisher, err := NewEventBridgePublisher(eventbridgeClient, EventBridgeConfig{
		BusARN: "EVENTBRIDGE_ARN",
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	ctx := context.Background()
	res, err := publisher.PublishBatch(ctx, []*messaging_pb.Message{{
		MessageId: "message-id",
		Body: &messaging_pb.Any{
			TypeUrl: "type.googleapis.com/o5.deployer.v1.topic.CloudFormationRequestMessage",
			Value:   []byte("test"),
		},
		DestinationTopic: "test-topic",
		GrpcService:      "o5.deployer.v1.topic.CloudFormationRequestTopic",
		GrpcMethod:       "Test",
		SourceApp:        "SOURCE",
		SourceEnv:        "ENV",
		Headers: map[string]string{
			"key": "val",
		},
	}})
	if err != nil {
		t.Fatal(err.Error())
	}

	if len(res) != 1 {
		t.Fatalf("expected 1 result, got %d", len(res))
	}

	if res[0] != "message-id" {
		t.Fatalf("expected message-id, got %s", res[0])
	}

	t.Log(res)

	if len(gotEntries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(gotEntries))
	}

	entry := gotEntries[0]

	assert.NotNil(t, entry.Source)
	assert.Equal(t, "o5/ENV/SOURCE", *entry.Source)

	assert.NotNil(t, entry.DetailType)
	assert.Equal(t, "o5-message/v1", *entry.DetailType)

	assert.NotNil(t, entry.EventBusName)
	assert.Equal(t, "EVENTBRIDGE_ARN", *entry.EventBusName)

	detail := map[string]any{}
	if err := json.Unmarshal([]byte(*entry.Detail), &detail); err != nil {
		t.Fatal(err.Error())
	}

	assert.Equal(t, "Test", detail["grpcMethod"])
	assert.Equal(t, "o5.deployer.v1.topic.CloudFormationRequestTopic", detail["grpcService"])

}
