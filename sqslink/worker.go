package sqslink

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/google/uuid"
	"github.com/pentops/log.go/log"
	"github.com/pentops/o5-go/dante/v1/dante_pb"
	"github.com/pentops/o5-go/dante/v1/dante_tpb"
	"github.com/pentops/o5-go/messaging/v1/messaging_tpb"
	"github.com/pentops/o5-runtime-sidecar/outbox"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const RawMessageName = "/o5.messaging.v1.topic.RawMessageTopic/Raw"

type SQSAPI interface {
	ReceiveMessage(ctx context.Context, input *sqs.ReceiveMessageInput, opts ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	DeleteMessage(ctx context.Context, input *sqs.DeleteMessageInput, opts ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error)
}

type DeadLetterHandler interface {
	SendOne(context.Context, outbox.RoutableMessage) error
}

type Worker struct {
	SQSClient         SQSAPI
	QueueURL          string
	deadLetterHandler DeadLetterHandler

	services map[string]*service
}

func NewWorker(sqs SQSAPI, queueURL string, deadLetters DeadLetterHandler) *Worker {
	return &Worker{
		SQSClient:         sqs,
		QueueURL:          queueURL,
		services:          make(map[string]*service),
		deadLetterHandler: deadLetters,
	}
}

type Invoker interface {
	Invoke(context.Context, string, interface{}, interface{}, ...grpc.CallOption) error
}

type service struct {
	requestMessage protoreflect.MessageDescriptor
	invoker        Invoker
	fullName       string
	customParser   func([]byte) (proto.Message, error)
}

func (ss service) HandleMessage(ctx context.Context, message *Message) error {
	outputMessage := &emptypb.Empty{}
	// Receive response header
	var responseHeader metadata.MD
	err := ss.invoker.Invoke(ctx, ss.fullName, message.Proto, outputMessage, grpc.Header(&responseHeader))
	return err
}

type Endpoint interface {
	RoundTrip(ctx context.Context, serviceName string, protoMessage []byte) error
}

func (ww *Worker) RegisterService(ctx context.Context, service protoreflect.ServiceDescriptor, invoker Invoker) error {
	methods := service.Methods()
	for ii := 0; ii < methods.Len(); ii++ {
		method := methods.Get(ii)
		if err := ww.registerMethod(ctx, method, invoker); err != nil {
			return err
		}
	}
	return nil
}

func (ww *Worker) registerMethod(ctx context.Context, method protoreflect.MethodDescriptor, invoker Invoker) error {
	serviceName := method.Parent().(protoreflect.ServiceDescriptor).FullName()
	ss := &service{
		requestMessage: method.Input(),
		fullName:       fmt.Sprintf("/%s/%s", serviceName, method.Name()),
		invoker:        invoker,
	}

	log.WithField(ctx, "service", ss.fullName).Info("Registering Worker Service")

	if ss.fullName == RawMessageName {
		ss.customParser = func(b []byte) (proto.Message, error) {
			snsMessage := &SNSMessageWrapper{}
			err := json.Unmarshal(b, snsMessage)
			if err != nil {
				return &messaging_tpb.RawMessage{
					Payload: b,
				}, nil
			}

			return &messaging_tpb.RawMessage{
				Topic:   snsMessage.TopicArn,
				Payload: snsMessage.Message,
			}, nil
		}
	}

	ww.services[ss.fullName] = ss

	return nil
}

func (ww *Worker) Run(ctx context.Context) error {

	for {
		out, err := ww.SQSClient.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl: &ww.QueueURL,

			// Max = 10
			MaxNumberOfMessages: 10,

			// The duration (in seconds) for which the call waits for a message to arrive in
			// the queue before returning.
			WaitTimeSeconds: 5,

			// The duration (in seconds) that the received messages are hidden from subsequent
			// retrieve requests after being retrieved by a ReceiveMessage request.
			VisibilityTimeout: 30,

			MessageAttributeNames: []string{
				contentTypeAttribute,
				serviceAttribute,
			},

			AttributeNames: []types.QueueAttributeName{
				// this type conversion is probably a bug in the SDK
				types.QueueAttributeName(types.MessageSystemAttributeNameApproximateReceiveCount),
			},
		})
		if err != nil {
			return err
		}

		for _, msg := range out.Messages {
			ww.handleMessage(ctx, msg)
		}
	}
}

const (
	// this the the most magic of magic strings, built by the protoc-gen-go
	// extension for messaging
	serviceAttribute     = "grpc-service"
	contentTypeAttribute = "Content-Type"
)

func getReceiveCount(msg types.Message) int {
	receiveCountAttribute, ok := msg.Attributes[string(types.MessageSystemAttributeNameApproximateReceiveCount)]
	if !ok {
		return 0
	}
	asInt, err := strconv.Atoi(receiveCountAttribute)
	if err != nil {
		return 0
	}
	return asInt

}

