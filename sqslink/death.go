package sqslink

import (
	"context"

	"github.com/pentops/o5-go/messaging/v1/messaging_pb"
	"github.com/pentops/o5-go/messaging/v1/messaging_tpb"
	"github.com/pentops/o5-runtime-sidecar/awsmsg"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type DeadLetterHandler interface {
	DeadMessage(context.Context, *messaging_tpb.DeadMessage) error
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

func (dlh *O5MessageDeadLetterHandler) DeadMessage(ctx context.Context, death *messaging_tpb.DeadMessage) error {

	death.HandlerApp = dlh.source.SourceApp
	death.HandlerEnv = dlh.source.SourceEnv

	protoBody, err := protojson.Marshal(death)
	if err != nil {
		return err
	}

	wireMsg := &messaging_pb.Message{
		MessageId:   death.DeathId,
		GrpcService: "o5.deployer.v1.topic.DeadLetterTopic",
		GrpcMethod:  "DeadLetter",
		Body: &messaging_pb.Any{
			TypeUrl: "type.googleapis.com/o5.deployer.v1.topic.DeadMessage",
			Value:   protoBody,
		},
		SourceApp: dlh.source.SourceApp,
		SourceEnv: dlh.source.SourceEnv,
		Timestamp: timestamppb.Now(),
		Headers: map[string]string{
			"o5-sidecar-worker-version": dlh.source.SidecarVersion,
		},
	}

	if err := dlh.publisher.Publish(ctx, wireMsg); err != nil {
		return err
	}

	return nil
}
