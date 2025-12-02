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

type Connector struct {
	Config AMQPConfig

	dialLock sync.Mutex

	_conn    *amqp.Connection
	_channel *amqp.Channel
}

func NewConnector(config AMQPConfig) *Connector {
	return &Connector{
		Config: config,
	}
}

func (c *Connector) Channel() (*amqp.Channel, error) {
	c.dialLock.Lock()
	defer c.dialLock.Unlock()

	// if we already have a valid channel, return it.
	if c._channel != nil && !c._channel.IsClosed() {
		return c._channel, nil
	}

	var err error

	// if we don't have a valid connection, create it.
	if c._conn == nil || c._conn.IsClosed() {
		c._conn, err = amqp.Dial(c.Config.URI)
		if err != nil {
			return nil, err
		}
	}

	ch, err := c._conn.Channel()
	if err != nil {
		return nil, err
	}

	c._channel = ch
	return c._channel, nil
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
