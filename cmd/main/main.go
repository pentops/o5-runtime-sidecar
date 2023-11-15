package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/pentops/custom-proto-api/jsonapi"
	"github.com/pentops/custom-proto-api/swagger"
	"github.com/pentops/o5-runtime-sidecar/jwtauth"
	"github.com/pentops/o5-runtime-sidecar/outbox"
	"github.com/pentops/o5-runtime-sidecar/protoread"
	"github.com/pentops/o5-runtime-sidecar/proxy"
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

	var snsSender *outbox.SNSBatcher

	if envConfig.PostgresOutboxURI != "" || envConfig.SQSURL != "" {
		if envConfig.SNSPrefix == "" {
			return fmt.Errorf("SNS prefix required when using Postgres outbox")
		}
		snsSender = outbox.NewSNSBatcher(sns.NewFromConfig(awsConfig), envConfig.SNSPrefix)
	}

	var worker *sqslink.Worker
	if envConfig.SQSURL != "" {
		sqsClient := sqs.NewFromConfig(awsConfig)
		worker = sqslink.NewWorker(sqsClient, envConfig.SQSURL, snsSender)
	}

	var router *proxy.Router
	if envConfig.PublicPort != 0 {
		router = proxy.NewRouter(codecOptions)
	}

	if envConfig.StaticFiles != "" {
		if router == nil {
			return fmt.Errorf("static files configured but no public port")
		}

		router.SetNotFoundHandler(http.FileServer(http.Dir(envConfig.StaticFiles)))
	}

	eg, ctx := errgroup.WithContext(ctx)

	var jwksManager *jwtauth.JWKSManager
	if router != nil && len(envConfig.JWKS) > 0 {
		jwksManager, err = jwtauth.NewKeyManagerFromURLs(envConfig.JWKS...)
		if err != nil {
			return fmt.Errorf("failed to load JWKS: %w", err)
		}

		router.AuthFunc = jwtauth.JWKSAuthFunc(jwksManager)
		eg.Go(func() error {
			return jwksManager.Run(ctx)
		})

	}

	httpServices := make([]protoreflect.ServiceDescriptor, 0)

	endpoints := strings.Split(envConfig.Service, ",")
	for _, endpoint := range endpoints {
		endpoint = strings.TrimSpace(endpoint)
		if endpoint == "" {
			continue
		}

		conn, err := grpc.DialContext(ctx, envConfig.Service, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return fmt.Errorf("dial: %w", err)
		}
		defer conn.Close()

		services, err := protoread.FetchServices(ctx, conn)
		if err != nil {
			return fmt.Errorf("fetch: %w", err)
		}

		// TODO: CORS
		// TODO: Logging
		// TODO: Metrics
		// TODO: Custom forwarding headers

		for _, ss := range services {
			name := string(ss.FullName())
			switch {
			case strings.HasSuffix(name, "Service"):
				if router == nil {
					return fmt.Errorf("service %s requires a public port", name)
				}
				if err := router.RegisterService(ss, conn); err != nil {
					return fmt.Errorf("register service %s: %w", name, err)
				}
				httpServices = append(httpServices, ss)
			case strings.HasSuffix(name, "Topic"):
				if worker == nil {
					return fmt.Errorf("topic %s requires an SQS URL", name)
				}
				if err := worker.RegisterService(ss, conn); err != nil {
					return fmt.Errorf("register worker %s: %w", name, err)
				}
			default:
				log.WithField(ctx, "service", name).Error("Unknown service type")
				// but continue
			}
		}
	}

	if router == nil && worker == nil && envConfig.PostgresOutboxURI == "" {
		return fmt.Errorf("no router and no worker. Nothing to do")
	}

	if router != nil {
		swaggerDocument, err := swagger.Build(router.CodecOptions, httpServices)
		if err != nil {
			return fmt.Errorf("building swagger: %w", err)
		}

		if err := router.StaticJSON("/swagger.json", swaggerDocument); err != nil {
			return err
		}

		srv := http.Server{
			Handler: router,
			Addr:    fmt.Sprintf(":%d", envConfig.PublicPort),
		}

		if jwksManager != nil {
			// Wait for keys to be loaded before starting the server
			if err := jwksManager.WaitForKeys(ctx); err != nil {
				return fmt.Errorf("failed to load JWKS: %w", err)
			}
		}

		go func() {
			<-ctx.Done()
			srv.Shutdown(ctx) // nolint:errcheck
		}()

		eg.Go(func() error {
			return srv.ListenAndServe()
		})
	}

	if worker != nil {
		eg.Go(func() error {
			return worker.Run(ctx)
		})
	}

	if envConfig.PostgresOutboxURI != "" {
		eg.Go(func() error {
			return outbox.Listen(ctx, envConfig.PostgresOutboxURI, snsSender)
		})
	}

	return eg.Wait()

}
