package msgconvert

import (
	"fmt"
	"strings"

	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_pb"
	"github.com/pentops/o5-runtime-sidecar/sidecar"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type ReflectionClient interface {
	ProtoToJSON(protoreflect.Message) ([]byte, error)
	FindMessageByName(protoreflect.FullName) (protoreflect.MessageType, error)
}

type Converter struct {
	source     sidecar.AppInfo
	reflection ReflectionClient
}

func NewConverter(source sidecar.AppInfo) *Converter {
	return &Converter{
		source: source,
	}
}

func (ll *Converter) SetReflectionClient(reflection ReflectionClient) {
	ll.reflection = reflection
}

func (ll *Converter) ParseMessage(id string, data []byte) (*messaging_pb.Message, error) {
	msg := &messaging_pb.Message{}
	if err := protojson.Unmarshal(data, msg); err != nil {
		return nil, fmt.Errorf("error unmarshalling outbox message: %w", err)
	}

	msg.MessageId = id

	return ll.ConvertMessage(msg)
}

func (ll *Converter) ConvertMessage(msg *messaging_pb.Message) (*messaging_pb.Message, error) {
	msg.SourceApp = ll.source.SourceApp
	msg.SourceEnv = ll.source.SourceEnv

	if msg.Headers == nil {
		msg.Headers = map[string]string{}
	}
	msg.Headers["o5-sidecar-outbox-version"] = ll.source.SidecarVersion

	// If we can, and the message isn't already in J5 (or raw), convert it to J5
	if ll.reflection != nil {
		switch msg.Body.Encoding {
		case messaging_pb.WireEncoding_UNSPECIFIED,
			messaging_pb.WireEncoding_PROTOJSON:
			// Proto: Convert to J5_JSON
			body, err := ll.toJ5JSON(msg.Body)
			if err != nil {
				return nil, fmt.Errorf("error converting proto to J5_JSON: %w", err)
			}
			msg.Body = body
		}
	}

	switch ext := msg.Extension.(type) {
	case *messaging_pb.Message_Request_:
		// this value is copied to the message body's request.reply_to field by the
		// receiver's sidecar.
		// The receiver app copies the request field from the request to the reply.
		// The generated code for the reply message copies the reply_to field to the
		// message wrapper's Reply.ReplyTo field, allowing the queue subscription rules to filter the reply.
		ext.Request.ReplyTo = fmt.Sprintf("%s/%s", ll.source.SourceEnv, ll.source.SourceApp)
	}

	return msg, nil
}

func (ll *Converter) toJ5JSON(body *messaging_pb.Any) (*messaging_pb.Any, error) {
	typeName := strings.TrimPrefix(body.TypeUrl, "type.googleapis.com/")
	mt, err := ll.reflection.FindMessageByName(protoreflect.FullName(typeName))
	if err != nil {
		return nil, fmt.Errorf("error finding message type: %w", err)
	}

	msg := mt.New()
	switch body.Encoding {
	case messaging_pb.WireEncoding_UNSPECIFIED:
		if err := proto.Unmarshal(body.Value, msg.Interface()); err != nil {
			return nil, fmt.Errorf("error unmarshalling proto: %w", err)
		}

	case messaging_pb.WireEncoding_PROTOJSON:
		if err := protojson.Unmarshal(body.Value, msg.Interface()); err != nil {
			return nil, fmt.Errorf("error unmarshalling protojson: %w", err)
		}

	default:
		return nil, fmt.Errorf("unexpected encoding: %v", body.Encoding)
	}

	jsonBytes, err := ll.reflection.ProtoToJSON(msg)
	if err != nil {
		return nil, fmt.Errorf("error converting proto to JSON: %w", err)
	}

	return &messaging_pb.Any{
		TypeUrl:  body.TypeUrl,
		Encoding: messaging_pb.WireEncoding_J5_JSON,
		Value:    jsonBytes,
	}, nil
}
