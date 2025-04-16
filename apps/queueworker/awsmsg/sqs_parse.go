package awsmsg

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/google/uuid"
	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_pb"
	"github.com/pentops/o5-runtime-sidecar/adapters/eventbridge"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	contentTypeAttribute = "Content-Type"
	serviceAttribute     = "grpc-service"
	grpcMessageAttribute = "grpc-message"
)

type SNSMessageWrapper struct {
	Type      string `json:"Type"`
	Message   string `json:"Message"`
	MessageID string `json:"MessageId"`
	TopicArn  string `json:"TopicArn"`
	Timestamp string `json:"Timestamp"`
}

var messageIDNamespace = uuid.MustParse("B71AFF66-460A-424C-8927-9AF8C9135BF9")

var SQSMessageAttributes = []string{
	serviceAttribute,
	contentTypeAttribute,
	grpcMessageAttribute,
}

func ParseSQSMessage(msg types.Message) (*messaging_pb.Message, error) {
	serviceNameAttributeValue, ok := msg.MessageAttributes[serviceAttribute]
	if ok && serviceNameAttributeValue.StringValue != nil {
		return parseServiceMessage(msg, *serviceNameAttributeValue.StringValue)
	}

	// Try to parse various wrapper methods

	// EventBridge
	wrapper := &eventbridge.EventBridgeWrapper{}
	if err := json.Unmarshal([]byte(*msg.Body), wrapper); err == nil && wrapper.DetailType != "" {
		switch wrapper.DetailType {
		case eventbridge.EventBridgeO5MessageDetailType:
			outboxMessage := &messaging_pb.Message{}
			if err := protojson.Unmarshal(wrapper.Detail, outboxMessage); err != nil {
				return nil, fmt.Errorf("failed to unmarshal o5-message: %w", err)
			}
			return outboxMessage, nil

		default:
			return nil, fmt.Errorf("unknown event-bridge detail type: %s", wrapper.DetailType)
		}
	}

	// SNS Wrapper
	snsWrapper := &SNSMessageWrapper{}
	if err := json.Unmarshal([]byte(*msg.Body), snsWrapper); err == nil {
		if snsWrapper.Type == "Notification" && strings.HasPrefix(snsWrapper.TopicArn, "arn:aws:sns:") {
			topicParts := strings.Split(snsWrapper.TopicArn, ":")
			if len(topicParts) != 6 {
				return nil, fmt.Errorf("invalid SNS topic ARN: %s", snsWrapper.TopicArn)
			}

			outboxMessage := &messaging_pb.Message{
				MessageId: *msg.MessageId,
				Body: &messaging_pb.Any{
					Encoding: messaging_pb.WireEncoding_RAW,
					Value:    []byte(snsWrapper.Message),
				},
				GrpcService:      "o5.messaging.v1.topic.RawMessageTopic",
				GrpcMethod:       "Raw",
				DestinationTopic: topicParts[5],
			}
			if timestamp, err := time.Parse(time.RFC3339, snsWrapper.Timestamp); err == nil {
				outboxMessage.Timestamp = timestamppb.New(timestamp)
			} else {
				outboxMessage.Timestamp = timestamppb.Now()
			}

			return outboxMessage, nil
		}
	}

	o5Wrapper := &messaging_pb.Message{}
	if err := protojson.Unmarshal([]byte(*msg.Body), o5Wrapper); err == nil {
		return o5Wrapper, nil
	}

	return nil, fmt.Errorf("unsupported message format")
}

func looksLikeJSON(body []byte) bool {
	body = bytes.TrimLeft(body, " \t\n")
	body = bytes.TrimRight(body, " \t\n")
	if len(body) == 0 {
		return false
	}
	firstChar := body[0]
	lastChar := body[len(body)-1]

	if firstChar == '{' && lastChar == '}' {
		return true
	}
	if firstChar == '[' && lastChar == ']' {
		return true
	}
	return false
}

func parseServiceMessage(msg types.Message, serviceName string) (*messaging_pb.Message, error) {

	parts := strings.Split(serviceName, "/")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid service name: %s", serviceName)
	}
	grpcService := parts[1]
	grpcMethod := parts[2]

	body := []byte(*msg.Body)

	// Parse Message
	var encoding messaging_pb.WireEncoding
	var encodingSpecified bool
	contentTypeAttributeValue, ok := msg.MessageAttributes[contentTypeAttribute]
	if ok && contentTypeAttributeValue.StringValue != nil {
		encodingSpecified = true
		switch *contentTypeAttributeValue.StringValue {
		case "application/json":
			encoding = messaging_pb.WireEncoding_PROTOJSON
		case "application/protobuf", "":
			encoding = messaging_pb.WireEncoding_UNSPECIFIED
		default:
			return nil, fmt.Errorf("unsupported content type: %s", *contentTypeAttributeValue.StringValue)
		}
	} else {
		if looksLikeJSON(body) {
			encoding = messaging_pb.WireEncoding_PROTOJSON
		} else {
			// fairly big assumption...
			encoding = messaging_pb.WireEncoding_UNSPECIFIED

			msgBytes, err := base64.StdEncoding.DecodeString(string(body))
			if err != nil {
				return nil, fmt.Errorf("failed to decode base64: %w", err)
			}
			body = msgBytes
		}
	}

	if looksLikeJSON(body) {
		// ignoring the content-type header, as the SNS wrapper wraps the
		// message in a JSON object and still copies the headers exactly as-is
		snsWrapper := &SNSMessageWrapper{}
		if err := json.Unmarshal(body, snsWrapper); err == nil {
			if snsWrapper.Type == "Notification" && strings.HasPrefix(snsWrapper.TopicArn, "arn:aws:sns:") {
				body = []byte(snsWrapper.Message)
				if !encodingSpecified {
					if looksLikeJSON(body) {
						encoding = messaging_pb.WireEncoding_PROTOJSON
					} else {
						encoding = messaging_pb.WireEncoding_UNSPECIFIED
					}
				}
			}
		}

	}

	o5MessageID := uuid.NewSHA1(messageIDNamespace, []byte(*msg.MessageId)).String()

	var typeURL string

	if attr, ok := msg.MessageAttributes[grpcMessageAttribute]; ok && attr.StringValue != nil {
		typeURL = fmt.Sprintf("type.googleapis.com/%s", *msg.MessageAttributes[grpcMessageAttribute].StringValue)
	}

	return &messaging_pb.Message{
		MessageId: o5MessageID,
		Body: &messaging_pb.Any{
			Encoding: encoding,
			Value:    body,
			TypeUrl:  typeURL,
		},
		GrpcService: grpcService,
		GrpcMethod:  grpcMethod,
	}, nil
}
