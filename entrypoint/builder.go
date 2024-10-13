package entrypoint

import (
	"fmt"
	"os"

	"github.com/pentops/o5-runtime-sidecar/adapters/eventbridge"
	"github.com/pentops/o5-runtime-sidecar/adapters/postgres"
	"github.com/pentops/o5-runtime-sidecar/apps/bridge"
	"github.com/pentops/o5-runtime-sidecar/apps/httpserver"
	"github.com/pentops/o5-runtime-sidecar/apps/postgres/pgoutbox"
	"github.com/pentops/o5-runtime-sidecar/apps/postgres/pgproxy"
	"github.com/pentops/o5-runtime-sidecar/apps/queueworker"
	"github.com/pentops/o5-runtime-sidecar/sidecar"
)

type Config struct {
	AppName         string `env:"APP_NAME" `
	EnvironmentName string `env:"ENVIRONMENT_NAME" `
	SidecarVersion  string // set from main

	httpserver.ServerConfig
	queueworker.WorkerConfig
	pgproxy.ProxyConfig
	pgoutbox.OutboxConfig
	bridge.BridgeConfig
	eventbridge.EventBridgeConfig

	ServiceEndpoints []string `env:"SERVICE_ENDPOINT" default:""`
}

type Publisher interface {
	pgoutbox.Batcher
	queueworker.Publisher
}

func FromConfig(envConfig Config, awsConfig AWSProvider) (*Runtime, error) {
	rt := NewRuntime()
	rt.endpoints = envConfig.ServiceEndpoints

	srcConfig := sidecar.AppInfo{
		SourceApp:      envConfig.AppName,
		SourceEnv:      envConfig.EnvironmentName,
		SidecarVersion: envConfig.SidecarVersion,
	}

	if envConfig.EventBridgeARN != "" {
		rt.sender = eventbridge.NewEventBridgePublisher(awsConfig.EventBridge(), envConfig.EventBridgeARN)
	}

	os.Environ()

	pgConfigs := postgres.NewConnectorSet(awsConfig, postgres.EnvProvider{})

	if len(envConfig.PostgresOutboxURI) > 0 {
		if rt.sender == nil {
			return nil, fmt.Errorf("outbox requires a sender (set EVENTBRIDGE_ARN)")
		}
		apps, err := pgoutbox.NewApps(envConfig.OutboxConfig, srcConfig, rt.sender, pgConfigs)
		if err != nil {
			return nil, fmt.Errorf("creating outbox listener: %w", err)
		}
		rt.outboxListeners = append(rt.outboxListeners, apps...)
	}

	if len(envConfig.PostgresProxy) > 0 {
		proxy, err := pgproxy.NewApp(envConfig.ProxyConfig, pgConfigs)
		if err != nil {
			return nil, fmt.Errorf("creating postgres proxy: %w", err)
		}
		rt.postgresProxy = proxy
	}

	if envConfig.WorkerConfig.SQSURL != "" {
		worker, err := queueworker.NewApp(envConfig.WorkerConfig, srcConfig, rt.sender, awsConfig.SQS())
		if err != nil {
			return nil, fmt.Errorf("creating queue worker: %w", err)
		}
		rt.queueWorker = worker
	}

	if envConfig.AdapterAddr != "" {
		if rt.sender == nil {
			return nil, fmt.Errorf("bridge requires a sender")
		}
		rt.adapter = bridge.NewApp(envConfig.AdapterAddr, rt.sender, srcConfig)
	}

	if envConfig.PublicAddr != "" {
		router, err := httpserver.NewRouter(envConfig.ServerConfig, srcConfig)
		if err != nil {
			return nil, fmt.Errorf("creating router: %w", err)
		}
		rt.routerServer = router
	}

	return rt, nil
}
