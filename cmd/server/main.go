package main

import (
	"context"
	"fmt"
	"os"
	"reflect"

	"github.com/pentops/o5-runtime-sidecar/entrypoint"
	"github.com/pentops/runner/cliconf"

	"github.com/pentops/log.go/log"
)

var Version string = "dev"

func main() {
	ctx := context.Background()
	ctx = log.WithFields(ctx, map[string]any{
		"application": "o5-runtime-sidecar",
		"version":     Version,
	})

	cfg := &entrypoint.Config{}

	args := os.Args[1:]
	configValue := reflect.ValueOf(cfg)
	parseError := cliconf.ParseCombined(configValue, args)
	if parseError != nil {
		log.WithError(ctx, parseError).Error("Config Failure")
		os.Exit(1)
	}

	cfg.SidecarVersion = Version

	if err := run(ctx, *cfg); err != nil {
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
