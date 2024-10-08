package entrypoint

import (
	"fmt"
	"net/http"

	"github.com/pentops/jwtauth/jwks"
	"github.com/pentops/o5-runtime-sidecar/awsmsg"
	"github.com/pentops/o5-runtime-sidecar/proxy"
	"github.com/pentops/o5-runtime-sidecar/sqslink"
	"github.com/rs/cors"
)

type Config struct {
	// Port to expose to the external LB. 0 disables
	PublicAddr string `env:"PUBLIC_ADDR" default:""`

	// Port to expose locally to the running service(s). 0 disables
	AdapterAddr string `env:"ADAPTER_ADDR" default:""`

	ServiceEndpoints []string `env:"SERVICE_ENDPOINT" default:""`
	StaticFiles      string   `env:"STATIC_FILES" default:""`
	SQSURL           string   `env:"SQS_URL" default:""`

	PostgresOutboxURI []string `env:"POSTGRES_OUTBOX" default:""`
	SNSPrefix         string   `env:"SNS_PREFIX" default:""`
	EventBridgeARN    string   `env:"EVENTBRIDGE_ARN" default:""`
	AppName           string   `env:"APP_NAME" default:""`
	EnvironmentName   string   `env:"ENVIRONMENT_NAME" default:""`

	CORSOrigins []string `env:"CORS_ORIGINS" default:""`

	JWKS []string `env:"JWKS" default:""`

	ResendChance int `env:"RESEND_CHANCE" required:"false"`

	NoDeadLetters bool `env:"NO_DEADLETTERS" default:"false"`

	// set from main
	SidecarVersion string
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
		if envConfig.AppName == "" {
			return nil, fmt.Errorf("APP_NAME is required when using eventbridge")
		}
		if envConfig.EnvironmentName == "" {
			return nil, fmt.Errorf("ENVIRONMENT_NAME is required when using eventbridge")
		}

		rt.sender = awsmsg.NewEventBridgePublisher(awsConfig.EventBridge(), envConfig.EventBridgeARN)
	} else if envConfig.SNSPrefix != "" {
		rt.sender = awsmsg.NewSNSPublisher(awsConfig.SNS(), envConfig.SNSPrefix)
	}

	if len(envConfig.PostgresOutboxURI) > 0 {
		if rt.sender == nil {
			return nil, fmt.Errorf("outbox requires a sender (set SNS_PREFIX)")
		}

		for _, uri := range envConfig.PostgresOutboxURI {
			uri := uri
			name := "outbox"
			if len(envConfig.PostgresOutboxURI) > 1 {
				name = fmt.Sprintf("outbox-%s", uri)
			}
			listener, err := newOutboxListener(name, uri, rt.sender, srcConfig)
			if err != nil {
				return nil, fmt.Errorf("creating outbox listener: %w", err)
			}
			rt.outboxListeners = append(rt.outboxListeners, listener)
		}

	}

	if envConfig.SQSURL != "" {
		var sender sqslink.DeadLetterHandler
		if !envConfig.NoDeadLetters {
			if rt.sender == nil {
				return nil, fmt.Errorf("outbox requires a sender (set SNS_PREFIX or EVENTBRIDGE_ARN)")
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

	if len(envConfig.JWKS) > 0 {
		jwksManager := jwks.NewKeyManager()
		if err := jwksManager.AddSourceURLs(envConfig.JWKS...); err != nil {
			return nil, fmt.Errorf("configuring JWKS: %w", err)
		}

		rt.jwks = jwksManager
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
		if rt.jwks != nil {
			rt.routerServer.SetJWKS(rt.jwks)
		}

	}

	return rt, nil
}
