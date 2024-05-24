package entrypoint

import (
	"fmt"
	"net/http"

	jsonapi_codec "github.com/pentops/jsonapi/codec"
	"github.com/pentops/jsonapi/gen/j5/source/v1/source_j5pb"
	"github.com/pentops/jsonapi/proxy"
	"github.com/pentops/jwtauth/jwks"
	"github.com/pentops/o5-runtime-sidecar/jwtauth"
	"github.com/pentops/o5-runtime-sidecar/outbox"
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
}

func FromConfig(envConfig Config, awsConfig AWSProvider) (*Runtime, error) {
	rt := NewRuntime()
	rt.endpoints = envConfig.ServiceEndpoints

	if envConfig.EventBridgeARN != "" {
		if envConfig.AppName == "" {
			return nil, fmt.Errorf("APP_NAME is required when using eventbridge")
		}
		if envConfig.EnvironmentName == "" {
			return nil, fmt.Errorf("ENVIRONMENT_NAME is required when using eventbridge")
		}

		rt.sender = outbox.NewEventBridgePublisher(awsConfig.EventBridge(), envConfig.EventBridgeARN, envConfig.AppName, envConfig.EnvironmentName)
	} else if envConfig.SNSPrefix != "" {
		rt.sender = outbox.NewSNSBatcher(awsConfig.SNS(), envConfig.SNSPrefix)
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
			rt.outboxListeners = append(rt.outboxListeners, newOutboxListener(name, uri, rt.sender))
		}

	}

	if envConfig.SQSURL != "" {
		var sender sqslink.DeadLetterHandler
		if !envConfig.NoDeadLetters {
			if rt.sender == nil {
				return nil, fmt.Errorf("outbox requires a sender (set SNS_PREFIX or EVENTBRIDGE_ARN)")
			}
			sender = rt.sender
		}
		rt.queueWorker = sqslink.NewWorker(awsConfig.SQS(), envConfig.SQSURL, sender, envConfig.ResendChance)
	}

	if envConfig.AdapterAddr != "" {
		if rt.sender == nil {
			return nil, fmt.Errorf("adapter requires a sender")
		}
		rt.adapter = newAdapterServer(envConfig.AdapterAddr, rt.sender)
	}

	if len(envConfig.JWKS) > 0 {
		jwksManager := jwks.NewKeyManager()
		if err := jwksManager.AddSourceURLs(envConfig.JWKS...); err != nil {
			return nil, fmt.Errorf("configuring JWKS: %w", err)
		}

		rt.jwks = jwksManager
	}

	if envConfig.PublicAddr != "" {
		codecOptions := &source_j5pb.CodecOptions{
			ShortEnums: &source_j5pb.ShortEnumOptions{
				UnspecifiedSuffix: "UNSPECIFIED",
				StrictUnmarshal:   true,
			},
			WrapOneof: true,
		}

		router := proxy.NewRouter(jsonapi_codec.NewCodec(codecOptions))

		router.HealthCheck("/healthz", func() error {
			return nil
		})

		if len(envConfig.CORSOrigins) > 0 {
			router.Use(cors.New(cors.Options{
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
			rt.routerServer.globalAuth = proxy.AuthHeadersFunc(jwtauth.JWKSAuthFunc(rt.jwks))
		}

	}

	return rt, nil
}
