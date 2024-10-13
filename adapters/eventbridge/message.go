package eventbridge

import (
	"context"

	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_pb"
)

type Publisher interface {
	PublisherID() string
	Publish(ctx context.Context, msg *messaging_pb.Message) error
	PublishBatch(ctx context.Context, msgs []*messaging_pb.Message) ([]string, error)
}
