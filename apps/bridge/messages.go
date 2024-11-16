package bridge

import (
	"context"
	"fmt"

	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_pb"
	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_tpb"
	"github.com/pentops/o5-runtime-sidecar/sidecar"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Publisher interface {
	Publish(ctx context.Context, msg *messaging_pb.Message) error
}

type MessageBridge struct {
	publisher Publisher
	messaging_tpb.UnimplementedMessageBridgeTopicServer
	source sidecar.AppInfo
}

func NewMessageBridge(publisher Publisher, source sidecar.AppInfo) *MessageBridge {
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
