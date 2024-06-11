package adapter

import (
	"context"
	"fmt"

	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_tpb"
	"github.com/pentops/o5-runtime-sidecar/awsmsg"
	"google.golang.org/protobuf/types/known/emptypb"
)

type MessageBridge struct {
	publisher awsmsg.Publisher
	messaging_tpb.UnimplementedMessageBridgeTopicServer
	source awsmsg.SourceConfig
}

func NewMessageBridge(publisher awsmsg.Publisher, source awsmsg.SourceConfig) *MessageBridge {
	return &MessageBridge{
		publisher: publisher,
		source:    source,
	}
}

func (mb *MessageBridge) Send(ctx context.Context, req *messaging_tpb.SendMessage) (*emptypb.Empty, error) {
	msg := req.Message

	msg.SourceApp = mb.source.SourceApp
	msg.SourceEnv = mb.source.SourceEnv

	if err := mb.publisher.Publish(ctx, msg); err != nil {
		return nil, fmt.Errorf("couldn't send marshalled msg: %w", err)
	}
	return &emptypb.Empty{}, nil
}
