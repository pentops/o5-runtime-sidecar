package outbox

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

type EventBridgeAPI interface {
	PutEvents(ctx context.Context, params *eventbridge.PutEventsInput, optFns ...func(*eventbridge.Options)) (*eventbridge.PutEventsOutput, error)
}

type EventBridgePublisher struct {
	client          EventBridgeAPI
	busARN          string
	sourceName      string
	environmentName string
}

func NewEventBridgePublisher(client EventBridgeAPI, busARN, sourceName, environmentName string) *EventBridgePublisher {
	return &EventBridgePublisher{client: client, busARN: busARN, sourceName: sourceName, environmentName: environmentName}
}

func (b *EventBridgePublisher) SendOne(ctx context.Context, message RoutableMessage) error {
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

func (p *EventBridgePublisher) SendMarshalled(ctx context.Context, msg MarshalledMessage) error {
	prepared := &Message{
		ID:          uuid.NewString(),
		Message:     msg.Body,
		Headers:     msg.Headers,
		Destination: msg.Topic,
	}

	if err := prepared.expandAndValidate(); err != nil {
		return err
	}

	_, err := p.SendMultiBatch(ctx, []*Message{prepared})
	return err
}

func (p *EventBridgePublisher) SendMultiBatch(ctx context.Context, msgs []*Message) ([]string, error) {

	// TODO: Calculate the size of the request, the 'total entry size' must be
	// less than 256KB
	// 14 bytes for the Time parameter
	// Source, Detail and DetailType in UTF-8
	// https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-putevent-size.html
	entries := make([]types.PutEventsRequestEntry, len(msgs))
	for idx, msg := range msgs {
		msg.SourceEnvironment = p.environmentName
		detailJSON, err := json.Marshal(msg)
		if err != nil {
			return nil, err
		}

		fmt.Println(string(detailJSON))

		entries[idx] = types.PutEventsRequestEntry{
			Detail:       aws.String(string(detailJSON)),
			DetailType:   aws.String("o5-message"),
			Source:       aws.String(p.sourceName),
			EventBusName: aws.String(p.busARN),
		}

	}

	res, err := p.client.PutEvents(ctx, &eventbridge.PutEventsInput{
		Entries: entries,
	})
	if err != nil {
		return nil, err
	}

	var firstError error

	successfulIDs := []string{}
	for idx, entry := range res.Entries {
		request := msgs[idx]
		if entry.ErrorCode != nil {
			if firstError == nil {
				firstError = fmt.Errorf("failed to send event %s: %s %s", request.ID, *entry.ErrorCode, *entry.ErrorMessage)
			}
			continue
		}

		successfulIDs = append(successfulIDs, *entry.EventId)
	}

	return successfulIDs, firstError
}
