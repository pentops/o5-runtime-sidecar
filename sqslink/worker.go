package sqslink

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/pentops/log.go/log"
	"github.com/pentops/o5-go/messaging/v1/messaging_tpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

const RawMessageName = "/o5.messaging.v1.topic.RawMessageTopic/Raw"

type SQSAPI interface {
	ReceiveMessage(ctx context.Context, input *sqs.ReceiveMessageInput, opts ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	DeleteMessage(ctx context.Context, input *sqs.DeleteMessageInput, opts ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error)
}

type Worker struct {
	SQSClient SQSAPI
	QueueURL  string

	services map[string]*service
}

func NewWorker(sqs SQSAPI, queueURL string) *Worker {
	return &Worker{
		SQSClient: sqs,
		QueueURL:  queueURL,
		services:  make(map[string]*service),
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

func (ss service) parseMessage(contentType string, raw []byte) (proto.Message, error) {

	if ss.customParser != nil {
		return ss.customParser(raw)
	}

	msg := dynamicpb.NewMessage(ss.requestMessage)

	if contentType == "" {
		if raw[0] == '{' {
			contentType = "application/json"
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

type Endpoint interface {
	RoundTrip(ctx context.Context, serviceName string, protoMessage []byte) error
}

func (ww *Worker) RegisterService(service protoreflect.ServiceDescriptor, invoker Invoker) error {
	methods := service.Methods()
	for ii := 0; ii < methods.Len(); ii++ {
		method := methods.Get(ii)
		if err := ww.registerMethod(method, invoker); err != nil {
			return err
		}
	}
	return nil
}

func (ww *Worker) registerMethod(method protoreflect.MethodDescriptor, invoker Invoker) error {
	serviceName := method.Parent().(protoreflect.ServiceDescriptor).FullName()
	ss := &service{
		requestMessage: method.Input(),
		fullName:       fmt.Sprintf("/%s/%s", serviceName, method.Name()),
		invoker:        invoker,
	}

	log.WithField(context.Background(), "service", ss.fullName).Debug("Registering service")

	if ss.fullName == RawMessageName {
		ss.customParser = func(b []byte) (proto.Message, error) {
			return &messaging_tpb.RawMessage{
				Payload: b,
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

func (ww *Worker) handleMessage(ctx context.Context, msg types.Message) {
	//messageID := *msg.MessageId
	//receiptHandle := *msg.ReceiptHandle

	handler, inputMessage, err := ww.parseMessage(msg)
	if err != nil {
		// Leave it here, we need to retry
		log.WithError(ctx, err).Error("failed to handle message")
		return
	}

	ctx = log.WithFields(ctx, map[string]interface{}{
		"grpc-service": handler.fullName,
		"message-id":   *msg.MessageId,
	})
	log.Debug(ctx, "begin handle message")

	outputMessage := &emptypb.Empty{}

	// Handle, with catch

	// Receive response header
	var responseHeader metadata.MD
	err = handler.invoker.Invoke(ctx, handler.fullName, inputMessage, outputMessage, grpc.Header(&responseHeader))
	if err != nil {
		// TODO: something
		log.WithError(ctx, err).Error("failed to handle, leaving")
		return
	}

	log.WithFields(ctx, map[string]interface{}{}).Info("handled message")

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

func (ww *Worker) parseMessage(msg types.Message) (*service, proto.Message, error) {

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

	handler, ok := ww.services[serviceName]
	if !ok {
		return nil, nil, fmt.Errorf("no handler for service %s", serviceName)
	}

	var err error

	parsed, err := handler.parseMessage(contentType, []byte(*msg.Body))
	if err != nil {
		return nil, nil, fmt.Errorf("parsing message for service %s: %w", serviceName, err)
	}

	return handler, parsed, nil
}
