package entrypoint

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type AWSConfigBuilder struct {
	config aws.Config
}

func (acb *AWSConfigBuilder) SNS() SNSAPI {
	return sns.NewFromConfig(acb.config)
}

func (acb *AWSConfigBuilder) SQS() SQSAPI {
	return sqs.NewFromConfig(acb.config)
}

func (acb *AWSConfigBuilder) EventBridge() EventBridgeAPI {
	return eventbridge.NewFromConfig(acb.config)
}

func (acb *AWSConfigBuilder) Region() string {
	return acb.config.Region
}

func (acb *AWSConfigBuilder) Credentials() aws.CredentialsProvider {
	return acb.config.Credentials
}

func NewAWSConfigBuilder(provided aws.Config) *AWSConfigBuilder {
	return &AWSConfigBuilder{config: provided}
}

func NewDefaultAWSConfigBuilder(ctx context.Context) (*AWSConfigBuilder, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("couldn't load aws config: %w", err)
	}
	return NewAWSConfigBuilder(cfg), nil
}

type AWSProvider interface {
	SNS() SNSAPI
	SQS() SQSAPI
	EventBridge() EventBridgeAPI

	Region() string
	Credentials() aws.CredentialsProvider
}

// SNSAPI is an interface for the SNS client which satisfies the interfaces of
// other packages
type SNSAPI interface {
	PublishBatch(ctx context.Context, params *sns.PublishBatchInput, optFns ...func(*sns.Options)) (*sns.PublishBatchOutput, error)
	Publish(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error)
}

// SQSAPI is an interface for the SQS client which satisfies the interfaces of
// other packages
type SQSAPI interface {
	ReceiveMessage(ctx context.Context, input *sqs.ReceiveMessageInput, opts ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	DeleteMessage(ctx context.Context, input *sqs.DeleteMessageInput, opts ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error)
}

// EventBridgeAPI is an interface for the EventBridge client which satisfies the
// interfaces of other packages
type EventBridgeAPI interface {
	PutEvents(ctx context.Context, params *eventbridge.PutEventsInput, optFns ...func(*eventbridge.Options)) (*eventbridge.PutEventsOutput, error)
}
