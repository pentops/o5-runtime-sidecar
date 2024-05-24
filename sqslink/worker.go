package sqslink

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/google/uuid"
	"github.com/pentops/dante/gen/o5/dante/v1/dante_pb"
	"github.com/pentops/dante/gen/o5/dante/v1/dante_tpb"
	"github.com/pentops/log.go/log"
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
	resendChance      int

	services map[string]Handler
}

type Handler interface {
	HandleMessage(context.Context, *O5Message) error
}

type HandlerFunc func(context.Context, *O5Message) error

func (hf HandlerFunc) HandleMessage(ctx context.Context, msg *O5Message) error {
	return hf(ctx, msg)
}

// Is this message is randomly selected based on percent received?
func randomlySelected(ctx context.Context, pct int) bool {
	if pct == 0 {
		return false
	}

	if pct == 100 {
		return true
	}

	if pct > 100 || pct < 0 {
		log.Infof(ctx, "Received invalid percent for randomly selecting a message: %v", pct)
		return false
	}

	r, err := rand.Int(rand.Reader, big.NewInt(100))
	if err != nil {
		log.WithError(ctx, err).Error("couldn't generate random number for selecting message")
		return false
	}

	if r.Int64() <= big.NewInt(int64(pct)).Int64() {
		log.Infof(ctx, "Message randomly selected: rand of %v and percent of %v", r.Int64(), pct)
		return true
	}
	return false
}

func NewWorker(sqs SQSAPI, queueURL string, deadLetters DeadLetterHandler, resendChance int) *Worker {
	return &Worker{
		SQSClient:         sqs,
		QueueURL:          queueURL,
		services:          make(map[string]Handler),
		deadLetterHandler: deadLetters,
		resendChance:      resendChance,
	}
}

type Invoker interface {
	Invoke(context.Context, string, interface{}, interface{}, ...grpc.CallOption) error
}

type GRPCMessage struct {
	Proto        proto.Message
	ServiceName  string
	SQSMessageID string
	MessageID    string
}

type service struct {
	requestMessage protoreflect.MessageDescriptor
	invoker        Invoker
	fullName       string
	customParser   func([]byte) (proto.Message, error)
}

func (ss service) HandleMessage(ctx context.Context, msg *O5Message) error {
	protoBody, err := ss.parseMessageBody(msg.ContentType, msg.Message)
	if err != nil {
		return fmt.Errorf("failed to parse message body: %w", err)
	}

	outputMessage := &emptypb.Empty{}
	// Receive response header
	var responseHeader metadata.MD
	err = ss.invoker.Invoke(ctx, ss.fullName, protoBody, outputMessage, grpc.Header(&responseHeader))
	return err
}

func (ss service) parseMessageBody(contentType string, raw []byte) (proto.Message, error) {
	if ss.customParser != nil {
		return ss.customParser(raw)
	}

	msg := dynamicpb.NewMessage(ss.requestMessage)

	switch contentType {
	case "application/json":
		if err := protojson.Unmarshal(raw, msg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal json: %w", err)
		}
	case "application/protobuf":
		if err := proto.Unmarshal(raw, msg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal protobuf: %w", err)
		}
	default:
		return nil, fmt.Errorf("unknown content type: %s", contentType)
	}

	return msg, nil
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
				log.WithError(ctx, err).Error("failed to unmarshal SNS message, falling back to raw")
				return &messaging_tpb.RawMessage{
					Payload: b,
				}, nil
			}

			return &messaging_tpb.RawMessage{
				Topic:   snsMessage.TopicArn,
				Payload: []byte(snsMessage.Message),
			}, nil
		}
	}

	ww.services[ss.fullName] = ss

	return nil
}

func (ww *Worker) RegisterHandler(fullMethod string, handler Handler) {
	ww.services[fullMethod] = handler
}

func (ww *Worker) Run(ctx context.Context) error {
	for {
		if err := ww.FetchOnce(ctx); err != nil {
			return err
		}
	}
}

