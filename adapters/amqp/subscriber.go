package amqp

import (
	"context"
	"fmt"

	"github.com/pentops/j5/lib/j5codec"
	"github.com/pentops/log.go/log"
	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_pb"
	"github.com/pentops/o5-runtime-sidecar/apps/queueworker/messaging"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Subscriber struct {
	queueName string
	conn      *Connection
	handler   messaging.Handler
}

func NewSubscriber(conn *Connection, queueName string, router messaging.Handler) (*Subscriber, error) {
	sub := &Subscriber{
		conn:      conn,
		queueName: queueName,
		handler:   router,
	}
	return sub, nil

}

func (p *Subscriber) Run(ctx context.Context) error {
	ch, err := p.conn.getChannel()
	if err != nil {
		return err
	}

	delivery, err := ch.ConsumeWithContext(ctx,
		p.queueName, // queue
		"",          // consumer
		false,       // autoAck
		false,       // exclusive
		false,       // noLocal
		false,       // noWait
		nil,         // args
	)
	if err != nil {
		return err
	}
	defer ch.Close()
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-delivery:
			if !ok {
				return fmt.Errorf("AMQP delivery channel closed")
			}

			err = p.handleDelivery(ctx, msg)
			if err != nil {
				return err
			}
		}
	}

}

func (p *Subscriber) handleDelivery(ctx context.Context, delivery amqp.Delivery) error {
	count, ok := delivery.Headers["x-delivery-count"].(int64)
	if !ok {
		count = 0
	}

	if delivery.ContentType != "application/o5-message" {
		return fmt.Errorf("invalid content type: %s", delivery.ContentType)
	}

	msg := &messaging_pb.Message{}
	err := j5codec.Global.JSONToProto(delivery.Body, msg.ProtoReflect())
	if err != nil {
		return err
	}

	handlerError := p.handler.HandleMessage(ctx, msg)
	if handlerError == nil { // LOGIC INVERSION
		err = delivery.Ack(false)
		if err != nil {
			return err
		}
		return nil
	}
	log.WithError(ctx, handlerError).Error("Message Handler: Error")
	if count >= 3 {
		log.Info(ctx, "Message Handler: Killing after 3 attempts")

	} else {
		log.Info(ctx, "Message Handler: Requeuing message")
		err = delivery.Nack(false, true) // 'requeue', meaning a DLX should be set up
	}
	return err
}
