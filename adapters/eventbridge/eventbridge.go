package eventbridge

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
	"github.com/pentops/j5/lib/j5codec"
	"github.com/pentops/log.go/log"
	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_pb"
)

type EventBridgeConfig struct {
	BusARN string `env:"EVENTBRIDGE_ARN" default:""`
}

const (
	EventBridgeO5MessageDetailType = "o5-message/v1"
)

type EventBridgeAPI interface {
	PutEvents(ctx context.Context, params *eventbridge.PutEventsInput, optFns ...func(*eventbridge.Options)) (*eventbridge.PutEventsOutput, error)
}

type EventBridgeWrapper struct {
	Detail       json.RawMessage `json:"detail"`
	DetailType   string          `json:"detail-type"`
	EventBusName string          `json:"eventBusName"`
	Source       string          `json:"source"`
	Account      string          `json:"account"`
	Region       string          `json:"region"`
	Resources    []string        `json:"resources"`
	Time         string          `json:"time"`
}

type EventBridgePublisher struct {
	client EventBridgeAPI
	EventBridgeConfig
}

func NewEventBridgePublisher(client EventBridgeAPI, config EventBridgeConfig) (*EventBridgePublisher, error) {
	if config.BusARN == "" {
		return nil, fmt.Errorf("missing $EVENTBRIDGE_ARN")
	}

	return &EventBridgePublisher{
		client:            client,
		EventBridgeConfig: config,
	}, nil
}

func (p *EventBridgePublisher) PublisherID() string {
	return p.BusARN
}

func (p *EventBridgePublisher) Publish(ctx context.Context, message *messaging_pb.Message) error {
	_, err := p.PublishBatch(ctx, []*messaging_pb.Message{message})
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	return nil
}

func (p *EventBridgePublisher) PublishBatch(ctx context.Context, messages []*messaging_pb.Message) ([]string, error) {
	// TODO: Calculate the size of the request, the 'total entry size' must be
	// less than 256KB
	// 14 bytes for the Time parameter
	// Source, Detail and DetailType in UTF-8
	// https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-putevent-size.html
	entries := make([]types.PutEventsRequestEntry, len(messages))
	for idx, msg := range messages {
		entry, err := p.buildPutEventEntry(msg)
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
			log.WithFields(ctx, map[string]any{
				"eventBusArn":  p.BusARN,
				"messageId":    request.MessageId,
				"errorCode":    *entry.ErrorCode,
				"errorMessage": *entry.ErrorMessage,
			}).Error("Failed to PutEvent to EventBus")

			if firstError == nil {
				firstError = fmt.Errorf("failed to send event %s: %s %s", request.MessageId, *entry.ErrorCode, *entry.ErrorMessage)
			}

			continue
		}

		log.WithFields(ctx, map[string]any{
			"eventBusArn": p.BusARN,
			"messageId":   request.MessageId,
		}).Info("Published to EventBus")

		successfulIDs = append(successfulIDs, request.MessageId)
	}

	return successfulIDs, firstError
}

func (p *EventBridgePublisher) buildPutEventEntry(input *messaging_pb.Message) (*types.PutEventsRequestEntry, error) {
	// eventbridge requires JSON bodies.
	detail, err := j5codec.Global.ProtoToJSON(input.ProtoReflect())
	if err != nil {
		return nil, err
	}

	source := fmt.Sprintf("o5/%s/%s", input.SourceEnv, input.SourceApp)

	entry := &types.PutEventsRequestEntry{
		Detail:       aws.String(string(detail)),
		DetailType:   aws.String(EventBridgeO5MessageDetailType),
		Source:       aws.String(source),
		EventBusName: aws.String(p.BusARN),
	}

	return entry, nil
}
