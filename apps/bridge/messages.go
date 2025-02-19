package bridge

import (
	"context"
	"fmt"

	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_pb"
	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_tpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Publisher interface {
	Publish(ctx context.Context, msg *messaging_pb.Message) error
}

type Converter interface {
	ConvertMessage(*messaging_pb.Message) (*messaging_pb.Message, error)
}

type MessageBridge struct {
	publisher Publisher
	messaging_tpb.UnimplementedMessageBridgeTopicServer
	converter Converter
}

func NewMessageBridge(publisher Publisher, converter Converter) *MessageBridge {
	return &MessageBridge{
		publisher: publisher,
		converter: converter,
	}
}

func (mb *MessageBridge) Send(ctx context.Context, req *messaging_tpb.SendMessage) (*emptypb.Empty, error) {
	msg, err := mb.converter.ConvertMessage(req.Message)
	if err != nil {
		return nil, fmt.Errorf("couldn't convert message: %w", err)
	}

	if err := mb.publisher.Publish(ctx, msg); err != nil {
		return nil, fmt.Errorf("couldn't send marshalled msg: %w", err)
	}

	return &emptypb.Empty{}, nil
}
