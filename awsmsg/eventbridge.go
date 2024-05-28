package awsmsg

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
	"github.com/pentops/o5-go/messaging/v1/messaging_pb"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	EventBridgeO5MessageDetailType = "o5-message/v1"
)

type EventBridgeAPI interface {
	PutEvents(ctx context.Context, params *eventbridge.PutEventsInput, optFns ...func(*eventbridge.Options)) (*eventbridge.PutEventsOutput, error)
}

type EventBridgePublisher struct {
	client EventBridgeAPI
	BusARN string
}

func NewEventBridgePublisher(client EventBridgeAPI, busARN string) *EventBridgePublisher {
	return &EventBridgePublisher{
		client: client,
		BusARN: busARN,
	}
}

func (p *EventBridgePublisher) Publish(ctx context.Context, message *messaging_pb.Message) error {
	_, err := p.PublishBatch(ctx, []*messaging_pb.Message{message})
	return err
}

func (p *EventBridgePublisher) PublishBatch(ctx context.Context, messages []*messaging_pb.Message) ([]string, error) {

	// TODO: Calculate the size of the request, the 'total entry size' must be
	// less than 256KB
	// 14 bytes for the Time parameter
	// Source, Detail and DetailType in UTF-8
	// https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-putevent-size.html
	entries := make([]types.PutEventsRequestEntry, len(messages))
	for idx, msg := range messages {
		entry, err := p.prepareMessage(msg)
		if err != nil {
			return nil, err
		}
		entries[idx] = *entry
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
		request := messages[idx]
		if entry.ErrorCode != nil {
			if firstError == nil {
				firstError = fmt.Errorf("failed to send event %s: %s %s", request.MessageId, *entry.ErrorCode, *entry.ErrorMessage)
			}
			continue
		}

		successfulIDs = append(successfulIDs, request.MessageId)
	}

	return successfulIDs, firstError
}

func (p *EventBridgePublisher) prepareMessage(input *messaging_pb.Message) (*types.PutEventsRequestEntry, error) {

	// eventbridge requires JSON bodies.
	detai, err := protojson.Marshal(input)
	if err != nil {
		return nil, err
	}

	source := fmt.Sprintf("o5/%s/%s", input.SourceEnv, input.SourceApp)

	entry := &types.PutEventsRequestEntry{
		Detail:       aws.String(string(detai)),
		DetailType:   aws.String(EventBridgeO5MessageDetailType),
		Source:       aws.String(source),
		EventBusName: aws.String(p.BusARN),
	}

	return entry, nil

}
