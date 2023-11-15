package outbox

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/sns"
)

type mockSNSAPI struct {
	requests []*sns.PublishBatchInput
	SNSAPI
}

func (m *mockSNSAPI) PublishBatch(ctx context.Context, params *sns.PublishBatchInput, optFns ...func(*sns.Options)) (*sns.PublishBatchOutput, error) {
	m.requests = append(m.requests, params)
	return &sns.PublishBatchOutput{}, nil
}

func TestSNSBatcher(t *testing.T) {
	mock := &mockSNSAPI{}
	sb := NewSNSBatcher(mock, "prefix-")

	messages := make([]*Message, 21)
	for idx := range messages {
		messages[idx] = &Message{
			ID:      fmt.Sprintf("id%d", idx),
			Message: []byte(fmt.Sprintf("message %d", idx)),
			Headers: map[string][]string{
				"foo": {fmt.Sprintf("foo%d", idx)},
			},
		}
	}

	if err := sb.SendBatch(context.Background(), "foo", messages); err != nil {
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
}