func (ww *Worker) handleMessage(ctx context.Context, msg types.Message) {
	//messageID := *msg.MessageId
	//receiptHandle := *msg.ReceiptHandle

	parsed, handler, err := ww.parseMessage(msg)
	if err != nil {
		// Leave it here, we need to retry
		log.WithError(ctx, err).Error("failed to handle message")
		return
	}

	ctx = log.WithFields(ctx, map[string]interface{}{
		"grpc-service":   parsed.ServiceName,
		"sqs-message-id": parsed.SQSMessageID,
	})
	log.Debug(ctx, "begin handle message")

	err = handler.HandleMessage(ctx, parsed)
	if err != nil {
		ctx = log.WithError(ctx, err)
		log.Error(ctx, "Error handling message")
		if ww.deadLetterHandler == nil && getReceiveCount(msg) <= 3 {
			log.Error(ctx, "Error handling message, leaving in queue")
			return
		}
		err := ww.killMessage(ctx, parsed, err)
		if err != nil {
			log.WithField(ctx, "killError", err.Error()).Error("Error killing message, leaving in queue")
			return
		}
		log.Info(ctx, "Killed")
	} else {
		log.Info(ctx, "Success")
	}

	// Delete
	_, err = ww.SQSClient.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      &ww.QueueURL,
		ReceiptHandle: msg.ReceiptHandle,
	})
	if err != nil {
		log.WithError(ctx, err).Error("failed to delete message")
		return
	}
}

type Message struct {
	Proto        proto.Message
	ServiceName  string
	SQSMessageID string
	MessageID    string
}

type Handler interface {
	HandleMessage(context.Context, *Message) error
}

type SNSMessageWrapper struct {
	Type      string `json:"Type"`
	Message   []byte `json:"Message"`
	MessageID string `json:"MessageId"`
	TopicArn  string `json:"TopicArn"`
}

func (ss *service) parseMessageBody(contentType string, raw []byte) (proto.Message, error) {
	if ss.customParser != nil {
		return ss.customParser(raw)
	}

	msg := dynamicpb.NewMessage(ss.requestMessage)

	if contentType == "" {
		if raw[0] == '{' {
			contentType = "application/json"
			snsMessage := &SNSMessageWrapper{}
			err := json.Unmarshal(raw, snsMessage)
			if err == nil { // IF NO ERROR
				raw = snsMessage.Message
			}

		} else {
			contentType = "application/protobuf"
		}
	}

	switch contentType {
	case "application/json":
		if err := protojson.Unmarshal(raw, msg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal json: %w", err)
		}
	case "application/protobuf":
		msgBytes, err := base64.StdEncoding.DecodeString(string(raw))
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64: %w", err)
		}

		if err := proto.Unmarshal(msgBytes, msg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal protobuf: %w", err)
		}
	default:
		return nil, fmt.Errorf("unknown content type: %s", contentType)
	}

	return msg, nil
}

func (ww *Worker) parseMessage(msg types.Message) (*Message, Handler, error) { //*service, proto.Message, error) {

	// Parse Message
	var serviceName string
	serviceNameAttributeValue, ok := msg.MessageAttributes[serviceAttribute]
	if ok {
		serviceName = *serviceNameAttributeValue.StringValue
	} else {
		serviceName = RawMessageName
	}

	var contentType string
	contentTypeAttributeValue, ok := msg.MessageAttributes[contentTypeAttribute]
	if ok {
		contentType = *contentTypeAttributeValue.StringValue
	}

	service, ok := ww.services[serviceName]
	if !ok {
		return nil, nil, fmt.Errorf("no handler for service %s", serviceName)
	}

	var err error

	parsed, err := service.parseMessageBody(contentType, []byte(*msg.Body))
	if err != nil {
		return nil, nil, fmt.Errorf("parsing message for service %s: %w", serviceName, err)
	}

	// TODO: Pass this through the message infra where possible
	messageID := uuid.NewSHA1(messageIDNamespace, []byte(*msg.MessageId)).String()

	return &Message{
		Proto:        parsed,
		ServiceName:  serviceName,
		SQSMessageID: *msg.MessageId,
		MessageID:    messageID,
	}, service, nil
}

var messageIDNamespace = uuid.MustParse("B71AFF66-460A-424C-8927-9AF8C9135BF9")

func (ww *Worker) killMessage(ctx context.Context, msg *Message, killError error) error {

	inputAny, err := anypb.New(msg.Proto)
	if err != nil {
		return fmt.Errorf("failed to marshal input message as ANY: %w", err)
	}
	inputJSON, err := protojson.Marshal(msg.Proto)
	if err != nil {
		return fmt.Errorf("failed to marshal input message as JSON: %w", err)
	}

	deadMessage := &dante_tpb.DeadMessage{
		InfraMessageId: msg.SQSMessageID,
		MessageId:      msg.MessageID,
		QueueName:      ww.QueueURL,
		GrpcName:       msg.ServiceName,
		Timestamp:      timestamppb.Now(),
		Problem: &dante_pb.Problem{
			Type: &dante_pb.Problem_UnhandledError{
				UnhandledError: &dante_pb.UnhandledError{
					Error: killError.Error(),
				},
			},
		},
		Payload: &dante_pb.Any{
			Proto: inputAny,
			Json:  string(inputJSON),
		},
	}
	if err := ww.deadLetterHandler.SendOne(ctx, deadMessage); err != nil {
		return err
	}
	return nil
}
