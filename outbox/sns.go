package outbox

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/pentops/log.go/log"
	"google.golang.org/protobuf/proto"
)

type SNSAPI interface {
	PublishBatch(ctx context.Context, params *sns.PublishBatchInput, optFns ...func(*sns.Options)) (*sns.PublishBatchOutput, error)
	Publish(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error)
}

type SNSBatcher struct {
	client SNSAPI
	prefix string
}

func NewSNSBatcher(client SNSAPI, prefix string) *SNSBatcher {
	return &SNSBatcher{
		client: client,
		prefix: prefix,
	}
}

type MarshalledMessage struct {
	Body    []byte
	Headers map[string]string
	Topic   string
}

func (b *SNSBatcher) SendMarshalled(ctx context.Context, message MarshalledMessage) error {
	encoded := base64.StdEncoding.EncodeToString(message.Body)
	attributes := map[string]types.MessageAttributeValue{}

	for key, val := range message.Headers {
		if val == "" {
			continue
		}
		attributes[key] = types.MessageAttributeValue{
			StringValue: aws.String(val),
			DataType:    aws.String("String"),
		}
	}

	dest := b.prefix + message.Topic

	_, err := b.client.Publish(ctx, &sns.PublishInput{
		Message:           aws.String(encoded),
		MessageAttributes: attributes,
		TopicArn:          aws.String(dest),
	})

	return err
}

type RoutableMessage interface {
	MessagingTopic() string
	MessagingHeaders() map[string]string
	proto.Message
}

func (b *SNSBatcher) SendOne(ctx context.Context, message RoutableMessage) error {
	protoBody, err := proto.Marshal(message)
	if err != nil {
		return err
	}

	return b.SendMarshalled(ctx, MarshalledMessage{
		Body:    protoBody,
		Headers: message.MessagingHeaders(),
		Topic:   message.MessagingTopic(),
	})

}

type snsMessage struct {
	body       string
	attributes map[string]types.MessageAttributeValue
}

func mapToHeaders(msg *Message) map[string]types.MessageAttributeValue {
	attributes := map[string]types.MessageAttributeValue{}
	for key, val := range msg.Headers {
		if val == "" {
			continue
		}
		attributes[key] = types.MessageAttributeValue{
			StringValue: aws.String(val),
			DataType:    aws.String("String"),
		}
	}
	attributes["grpc-service"] = types.MessageAttributeValue{
		StringValue: aws.String(fmt.Sprintf("/%s/%s", msg.GrpcService, msg.GrpcMethod)),
		DataType:    aws.String("String"),
	}
	attributes["grpc-message"] = types.MessageAttributeValue{
		StringValue: aws.String(msg.GrpcMessage),
		DataType:    aws.String("String"),
	}
	if msg.ReplyDest != nil {
		attributes["o5-reply-reply-to"] = types.MessageAttributeValue{
			StringValue: aws.String(*msg.ReplyDest),
			DataType:    aws.String("String"),
		}
	}

	return attributes
}

func encodeMessage(msg *Message) *snsMessage {
	body := base64.StdEncoding.EncodeToString(msg.Message)
	attributes := mapToHeaders(msg)

	return &snsMessage{
		body:       body,
		attributes: attributes,
	}
}

func (b *SNSBatcher) SendMultiBatch(ctx context.Context, msgs []*Message) ([]string, error) {

	byDestination := map[string][]*Message{}
	successIDs := []string{}

	for _, msg := range msgs {
		byDestination[msg.Destination] = append(byDestination[msg.Destination], msg)
	}

	for destination, messages := range byDestination {
		if err := b.SendBatch(ctx, destination, messages); err != nil {
			return nil, fmt.Errorf("error sending batch of outbox messages: %w", err)
		}
		for _, msg := range messages {
			successIDs = append(successIDs, msg.ID)
		}
	}
	return successIDs, nil
}

func (b *SNSBatcher) SendBatch(ctx context.Context, destination string, msgs []*Message) error {
	dest := b.prefix + destination

	for len(msgs) > 0 {
		var batch []*Message
		if len(msgs) > 10 {
			batch = msgs[:10]
			msgs = msgs[10:]
		} else {
			batch = msgs
			msgs = nil
		}

		var entries []types.PublishBatchRequestEntry
		for _, msg := range batch {
			encoded := encodeMessage(msg)

			entries = append(entries, types.PublishBatchRequestEntry{
				Id:                aws.String(msg.ID),
				Message:           aws.String(encoded.body),
				MessageAttributes: encoded.attributes,
			})
		}

		log.WithFields(ctx, map[string]interface{}{
			"destination": dest,
			"count":       len(batch),
		}).Debug("Sending Outbox Batch")

		out, err := b.client.PublishBatch(ctx, &sns.PublishBatchInput{
			PublishBatchRequestEntries: entries,
			TopicArn:                   aws.String(dest),
		})
		if err != nil {
			log.WithError(ctx, err).Error("Failed to send batch")
			return err
		}
		if len(out.Failed) > 0 {
			// TODO: Check if this can come up for some but not all. In theory,
			// only permissions and size should throw, we can pre-check size and permissions
			// should be the same for all.
			ff := out.Failed[0]
			return fmt.Errorf("at least one message failed: %s: %s", *ff.Code, *ff.Message)
		}
	}

	return nil
}
