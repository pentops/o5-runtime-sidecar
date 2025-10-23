package sqsmsg

import (
	"context"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/google/uuid"
	"github.com/pentops/log.go/log"
	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_pb"
	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_tpb"
	"github.com/pentops/o5-runtime-sidecar/apps/queueworker/messaging"
)

const RawMessageName = "/o5.messaging.v1.topic.RawMessageTopic/Raw"

type SQSAPI interface {
	ReceiveMessage(ctx context.Context, input *sqs.ReceiveMessageInput, opts ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	DeleteMessage(ctx context.Context, input *sqs.DeleteMessageInput, opts ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error)
}

type Worker struct {
	router            messaging.Handler
	SQSClient         SQSAPI
	QueueURL          string
	deadLetterHandler messaging.DeadLetterHandler
}

func NewWorker(sqs SQSAPI, queueURL string, deadLetters messaging.DeadLetterHandler, handler messaging.Handler) *Worker {
	return &Worker{
		SQSClient:         sqs,
		QueueURL:          queueURL,
		router:            handler,
		deadLetterHandler: deadLetters,
	}
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

		MessageAttributeNames: SQSMessageAttributes,

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
	parsed, err := ParseSQSMessage(msg)
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

	ctx = log.WithField(ctx, "sqs-message-id", msg.MessageId)

	err = ww.router.HandleMessage(ctx, parsed)
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
