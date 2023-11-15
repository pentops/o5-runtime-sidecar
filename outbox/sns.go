package outbox

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/pentops/log.go/log"
)

type SNSAPI interface {
	PublishBatch(ctx context.Context, params *sns.PublishBatchInput, optFns ...func(*sns.Options)) (*sns.PublishBatchOutput, error)
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
			body := base64.StdEncoding.EncodeToString(msg.Message)
			attributes := map[string]types.MessageAttributeValue{}
			for key, vals := range msg.Headers {
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
			entries = append(entries, types.PublishBatchRequestEntry{
				Id:                aws.String(msg.ID),
				Message:           aws.String(body),
				MessageAttributes: attributes,
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
