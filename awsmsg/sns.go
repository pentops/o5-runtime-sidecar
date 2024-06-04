package awsmsg

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/pentops/log.go/log"
	"github.com/pentops/o5-go/messaging/v1/messaging_pb"
)

type SNSAPI interface {
	Publish(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error)
	PublishBatch(ctx context.Context, params *sns.PublishBatchInput, optFns ...func(*sns.Options)) (*sns.PublishBatchOutput, error)
}

type SNSPublisher struct {
	client   SNSAPI
	topicARN string
}

func NewSNSPublisher(client SNSAPI, topicARN string) *SNSPublisher {
	return &SNSPublisher{
		client:   client,
		topicARN: topicARN,
	}
}

func (p *SNSPublisher) PublisherID() string {
	return p.topicARN
}

func prepareSNSMessage(msg *messaging_pb.Message) types.PublishBatchRequestEntry {
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

	attributes["o5-source-app"] = types.MessageAttributeValue{
		StringValue: aws.String(msg.SourceApp),
		DataType:    aws.String("String"),
	}

	attributes["o5-source-env"] = types.MessageAttributeValue{
		StringValue: aws.String(msg.SourceEnv),
		DataType:    aws.String("String"),
	}

	attributes["o5-message-id"] = types.MessageAttributeValue{
		StringValue: aws.String(msg.MessageId),
		DataType:    aws.String("String"),
	}

	attributes["grpc-method"] = types.MessageAttributeValue{
		StringValue: aws.String(msg.GrpcMethod),
		DataType:    aws.String("String"),
	}

	attributes["grpc-message"] = types.MessageAttributeValue{
		StringValue: aws.String(strings.TrimPrefix(msg.Body.TypeUrl, "type.googleapis.com/")),
		DataType:    aws.String("String"),
	}

	if msg.ReplyDest != nil {
		attributes["o5-reply-reply-to"] = types.MessageAttributeValue{
			StringValue: aws.String(*msg.ReplyDest),
			DataType:    aws.String("String"),
		}
	}

	body := base64.StdEncoding.EncodeToString(msg.Body.Value)

	return types.PublishBatchRequestEntry{
		Id:                aws.String(msg.MessageId),
		Message:           aws.String(string(body)),
		MessageAttributes: attributes,
	}
}

func (p *SNSPublisher) Publish(ctx context.Context, message *messaging_pb.Message) error {
	prepared := prepareSNSMessage(message)

	input := &sns.PublishInput{
		Message:           prepared.Message,
		TopicArn:          &p.topicARN,
		MessageAttributes: prepared.MessageAttributes,
	}
	_, err := p.client.Publish(ctx, input)
	return err
}

func (p *SNSPublisher) PublishBatch(ctx context.Context, msgs []*messaging_pb.Message) ([]string, error) {

	byDestination := map[string][]types.PublishBatchRequestEntry{}

	for _, msg := range msgs {
		topic := msg.DestinationTopic
		prepared := prepareSNSMessage(msg)
		byDestination[topic] = append(byDestination[topic], prepared)
	}

	batches := make([]*sns.PublishBatchInput, 0, len(byDestination))

	for destTopic, messages := range byDestination {
		topicARN := p.topicARN + destTopic
		for len(messages) > 0 {
			var batch []types.PublishBatchRequestEntry
			if len(messages) > 10 {
				batch = messages[:10]
				messages = messages[10:]
			} else {
				batch = messages
				messages = nil
			}
			batches = append(batches, &sns.PublishBatchInput{
				PublishBatchRequestEntries: batch,
				TopicArn:                   aws.String(topicARN),
			})
		}
	}

	var successIDs []string
	for _, batch := range batches {
		out, err := p.client.PublishBatch(ctx, batch)
		if err != nil {
			// return IDs of previous publishes
			return successIDs, err
		}

		for _, entry := range out.Successful {
			successIDs = append(successIDs, *entry.Id)
			log.WithFields(ctx, map[string]interface{}{
				"topicArn":  p.topicARN,
				"messageId": *entry.Id,
			}).Info("Published to SNS")
		}

		if len(out.Failed) > 0 {
			for _, entry := range out.Failed {
				log.WithFields(ctx, map[string]interface{}{
					"topicArn":     p.topicARN,
					"messageId":    entry.Id,
					"errorCode":    *entry.Code,
					"errorMessage": *entry.Message,
				}).Error("Failed to PublishBatch event to SNS")
			}

			// err earlier likely already is not nil, but just in case.
			return successIDs, fmt.Errorf("failed to send batch")
		}
	}

	return successIDs, nil

}
