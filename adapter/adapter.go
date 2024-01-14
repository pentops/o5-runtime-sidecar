package adapter

import (
	"context"

	"github.com/pentops/o5-go/messaging/v1/messaging_tpb"
	"github.com/pentops/o5-runtime-sidecar/outbox"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Sender interface {
	SendMarshalled(ctx context.Context, msg outbox.MarshalledMessage) error
}

type MessageBridge struct {
	sender Sender
	messaging_tpb.UnimplementedMessageBridgeTopicServer
}

func NewMessageBridge(sender Sender) *MessageBridge {
	return &MessageBridge{
		sender: sender,
	}
}

func (mb *MessageBridge) Send(ctx context.Context, req *messaging_tpb.SendMessage) (*emptypb.Empty, error) {

	headers := map[string]string{}
	for _, header := range req.Headers {
		headers[header.Key] = header.Value
	}

	msg := outbox.MarshalledMessage{
		Body:    req.Payload,
		Headers: headers,
		Topic:   req.Destination,
	}

	if err := mb.sender.SendMarshalled(ctx, msg); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
