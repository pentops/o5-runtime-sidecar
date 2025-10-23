package messaging

import (
	"context"

	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_pb"
	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_tpb"
	"github.com/pentops/o5-messaging/o5msg"
	"github.com/pentops/o5-runtime-sidecar/sidecar"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type DeadLetterHandler interface {
	DeadMessage(context.Context, *messaging_tpb.DeadMessage) error
}

type Publisher interface {
	Publish(ctx context.Context, msg *messaging_pb.Message) error
}

type O5MessageDeadLetterHandler struct {
	source    sidecar.AppInfo
	publisher Publisher
}

func NewO5MessageDeadLetterHandler(publisher Publisher, source sidecar.AppInfo) *O5MessageDeadLetterHandler {
	return &O5MessageDeadLetterHandler{
		source:    source,
		publisher: publisher,
	}
}

func (dlh *O5MessageDeadLetterHandler) DeadMessage(ctx context.Context, death *messaging_tpb.DeadMessage) error {

	death.HandlerApp = dlh.source.SourceApp
	death.HandlerEnv = dlh.source.SourceEnv

	wrapper, err := o5msg.WrapMessage(death)
	if err != nil {
		return err
	}

	wrapper.Headers["o5-sidecar-worker-version"] = dlh.source.SidecarVersion
	wrapper.SourceApp = dlh.source.SourceApp
	wrapper.SourceEnv = dlh.source.SourceEnv
	wrapper.Timestamp = timestamppb.Now()

	if err := dlh.publisher.Publish(ctx, wrapper); err != nil {
		return err
	}

	return nil
}
