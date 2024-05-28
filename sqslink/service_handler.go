package sqslink

import (
	"context"
	"fmt"

	"github.com/pentops/o5-go/messaging/v1/messaging_pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Invoker interface {
	Invoke(context.Context, string, interface{}, interface{}, ...grpc.CallOption) error
}

type service struct {
	requestMessage protoreflect.MessageDescriptor
	invoker        Invoker
	fullName       string
	customParser   func([]byte) (proto.Message, error)
}

func (ss service) HandleMessage(ctx context.Context, body *messaging_pb.Any) error {
	protoBody, err := ss.parseMessageBody(body)
	if err != nil {
		return fmt.Errorf("failed to parse message body: %w", err)
	}

	outputMessage := &emptypb.Empty{}
	// Receive response header
	var responseHeader metadata.MD
	err = ss.invoker.Invoke(ctx, ss.fullName, protoBody, outputMessage, grpc.Header(&responseHeader))
	return err
}

func (ss service) parseMessageBody(body *messaging_pb.Any) (proto.Message, error) {
	if ss.customParser != nil {
		return ss.customParser(body.Value)
	}

	msg := dynamicpb.NewMessage(ss.requestMessage)
	if body.TypeUrl != "" {
		expectedType := fmt.Sprintf("type.googleapis.com/%s", ss.requestMessage.FullName())
		if body.TypeUrl != expectedType {
			return nil, fmt.Errorf("unexpected type url: %s", body.TypeUrl)
		}
	}

	switch body.Encoding {
	case messaging_pb.WireEncoding_UNSPECIFIED:
		if err := proto.Unmarshal(body.Value, msg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal protobuf: %w", err)
		}

	case messaging_pb.WireEncoding_PROTOJSON:
		if err := protojson.Unmarshal(body.Value, msg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal json: %w", err)
		}
	default:
		return nil, fmt.Errorf("unknown wire encoding: %v", body.Encoding)
	}

	return msg, nil
}