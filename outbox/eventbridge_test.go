package outbox

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
	"github.com/google/uuid"
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

	publisher := NewEventBridgePublisher(eventbridgeClient, "EVENTBRIDGE_ARN", "SOURCE", "ENV")

	ctx := context.Background()
	res, err := publisher.SendMultiBatch(ctx, []*Message{{
		ID:          "message-id",
		Message:     []byte("test"),
		Destination: "test-topic",
		GrpcService: "o5.deployer.v1.topic.CloudFormationRequestTopic",
		GrpcMethod:  "Test",
		GrpcMessage: "test.v1.TestMessage",
		ContentType: "application/proto",
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

	if *entry.Source != "SOURCE" {
		t.Fatalf("expected SOURCE, got %s", *entry.Source)
	}

	if *entry.DetailType != "o5-message" {
		t.Fatalf("expected o5-message, got %s", *entry.DetailType)
	}

	if *entry.EventBusName != "EVENTBRIDGE_ARN" {
		t.Fatalf("expected EVENTBRIDGE_ARN, got %s", *entry.EventBusName)
	}

	detail := map[string]interface{}{}
	if err := json.Unmarshal([]byte(*entry.Detail), &detail); err != nil {
		t.Fatal(err.Error())
	}

	if detail["grpc-service"] != "o5.deployer.v1.topic.CloudFormationRequestTopic" {
		t.Fatalf("expected o5.deployer.v1.topic.CloudFormationRequestTopic, got %s", detail["grpc-service"])
	}

}
