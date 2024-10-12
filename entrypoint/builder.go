package entrypoint

import (
	"fmt"
	"net/http"

	"github.com/pentops/jwtauth/jwks"
	"github.com/pentops/o5-runtime-sidecar/awsmsg"
	"github.com/pentops/o5-runtime-sidecar/pgproxy"
	"github.com/pentops/o5-runtime-sidecar/proxy"
	"github.com/pentops/o5-runtime-sidecar/sqslink"
	"github.com/rs/cors"
)

type ServerConfig struct {
	// Port to expose to the external LB
	PublicAddr  string   `env:"PUBLIC_ADDR" default:""`
	JWKS        []string `env:"JWKS" default:""`
	StaticFiles string   `env:"STATIC_FILES" default:""`
	CORSOrigins []string `env:"CORS_ORIGINS" default:""`
}

type WorkerConfig struct {
	SQSURL        string `env:"SQS_URL" default:""`
	ResendChance  int    `env:"RESEND_CHANCE" required:"false"`
	NoDeadLetters bool   `env:"NO_DEADLETTERS" default:"false"`
}

type PostgresConfig struct {
	PostgresOutboxURI []string `env:"POSTGRES_OUTBOX" default:""`
	PostgresProxy     []string `env:"POSTGRES_IAM_PROXY" default:""`
	PostgresProxyBind string   `env:"POSTGRES_PROXY_BIND" default:"/socket/postgres"`
}

type Config struct {
	AppName         string `env:"APP_NAME" `
	EnvironmentName string `env:"ENVIRONMENT_NAME" `
	SidecarVersion  string // set from main

	ServerConfig
	WorkerConfig
	PostgresConfig

	// Port to expose locally to the running service(s). 0 disables
	AdapterAddr string `env:"ADAPTER_ADDR" default:""`

	ServiceEndpoints []string `env:"SERVICE_ENDPOINT" default:""`

	EventBridgeARN string `env:"EVENTBRIDGE_ARN" default:""`
}

func FromConfig(envConfig Config, awsConfig AWSProvider) (*Runtime, error) {
	rt := NewRuntime()
	rt.endpoints = envConfig.ServiceEndpoints

	srcConfig := awsmsg.SourceConfig{
		SourceApp:      envConfig.AppName,
		SourceEnv:      envConfig.EnvironmentName,
		SidecarVersion: envConfig.SidecarVersion,
	}

	if envConfig.EventBridgeARN != "" {
		rt.sender = awsmsg.NewEventBridgePublisher(awsConfig.EventBridge(), envConfig.EventBridgeARN)
	}

	pgConfigs := newPGConnSet(awsConfig)
	if len(envConfig.PostgresOutboxURI) > 0 {
		if rt.sender == nil {
			return nil, fmt.Errorf("outbox requires a sender (set EVENTBRIDGE_ARN)")
		}

		for _, rawVar := range envConfig.PostgresOutboxURI {
			conn, err := pgConfigs.getConnector(rawVar)
			if err != nil {
				return nil, fmt.Errorf("building postgres connection: %w", err)
			}

			listener, err := newOutboxListener(conn, rt.sender, srcConfig)
			if err != nil {
				return nil, fmt.Errorf("creating outbox listener: %w", err)
			}
			rt.outboxListeners = append(rt.outboxListeners, listener)
		}
	}

	if len(envConfig.PostgresProxy) > 0 {
		connectors := map[string]pgproxy.PGConnector{}
		for _, rawVar := range envConfig.PostgresProxy {
			conn, err := pgConfigs.getConnector(rawVar)
			if err != nil {
				return nil, fmt.Errorf("building postgres connection: %w", err)
			}
			connectors[conn.Name()] = conn
		}

		proxy, err := newPostgresProxy(envConfig.PostgresProxyBind, connectors)
		if err != nil {
			return nil, fmt.Errorf("creating postgres proxy: %w", err)
		}

		rt.postgresProxy = proxy
	}

	if envConfig.SQSURL != "" {
		var sender sqslink.DeadLetterHandler
		if !envConfig.NoDeadLetters {
			if rt.sender == nil {
				return nil, fmt.Errorf("outbox requires a sender (set EVENTBRIDGE_ARN)")
			}

			sender = sqslink.NewO5MessageDeadLetterHandler(rt.sender, srcConfig)
		}
		rt.queueWorker = sqslink.NewWorker(awsConfig.SQS(), envConfig.SQSURL, sender, envConfig.ResendChance)
	}

	if envConfig.AdapterAddr != "" {
		if rt.sender == nil {
			return nil, fmt.Errorf("adapter requires a sender")
		}
		rt.adapter = newAdapterServer(envConfig.AdapterAddr, rt.sender, srcConfig)
	}

	if envConfig.PublicAddr != "" {

		router := proxy.NewRouter()

		router.SetHealthCheck("/healthz", func() error {
			return nil
		})

		router.AddMiddleware(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Sidecar-Version", envConfig.SidecarVersion)
				next.ServeHTTP(w, r)
			})
		})

		if len(envConfig.CORSOrigins) > 0 {
			router.AddMiddleware(cors.New(cors.Options{
				AllowedOrigins:   envConfig.CORSOrigins,
				AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
				AllowedHeaders:   []string{"*"},
				AllowCredentials: true,
			}).Handler)
		}

		if envConfig.StaticFiles != "" {
			router.SetNotFoundHandler(http.FileServer(http.Dir(envConfig.StaticFiles)))
		}

		rt.routerServer = newRouterServer(envConfig.PublicAddr, router)

		if len(envConfig.JWKS) > 0 {
			jwksManager := jwks.NewKeyManager()
			if err := jwksManager.AddSourceURLs(envConfig.JWKS...); err != nil {
				return nil, fmt.Errorf("configuring JWKS: %w", err)
			}

			rt.jwks = jwksManager
			rt.routerServer.SetJWKS(rt.jwks)
		}

	}

	return rt, nil
}
