package sqslink

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/google/uuid"
	"github.com/pentops/log.go/log"
	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_pb"
	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_tpb"
	"github.com/pentops/o5-runtime-sidecar/apps/queueworker/awsmsg"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const RawMessageName = "/o5.messaging.v1.topic.RawMessageTopic/Raw"
const GenericTopic = "/o5.messaging.v1.topic.GenericMessageTopic/Generic"

type SQSAPI interface {
	ReceiveMessage(ctx context.Context, input *sqs.ReceiveMessageInput, opts ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	DeleteMessage(ctx context.Context, input *sqs.DeleteMessageInput, opts ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error)
}

type Handler interface {
	HandleMessage(context.Context, *messaging_pb.Message) error
}

type HandlerFunc func(context.Context, *messaging_pb.Message) error

func (hf HandlerFunc) HandleMessage(ctx context.Context, msg *messaging_pb.Message) error {
	return hf(ctx, msg)
}

type Worker struct {
	SQSClient         SQSAPI
	QueueURL          string
	deadLetterHandler DeadLetterHandler
	resendChance      int

	handlers        map[string]Handler
	fallbackHandler Handler
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
		handlers:          make(map[string]Handler),
		deadLetterHandler: deadLetters,
		resendChance:      resendChance,
	}
}

func (ww *Worker) RegisterService(ctx context.Context, service protoreflect.ServiceDescriptor, invoker AppLink) error {
	methods := service.Methods()
	for ii := 0; ii < methods.Len(); ii++ {
		method := methods.Get(ii)
		if err := ww.registerMethod(ctx, method, invoker); err != nil {
			return err
		}
	}
	return nil
}

func (ww *Worker) registerMethod(ctx context.Context, method protoreflect.MethodDescriptor, invoker AppLink) error {
	serviceName := method.Parent().(protoreflect.ServiceDescriptor).FullName()
	fullName := fmt.Sprintf("/%s/%s", serviceName, method.Name())

	if fullName == GenericTopic {
		log.WithField(ctx, "service", fullName).Info("Registering Generic Fallback")
		ww.fallbackHandler = &genericHandler{
			invoker: invoker,
		}

	} else {
		log.WithField(ctx, "service", fullName).Info("Registering Worker Service")
		ss := &service{
			requestMessage: method.Input(),
			fullName:       fullName,
			invoker:        invoker,
		}
		ww.handlers[ss.fullName] = ss
	}
	return nil
}

func (ww *Worker) RegisterHandler(fullMethod string, handler Handler) {
	ww.handlers[fullMethod] = handler
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

		MessageAttributeNames: awsmsg.SQSMessageAttributes,

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
		if randomlySelected(ctx, ww.resendChance) {
			ww.handleMessage(ctx, msg)
		}
	}
	return nil
}

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
	parsed, err := awsmsg.ParseSQSMessage(msg)
	if err != nil {
		// Leave it for retry unless we keep failing at parsing it
		log.WithError(ctx, err).Error("Message Worker: Failed to parse message")

		if ww.deadLetterHandler == nil && getReceiveCount(msg) <= 3 {
			log.WithError(ctx, err).Error("Message Worker: failed to parse message, leaving in queue")
			return
		}
		err := ww.killMessage(ctx, msg, parsed, err)
		if err != nil {
			log.WithField(ctx, "killError", err.Error()).Error("Message Worker: Error killing unparsable message, leaving in queue")
			return
		}
		log.Info(ctx, "Message Handler: Killed due to parsing issues")

		return
	}

	ctx = log.WithFields(ctx, map[string]any{
		"grpc-service":   parsed.GrpcService,
		"grpc-method":    parsed.GrpcMethod,
		"message-id":     parsed.MessageId,
		"topic":          parsed.DestinationTopic,
		"sqs-message-id": msg.MessageId,
	})
	log.Debug(ctx, "Message Handler: Begin")

	fullServiceName := fmt.Sprintf("/%s/%s", parsed.GrpcService, parsed.GrpcMethod)
	handler, ok := ww.handlers[fullServiceName]
	if !ok {
		if ww.fallbackHandler != nil {
			log.Debug(ctx, "Message Handler: Using fallback handler")
			handler = ww.fallbackHandler
		} else {
			log.Error(ctx, "no handler matched")
			if ww.deadLetterHandler == nil && getReceiveCount(msg) <= 3 {
				log.Error(ctx, "Error handling message, leaving in queue")
				return
			}
			err := ww.killMessage(ctx, msg, parsed, fmt.Errorf("no handler for %s", fullServiceName))
			if err != nil {
				log.WithField(ctx, "killError", err.Error()).
					Error("Message Worker: Error killing message, leaving in queue")
				return
			}
			log.Debug(ctx, "Message Handler: Killed")
			return
		}
	}

	err = handler.HandleMessage(ctx, parsed)

	if err != nil {
		ctx = log.WithError(ctx, err)
		log.Error(ctx, "Message Handler: Error")
		if ww.deadLetterHandler == nil && getReceiveCount(msg) <= 3 {
			log.Error(ctx, "Error handling message, leaving in queue")
			return
		}
		err := ww.killMessage(ctx, msg, parsed, err)
		if err != nil {
			log.WithField(ctx, "killError", err.Error()).
				Error("Message Worker: Error killing message, leaving in queue")
			return
		}
		log.Debug(ctx, "Message Handler: Killed")
		return
	} else {
		log.Info(ctx, "Message Handler: Success")
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

func (ww *Worker) killMessage(ctx context.Context, sqsMsg types.Message, msg *messaging_pb.Message, killError error) error {
	if ww.deadLetterHandler == nil {
		return fmt.Errorf("no dead letter handler")
	}

	if msg == nil {
		// Unparsable message
		msg = &messaging_pb.Message{
			MessageId: *sqsMsg.MessageId,
			Body: &messaging_pb.Any{
				TypeUrl:  RawMessageName,
				Encoding: messaging_pb.WireEncoding_RAW,
				Value:    []byte(*sqsMsg.Body),
			},
		}
	}

	meta := map[string]string{
		"messageId": *sqsMsg.MessageId,
		"queueUrl":  ww.QueueURL,
	}

	for k, v := range sqsMsg.MessageAttributes {
		meta["attr:"+k] = *v.StringValue
	}

	problem := &messaging_tpb.Problem{
		Type: &messaging_tpb.Problem_UnhandledError_{
			UnhandledError: &messaging_tpb.Problem_UnhandledError{
				Error: killError.Error(),
			},
		},
	}

	death := &messaging_tpb.DeadMessage{
		DeathId: uuid.New().String(),
		Problem: problem,
		Message: msg,
		Infra: &messaging_tpb.Infra{
			Type:     "SQS",
			Metadata: meta,
		},
	}

	err := ww.deadLetterHandler.DeadMessage(ctx, death)
	if err != nil {
		return err
	}

	// Delete
	_, err = ww.SQSClient.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      &ww.QueueURL,
		ReceiptHandle: sqsMsg.ReceiptHandle,
	})
	if err != nil {
		return err
	}
	return nil
}
