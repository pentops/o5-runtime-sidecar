package amqp

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/pentops/j5/lib/j5codec"
	"github.com/pentops/log.go/log"
	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_pb"
	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_tpb"
	"github.com/pentops/o5-runtime-sidecar/apps/queueworker/messaging"
	amqp "github.com/rabbitmq/amqp091-go"
)

const RawMessageName = "/o5.messaging.v1.topic.RawMessageTopic/Raw"

type Worker struct {
	queueName         string
	connector         *Connector
	handler           messaging.Handler
	deadLetterHandler messaging.DeadLetterHandler
}

func NewWorker(config AMQPConfig, router messaging.Handler, deadLetter messaging.DeadLetterHandler) (*Worker, error) {

	conn := NewConnector(config)

	sub := &Worker{
		connector:         conn,
		queueName:         config.Queue,
		handler:           router,
		deadLetterHandler: deadLetter,
	}
	return sub, nil

}

func (ww *Worker) Run(ctx context.Context) error {

	for {
		err := ww.runLoopOnce(ctx)
		if errors.Is(err, context.Canceled) {
			return nil
		}
		log.WithError(ctx, err).Error("Worker: Error in run loop, restarting")
	}
}

func (ww *Worker) runLoopOnce(ctx context.Context) error {
	ch, err := ww.connector.Channel()
	if err != nil {
		return err
	}

	delivery, err := ch.ConsumeWithContext(ctx,
		ww.queueName, // queue
		"",           // consumer
		false,        // autoAck
		false,        // exclusive
		false,        // noLocal
		false,        // noWait
		nil,          // args
	)
	if err != nil {
		return err
	}

	for msg := range delivery {
		err = ww.handleDelivery(ctx, msg)
		if err != nil {
			return err
		}
	}
	return nil
}

func (ww *Worker) handleDelivery(ctx context.Context, delivery amqp.Delivery) error {
	routingKey := delivery.RoutingKey
	ctx = log.WithFields(ctx, "routing_key", routingKey)
	log.Info(ctx, "Message Handler: Received message")
	count, ok := delivery.Headers["x-delivery-count"].(int64)
	if !ok {
		count = 0
	}

	if delivery.ContentType != "application/o5-message" {
		return fmt.Errorf("invalid content type: %q", delivery.ContentType)
	}

	ctx = log.WithFields(ctx, "deliveryCount", count)

	msg := &messaging_pb.Message{}
	err := j5codec.Global.JSONToProto(delivery.Body, msg.ProtoReflect())
	if err != nil {
		return err
	}

	handlerError := ww.handler.HandleMessage(ctx, msg)
	if handlerError == nil { // LOGIC INVERSION
		err = delivery.Ack(false)
		if err != nil {
			return err
		}
		return nil
	}
	log.WithError(ctx, handlerError).Error("Message Handler: Error")
	if count >= 3 && ww.deadLetterHandler != nil {
		log.Info(ctx, "Message Handler: Killing after 3 attempts")
		err = ww.killMessage(ctx, delivery, msg, handlerError)
		return err
	} else {
		log.Info(ctx, "Message Handler: Requeuing message")
		err = delivery.Nack(false, true) // 'requeue'
		if err != nil {
			return err
		}
	}
	return nil
}

func (ww *Worker) killMessage(ctx context.Context, delivery amqp.Delivery, msg *messaging_pb.Message, killError error) error {
	if ww.deadLetterHandler == nil {
		return fmt.Errorf("no dead letter handler")
	}

	if msg == nil {
		// Unparsable message
		msg = &messaging_pb.Message{
			MessageId: delivery.MessageId,
			Body: &messaging_pb.Any{
				TypeUrl:  RawMessageName,
				Encoding: messaging_pb.WireEncoding_RAW,
				Value:    []byte(delivery.Body),
			},
		}
	}

	meta := map[string]string{
		"messageId": delivery.MessageId,
		"queueName": ww.queueName,
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
	err = delivery.Ack(false)
	if err != nil {
		return err
	}
	return nil
}
