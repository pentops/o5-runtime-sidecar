package entrypoint

import (
	"fmt"

	"github.com/pentops/o5-runtime-sidecar/adapters/eventbridge"
	"github.com/pentops/o5-runtime-sidecar/adapters/msgconvert"
	"github.com/pentops/o5-runtime-sidecar/adapters/pgclient"
	"github.com/pentops/o5-runtime-sidecar/apps/bridge"
	"github.com/pentops/o5-runtime-sidecar/apps/httpserver"
	"github.com/pentops/o5-runtime-sidecar/apps/pgoutbox"
	"github.com/pentops/o5-runtime-sidecar/apps/pgproxy"
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
	srcConfig := sidecar.AppInfo{
		SourceApp:      envConfig.AppName,
		SourceEnv:      envConfig.EnvironmentName,
		SidecarVersion: envConfig.SidecarVersion,
	}

	runtime := NewRuntime()
	runtime.endpoints = envConfig.ServiceEndpoints
	runtime.msgConverter = msgconvert.NewConverter(srcConfig)

	if envConfig.EventBridgeConfig.BusARN != "" {
		s, err := eventbridge.NewEventBridgePublisher(awsConfig.EventBridge(), envConfig.EventBridgeConfig)
		if err != nil {
			return nil, fmt.Errorf("creating eventbridge publisher: %w", err)
		}

		runtime.sender = s
	}

	pgConfigs := pgclient.NewConnectorSet(awsConfig, pgclient.EnvProvider{})

	// Listen to a Postgres outbox table
	if len(envConfig.OutboxConfig.PostgresOutboxURI) > 0 {
		if runtime.sender == nil {
			return nil, fmt.Errorf("outbox requires a sender (set EVENTBRIDGE_ARN)")
		}

		a, err := pgoutbox.NewApps(envConfig.OutboxConfig, runtime.msgConverter, runtime.sender, pgConfigs)
		if err != nil {
			return nil, fmt.Errorf("creating outbox listener: %w", err)
		}

		runtime.outboxListeners = append(runtime.outboxListeners, a...)
	}

	// Proxy a Postgres connection, handling IAM auth
	if len(envConfig.PostgresProxy) > 0 {
		p, err := pgproxy.NewApp(envConfig.ProxyConfig, pgConfigs)
		if err != nil {
			return nil, fmt.Errorf("creating postgres proxy: %w", err)
		}

		runtime.postgresProxy = p
	}

	// Subscribe to SQS messages
	if envConfig.WorkerConfig.SQSURL != "" {
		w, err := queueworker.NewApp(envConfig.WorkerConfig, srcConfig, runtime.sender, awsConfig.SQS())
		if err != nil {
			return nil, fmt.Errorf("creating queue worker: %w", err)
		}

		runtime.queueWorker = w
	}

	// Serve an internal gRPC server, for the app to use messaging without an outbox
	if envConfig.BridgeConfig.AdapterAddr != "" {
		if runtime.sender == nil {
			return nil, fmt.Errorf("bridge requires a sender")
		}

		runtime.adapter = bridge.NewApp(envConfig.AdapterAddr, runtime.sender, runtime.msgConverter)
	}

	// Serve a public HTTP server
	if envConfig.PublicAddr != "" {
		r, err := httpserver.NewRouter(envConfig.ServerConfig, srcConfig)
		if err != nil {
			return nil, fmt.Errorf("creating router: %w", err)
		}

		runtime.routerServer = r
	}

	return runtime, nil
}
