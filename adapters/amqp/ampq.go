package amqp

import (
	"fmt"
	"strings"
	"sync"

	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_pb"
	amqp "github.com/rabbitmq/amqp091-go"
)

type AMQPConfig struct {
	URI      string `env:"AMQP_URI" default:""`
	Exchange string `env:"AMQP_EXCHANGE" default:""`
	Queue    string `env:"AMQP_QUEUE" default:""`
}

type Connection struct {
	connLock   sync.Mutex
	connection *amqp.Connection
	channel    *amqp.Channel
	exchange   string
}

func NewConnection(config AMQPConfig, envName string) (*Connection, error) {
	conn, err := amqp.Dial(config.URI)
	if err != nil {
		return nil, err
	}

	exchange := config.Exchange
	if exchange == "" {
		exchange = fmt.Sprintf("o5.exchange.%s", envName)
	}

	cw := &Connection{
		connection: conn,
		exchange:   exchange,
	}
	return cw, nil
}

func (p *Connection) getChannel() (*amqp.Channel, error) {
	p.connLock.Lock()
	defer p.connLock.Unlock()
	if p.channel != nil {
		if p.channel.IsClosed() {
			p.channel = nil
		} else {
			return p.channel, nil
		}
	}
	ch, err := p.connection.Channel()
	if err != nil {
		return nil, err
	}
	p.channel = ch
	return ch, nil
}

/* Topic Breakdown:

These are defined in o5 configs:

o5-infra/{topicName} is an SNS subscription only, not compatible.
global/{globalType}

/{package}.{topic}
/{package}.{topic}/{endpoint}
{topicName}

/{package}.{topic} becomes service.{package}.{topic}

// when published
// foo.v1.FooService/FooMethod becomes service.foo/v1/FooService.FooMethod

// when subscribing:
// a config of /foo.v1.FooService
// will become service.foo/v1/FooService.*

// a config of /foo.v1.FooService/FooMethod
// will become service.foo/v1/FooService.FooMethod

*/

func messageToRoutingKey(message *messaging_pb.Message) string {

	/*
		if message.DestinationTopic != "" {
			return fmt.Sprintf("topic.%s", message.DestinationTopic)
		}*/

	serviceShash := strings.ReplaceAll(message.GrpcService, ".", "/")

	return fmt.Sprintf("service.%s.%s", serviceShash, message.GrpcMethod)
}
