package amqp

import (
	"context"
	"errors"

	"github.com/pentops/j5/lib/j5codec"
	"github.com/pentops/log.go/log"
	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_pb"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Publisher struct {
	conn *Connection
}

func NewPublisher(conn *Connection) (*Publisher, error) {
	cw := &Publisher{
		conn: conn,
	}
	return cw, nil
}

func (p *Publisher) Publish(ctx context.Context, message *messaging_pb.Message) error {
	routingKey := messageToRoutingKey(message)

	ch, err := p.conn.getChannel()
	if err != nil {
		return err
	}

	// eventbridge requires JSON bodies.
	detail, err := j5codec.Global.ProtoToJSON(message.ProtoReflect())
	if err != nil {
		return err
	}

	log.WithFields(ctx, "exchange", p.conn.exchange, "routing_key", routingKey).Info("Publishing message to AMQP")
	return ch.PublishWithContext(ctx,
		p.conn.exchange, // exchange
		routingKey,      // routing key
		false,           // mandatory
		false,           // immediate
		amqp.Publishing{
			ContentType: "application/o5-message",
			Body:        detail,
		},
	)

}

func (p *Publisher) PublishBatch(ctx context.Context, messages []*messaging_pb.Message) ([]string, error) {

	errs := make([]error, 0)
	ids := make([]string, 0, len(messages))
	for _, msg := range messages {
		err := p.Publish(ctx, msg)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		ids = append(ids, msg.MessageId)
	}
	if len(errs) > 0 {
		return ids, errors.Join(errs...)
	}

	return ids, nil
}
