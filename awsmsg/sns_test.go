package awsmsg

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_pb"
)

type mockSNSAPI struct {
	requests []*sns.PublishBatchInput
	SNSAPI
}

func (m *mockSNSAPI) PublishBatch(ctx context.Context, params *sns.PublishBatchInput, optFns ...func(*sns.Options)) (*sns.PublishBatchOutput, error) {
	m.requests = append(m.requests, params)
	successes := make([]types.PublishBatchResultEntry, len(params.PublishBatchRequestEntries))
	for idx, entry := range params.PublishBatchRequestEntries {
		successes[idx] = types.PublishBatchResultEntry{
			Id: entry.Id,
		}
	}
	return &sns.PublishBatchOutput{
		Successful: successes,
	}, nil
}

func TestSNSBatcher(t *testing.T) {
	mock := &mockSNSAPI{}
	sb := NewSNSPublisher(mock, "prefix-")

	messages := make([]*messaging_pb.Message, 21)
	for idx := range messages {
		messages[idx] = &messaging_pb.Message{
			MessageId: fmt.Sprintf("id%d", idx),
			Body: &messaging_pb.Any{
				TypeUrl: "type.googleapis.com/o5.deployer.v1.topic.CloudFormationRequestMessage",
				Value:   []byte(fmt.Sprintf("message %d", idx)),
			},
			Headers: map[string]string{
				"foo": fmt.Sprintf("foo%d", idx),
			},
		}
	}

	successIDs, err := sb.PublishBatch(context.Background(), messages)
	if err != nil {
		t.Fatal(err)
	}

	if len(mock.requests) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(mock.requests))
	}
	if len(mock.requests[0].PublishBatchRequestEntries) != 10 {
		t.Fatalf("expected 10 entries, got %d", len(mock.requests[0].PublishBatchRequestEntries))
	}
	if len(mock.requests[1].PublishBatchRequestEntries) != 10 {
		t.Fatalf("expected 10 entries, got %d", len(mock.requests[1].PublishBatchRequestEntries))
	}
	if len(mock.requests[2].PublishBatchRequestEntries) != 1 {
		t.Fatalf("expected 1 entries, got %d", len(mock.requests[2].PublishBatchRequestEntries))
	}

	if len(successIDs) != 21 {
		t.Fatalf("expected 21 success IDs, got %d", len(successIDs))
	}
}
