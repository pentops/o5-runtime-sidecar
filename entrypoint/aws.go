package entrypoint

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/pentops/log.go/log"
)

type configLoaderFunc func(ctx context.Context) (aws.Config, error)

type AWSConfigBuilder struct {
	config       *aws.Config
	configLoader configLoaderFunc
}

var _ AWSProvider = (*AWSConfigBuilder)(nil)

func NewAWSConfigBuilder(provided configLoaderFunc) *AWSConfigBuilder {
	return &AWSConfigBuilder{configLoader: provided}
}

func (acb *AWSConfigBuilder) getConfig(ctx context.Context) (aws.Config, error) {
	if acb.config == nil {
		cfg, err := acb.configLoader(ctx)
		if err != nil {
			return aws.Config{}, fmt.Errorf("couldn't load aws config: %w", err)
		}
		acb.config = &cfg
	}
	return *acb.config, nil
}

func (acb *AWSConfigBuilder) SNS(ctx context.Context) (SNSAPI, error) {
	config, err := acb.getConfig(ctx)
	if err != nil {
		return nil, err
	}
	return sns.NewFromConfig(config), nil
}

func (acb *AWSConfigBuilder) SQS(ctx context.Context) (SQSAPI, error) {
	config, err := acb.getConfig(ctx)
	if err != nil {
		return nil, err
	}
	return sqs.NewFromConfig(config), nil
}

func (acb *AWSConfigBuilder) EventBridge(ctx context.Context) (EventBridgeAPI, error) {
	config, err := acb.getConfig(ctx)
	if err != nil {
		return nil, err
	}
	return eventbridge.NewFromConfig(config), nil
}

func (acb *AWSConfigBuilder) Region() string {
	return acb.config.Region
}

func (acb *AWSConfigBuilder) Credentials(ctx context.Context) (aws.CredentialsProvider, error) {
	config, err := acb.getConfig(ctx)
	if err != nil {
		return nil, err
	}
	return config.Credentials, nil
}

func NewDefaultAWSConfigBuilder(ctx context.Context) (*AWSConfigBuilder, error) {
	return NewAWSConfigBuilder(func(ctx context.Context) (aws.Config, error) {

		cfg, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			return aws.Config{}, fmt.Errorf("couldn't load aws config: %w", err)
		}

		stsClient := sts.NewFromConfig(cfg)
		callerIdentity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
		if err != nil {
			return aws.Config{}, err
		}

		log.WithFields(ctx, map[string]any{
			"account": aws.ToString(callerIdentity.Account),
			"arn":     aws.ToString(callerIdentity.Arn),
			"user":    aws.ToString(callerIdentity.UserId),
		}).Info("Sidecar AWS Identity")
		return cfg, nil
	}), nil
}

type AWSProvider interface {
	SNS(context.Context) (SNSAPI, error)
	SQS(context.Context) (SQSAPI, error)
	EventBridge(context.Context) (EventBridgeAPI, error)

	Region() string
	Credentials(context.Context) (aws.CredentialsProvider, error)
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