func (ww *Worker) FetchOnce(ctx context.Context) error {
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

	if len(out.Messages) == 0 {
		log.Info(ctx, "no messages")
	}

	for _, msg := range out.Messages {

		ww.handleMessage(ctx, msg)

		if randomlySelected(ctx, ww.resendChance) {
			ww.handleMessage(ctx, msg)
		}
	}
	return nil
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
	parsed, err := parseMessage(msg)
	if err != nil {
		// Leave it here, we need to retry
		log.WithError(ctx, err).Error("failed to parse message")
		return
	}

	if parsed.ID == "" {
		parsed.ID = uuid.NewSHA1(messageIDNamespace, []byte(parsed.SQSMessageID)).String()
	}

	ctx = log.WithFields(ctx, map[string]interface{}{
		"grpc-service":   parsed.GrpcService,
		"grpc-method":    parsed.GrpcMethod,
		"sqs-message-id": msg.MessageId,
	})

	fullServiceName := fmt.Sprintf("/%s/%s", parsed.GrpcService, parsed.GrpcMethod)
	handler, ok := ww.services[fullServiceName]
	if !ok {
		log.Error(ctx, "no handler matched")
		return
	}

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

type SNSMessageWrapper struct {
	Type      string `json:"Type"`
	Message   string `json:"Message"`
	MessageID string `json:"MessageId"`
	TopicArn  string `json:"TopicArn"`
}

type EventBridgeWrapper struct {
	Detail       json.RawMessage `json:"detail"`
	DetailType   string          `json:"detail-type"`
	EventBusName string          `json:"eventBusName"`
	Source       string          `json:"source"`
	Account      string          `json:"account"`
	Region       string          `json:"region"`
	Resources    []string        `json:"resources"`
	Time         string          `json:"time"`
}

type O5Message struct {
	ID                string            `json:"id"`
	SQSMessageID      string            `json:"sqs-message-id"`
	Message           []byte            `json:"body"`
	Destination       string            `json:"topic-name"`
	Headers           map[string]string `json:"headers"`
	SourceEnvironment string            `json:"source-environment"`
	ContentType       string            `json:"content-type"`

	GrpcService string  `json:"grpc-service"`
	GrpcMethod  string  `json:"grpc-method"`
	GrpcMessage string  `json:"grpc-message"`
	ReplyDest   *string `json:"reply-dest,omitempty"`
}

func parseMessage(msg types.Message) (*O5Message, error) {

	body := []byte(*msg.Body)

	// Parse Message
	var contentType string
	contentTypeAttributeValue, ok := msg.MessageAttributes[contentTypeAttribute]
	if ok {
		contentType = *contentTypeAttributeValue.StringValue
	}
	if contentType == "" {
		if len(body) > 0 && (body[0] == '{' || body[0] == '[') {
			contentType = "application/json"
		} else {
			// fairly big assumption...
			contentType = "application/protobuf"

			msgBytes, err := base64.StdEncoding.DecodeString(string(body))
			if err != nil {
				return nil, fmt.Errorf("failed to decode base64: %w", err)
			}
			body = msgBytes

		}
	}

	serviceNameAttributeValue, ok := msg.MessageAttributes[serviceAttribute]
	if ok && serviceNameAttributeValue.StringValue != nil {
		serviceName := *serviceNameAttributeValue.StringValue
		parts := strings.Split(serviceName, "/")
		if len(parts) != 3 {
			return nil, fmt.Errorf("invalid service name: %s", serviceName)
		}

		if contentType == "application/json" {
			snsWrapper := &SNSMessageWrapper{}
			if err := json.Unmarshal([]byte(*msg.Body), snsWrapper); err == nil && snsWrapper.Message != "" {
				body = []byte(snsWrapper.Message)

			}
		}

		return &O5Message{
			Message:      body,
			SQSMessageID: *msg.MessageId,
			GrpcService:  parts[1],
			GrpcMethod:   parts[2],
			ContentType:  contentType,
		}, nil
	}

	if contentType != "application/json" {
		return nil, fmt.Errorf("unsupported content type: %s", contentType)
	}

	// Try to parse various wrapper methods
	wrapper := &EventBridgeWrapper{}
	if err := json.Unmarshal([]byte(*msg.Body), wrapper); err == nil && wrapper.DetailType != "" {
		switch wrapper.DetailType {
		case "o5-message":
			outboxMessage := &O5Message{}
			if err := json.Unmarshal(wrapper.Detail, outboxMessage); err != nil {
				return nil, fmt.Errorf("failed to unmarshal o5-message: %w", err)
			}
			outboxMessage.SQSMessageID = *msg.MessageId
			if outboxMessage.ContentType == "" {
				outboxMessage.ContentType = "application/protobuf"
			}
			return outboxMessage, nil

		default:
			return nil, fmt.Errorf("unknown event-bridge detail type: %s", wrapper.DetailType)
		}

	}

	return nil, fmt.Errorf("unsupported message format")
}

var messageIDNamespace = uuid.MustParse("B71AFF66-460A-424C-8927-9AF8C9135BF9")

func (ww *Worker) killMessage(ctx context.Context, msg *O5Message, killError error) error {
	if ww.deadLetterHandler == nil {
		return fmt.Errorf("no dead letter handler")
	}

	deadMessage := &dante_tpb.DeadMessage{
		InfraMessageId: msg.SQSMessageID,
		MessageId:      msg.ID,
		QueueName:      ww.QueueURL,
		GrpcName:       fmt.Sprintf("/%s/%s", msg.GrpcService, msg.GrpcMethod),
		Timestamp:      timestamppb.Now(),
		Problem: &dante_pb.Problem{
			Type: &dante_pb.Problem_UnhandledError{
				UnhandledError: &dante_pb.UnhandledError{
					Error: killError.Error(),
				},
			},
		},
		Payload: &dante_pb.Any{
			Proto: &anypb.Any{
				TypeUrl: fmt.Sprintf("type.googleapis.com/%s", msg.GrpcMessage),
				Value:   msg.Message,
			},
		},
	}
	if err := ww.deadLetterHandler.SendOne(ctx, deadMessage); err != nil {
		return err
	}
	return nil
}
