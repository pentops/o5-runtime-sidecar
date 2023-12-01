package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/pentops/custom-proto-api/jsonapi"
	"github.com/pentops/o5-runtime-sidecar/outbox"
	"github.com/pentops/o5-runtime-sidecar/runtime"
	"github.com/pentops/o5-runtime-sidecar/sqslink"
	"gopkg.daemonl.com/envconf"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/pentops/log.go/log"
)

var Version string

type EnvConfig struct {
	PublicPort  int    `env:"PUBLIC_PORT" default:"0"`
	Service     string `env:"SERVICE_ENDPOINT" default:""`
	StaticFiles string `env:"STATIC_FILES" default:""`
	SQSURL      string `env:"SQS_URL" default:""`

	PostgresOutboxURI string `env:"POSTGRES_OUTBOX" default:""`
	SNSPrefix         string `env:"SNS_PREFIX" default:""`

	JWKS []string `env:"JWKS" default:""`
}

func main() {

	ctx := context.Background()
	ctx = log.WithFields(ctx, map[string]interface{}{
		"application": "o5-runtime-sidecar",
		"version":     Version,
	})

	cfg := EnvConfig{}

	if err := envconf.Parse(&cfg); err != nil {
		log.WithError(ctx, err).Error("Config Failure")
		os.Exit(1)
	}

	if err := run(ctx, cfg); err != nil {
		log.WithError(ctx, err).Error("Failed to serve")
		os.Exit(1)
	}
}

func run(ctx context.Context, envConfig EnvConfig) error {

	awsConfig, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	codecOptions := jsonapi.Options{
		ShortEnums: &jsonapi.ShortEnumsOption{
			UnspecifiedSuffix: "UNSPECIFIED",
			StrictUnmarshal:   true,
		},
		WrapOneof: true,
	}

	rt := runtime.Runtime{}

	if envConfig.PostgresOutboxURI != "" || envConfig.SQSURL != "" {
		if envConfig.SNSPrefix == "" {
			return fmt.Errorf("SNS prefix required when using Postgres outbox")
		}
		rt.Sender = outbox.NewSNSBatcher(sns.NewFromConfig(awsConfig), envConfig.SNSPrefix)
	}

	if envConfig.SQSURL != "" {
		sqsClient := sqs.NewFromConfig(awsConfig)
		rt.Worker = sqslink.NewWorker(sqsClient, envConfig.SQSURL, rt.Sender)
	}

	if envConfig.PublicPort != 0 {
		if err := rt.AddRouter(envConfig.PublicPort, codecOptions); err != nil {
			return fmt.Errorf("add router: %w", err)
		}
	}

	if envConfig.StaticFiles != "" {
		if err := rt.StaticFiles(envConfig.StaticFiles); err != nil {
			return fmt.Errorf("static files: %w", err)
		}
	}

	if len(envConfig.JWKS) > 0 {
		if err := rt.AddJWKS(ctx, envConfig.JWKS...); err != nil {
			return fmt.Errorf("add JWKS: %w", err)
		}
	}

	endpoints := strings.Split(envConfig.Service, ",")
	for _, endpoint := range endpoints {
		endpoint = strings.TrimSpace(endpoint)
		if endpoint == "" {
			continue
		}
		if err := rt.AddEndpoint(ctx, endpoint); err != nil {
			return fmt.Errorf("add endpoint %s: %w", endpoint, err)
		}
	}

	return rt.Run(ctx)

}
