package outbox

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/pentops/log.go/log"
	"github.com/pentops/o5-go/dante/v1/dante_tpb"
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

func (b *SNSBatcher) DeadLetter(ctx context.Context, message *dante_tpb.DeadMessage) error {
	protoBody, err := proto.Marshal(message)
	if err != nil {
		return err
	}

	encoded := base64.StdEncoding.EncodeToString(protoBody)

	attributes := map[string]types.MessageAttributeValue{}
	headers := message.MessagingHeaders()

	for key, val := range headers {
		if val == "" {
			continue
		}
		attributes[key] = types.MessageAttributeValue{
			StringValue: aws.String(val),
			DataType:    aws.String("String"),
		}
	}

	dest := b.prefix + message.MessagingTopic()

	_, err = b.client.Publish(ctx, &sns.PublishInput{
		Message:           aws.String(encoded),
		MessageAttributes: attributes,
		TopicArn:          aws.String(dest),
	})

	return err
}

type snsMessage struct {
	body       string
	attributes map[string]types.MessageAttributeValue
}

func mapToHeaders(attrs map[string][]string) map[string]types.MessageAttributeValue {
	attributes := map[string]types.MessageAttributeValue{}
	for key, vals := range attrs {
		for _, val := range vals {
			if val == "" {
				continue
			}
			attributes[key] = types.MessageAttributeValue{
				StringValue: aws.String(val),
				DataType:    aws.String("String"),
			}
		}
	}
	return attributes
}

func encodeMessage(msg *Message) *snsMessage {
	body := base64.StdEncoding.EncodeToString(msg.Message)
	attributes := mapToHeaders(msg.Headers)
	return &snsMessage{
		body:       body,
		attributes: attributes,
	}
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
