package sqslink

import (
	"context"

	"github.com/google/uuid"
	"github.com/pentops/dante/gen/o5/dante/v1/dante_tpb"
	"github.com/pentops/o5-go/messaging/v1/messaging_pb"
	"github.com/pentops/o5-runtime-sidecar/awsmsg"
	"google.golang.org/protobuf/proto"
)

type DeadLetterHandler interface {
	DeadMessage(context.Context, *dante_tpb.DeadMessage) error
}

type O5MessageDeadLetterHandler struct {
	source    awsmsg.SourceConfig
	publisher awsmsg.Publisher
}

func NewO5MessageDeadLetterHandler(publisher awsmsg.Publisher, source awsmsg.SourceConfig) *O5MessageDeadLetterHandler {
	return &O5MessageDeadLetterHandler{
		source:    source,
		publisher: publisher,
	}
}

func (dlh *O5MessageDeadLetterHandler) DeadMessage(ctx context.Context, deadMessage *dante_tpb.DeadMessage) error {

	protoBody, err := proto.Marshal(deadMessage)
	if err != nil {
		return err
	}

	wireMsg := &messaging_pb.Message{
		MessageId:   uuid.New().String(), // Using new here, as this is a unique instance of a death.
		GrpcService: "o5.deployer.v1.topic.DeadLetterTopic",
		GrpcMethod:  "DeadLetter",
		Body: &messaging_pb.Any{
			TypeUrl: "type.googleapis.com/o5.deployer.v1.topic.DeadMessage",
			Value:   protoBody,
		},
		DestinationTopic: deadMessage.MessagingTopic(),
		SourceApp:        dlh.source.SourceApp,
		SourceEnv:        dlh.source.SourceEnv,
	}

	if err := dlh.publisher.Publish(ctx, wireMsg); err != nil {
		return err
	}

	return nil
}
