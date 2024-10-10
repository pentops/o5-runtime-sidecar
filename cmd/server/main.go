package main

import (
	"context"
	"fmt"
	"os"

	"github.com/pentops/envconf.go/envconf"
	"github.com/pentops/o5-runtime-sidecar/entrypoint"

	"github.com/pentops/log.go/log"
)

var Version string = "dev"

func main() {
	ctx := context.Background()
	ctx = log.WithFields(ctx, map[string]interface{}{
		"application": "o5-runtime-sidecar",
		"version":     Version,
	})

	cfg := entrypoint.Config{}

	if err := envconf.Parse(&cfg); err != nil {
		log.WithError(ctx, err).Error("Config Failure")
		os.Exit(1)
	}

	cfg.SidecarVersion = Version

	if err := run(ctx, cfg); err != nil {
		log.WithError(ctx, err).Error("Failed to serve")
		os.Exit(1)
	}
}

func run(ctx context.Context, envConfig entrypoint.Config) error {
	awsBuilder, err := entrypoint.NewDefaultAWSConfigBuilder(ctx)
	if err != nil {
		return fmt.Errorf("failed to create AWS config builder: %w", err)
	}

	runtime, err := entrypoint.FromConfig(envConfig, awsBuilder)
	if err != nil {
		return fmt.Errorf("from config: %w", err)
	}

	return runtime.Run(ctx)
}
