package sqslink

import (
	"context"
	"fmt"

	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_pb"
	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_tpb"
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
}

func (ss service) HandleMessage(ctx context.Context, message *messaging_pb.Message) error {

	protoBody, err := ss.parseMessageBody(message)
	if err != nil {
		return fmt.Errorf("failed to parse message body: %w", err)
	}

	requestMetadata := metadata.MD{
		"x-o5-message-id": []string{message.MessageId},
	}
	ctx = metadata.NewOutgoingContext(ctx, requestMetadata)

	outputMessage := &emptypb.Empty{}

	// Receive response header
	var responseHeader metadata.MD
	err = ss.invoker.Invoke(ctx, ss.fullName, protoBody, outputMessage, grpc.Header(&responseHeader))
	return err
}

func (ss service) parseMessageBody(message *messaging_pb.Message) (proto.Message, error) {
	if message.Body.Encoding == messaging_pb.WireEncoding_RAW {
		return &messaging_tpb.RawMessage{
			Topic:   message.DestinationTopic,
			Payload: message.Body.Value,
		}, nil
	}

	body := message.Body

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

	switch ext := message.Extension.(type) {
	case *messaging_pb.Message_Request_:
		// The handler is the server which receives the request and replies to it.
		// The message metadata contains the platform level reply-to information
		// which the sidecar sending the request has set.
		// Both the Request and Reply message should have a 'request' field which is
		//j5.messaging.v1.RequestMetadata
		replyTo := ext.Request.ReplyTo
		if replyTo != "" {
			if err := setReplyTo(msg, replyTo); err != nil {
				return nil, fmt.Errorf("failed to set reply-to: %w", err)
			}
		}
	}

	return msg, nil
}

func setReplyTo(msg proto.Message, dest string) error {

	refl := msg.ProtoReflect()
	desc := refl.Descriptor()
	fields := desc.Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		if field.Kind() != protoreflect.MessageKind {
			continue
		}

		metadataMessageField := field.Message()
		if metadataMessageField.FullName() != "j5.messaging.v1.RequestMetadata" {
			continue
		}

		metadataMessage := refl.Mutable(field).Message()
		if !metadataMessage.IsValid() {
			return fmt.Errorf("reply metadata message is not valid (not set)")
		}

		replyField := metadataMessage.Descriptor().Fields().ByName("reply_to")
		if field == nil {
			return fmt.Errorf("reply_to field not found in request metadata")
		}

		metadataMessage.Set(replyField, protoreflect.ValueOfString(dest))

		return nil
	}

	return fmt.Errorf("request field not found")
}
