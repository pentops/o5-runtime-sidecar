package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/pentops/o5-runtime-sidecar/protoread"
	"github.com/pentops/o5-runtime-sidecar/proxy"
	"github.com/pentops/o5-runtime-sidecar/swagger"
	"gopkg.daemonl.com/envconf"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"gopkg.daemonl.com/log"
)

var Version string

type EnvConfig struct {
	PublicPort  int    `env:"PUBLIC_PORT" default:"8080"`
	Service     string `env:"SERVICE_ENDPOINT" default:""`
	StaticFiles string `env:"STATIC_FILES" default:""`
}

func main() {

	ctx := context.Background()
	ctx = log.WithFields(ctx, map[string]interface{}{
		"application": "userauth",
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

	sqsClient := sqs.NewFromConfig(awsConfig)

	router := proxy.NewRouter()

	if envConfig.StaticFiles != "" {
		router.SetNotFoundHandler(http.FileServer(http.Dir(envConfig.StaticFiles)))
	}

	allServices := make([]protoreflect.ServiceDescriptor, 0)

	endpoints := strings.Split(envConfig.Service, ",")
	for _, endpoint := range endpoints {
		endpoint = strings.TrimSpace(endpoint)
		if endpoint == "" {
			continue
		}

		conn, err := grpc.DialContext(ctx, envConfig.Service, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: backoff.Config{
				BaseDelay:  1 * time.Second,
				Multiplier: 1.6,
				MaxDelay:   120 * time.Second,
				Jitter:     0.2,
			},
			MinConnectTimeout: 20 * time.Second,
		}))
		if err != nil {
			return fmt.Errorf("dial: %w", err)
		}
		defer conn.Close()

		services, err := protoread.FetchServices(ctx, conn)
		if err != nil {
			return fmt.Errorf("fetch: %w", err)
		}

		// TODO: CORS
		// TODO: Auth
		// TODO: Logging
		// TODO: Metrics
		// TODO: Custom forwarding headers

		for _, ss := range services {
			allServices = append(allServices, ss)
			name := string(ss.FullName())
			switch {
			case strings.HasSuffix(name, "Service"):
				if err := router.RegisterService(ss, conn); err != nil {
					return err
				}
			case strings.HasSuffix(name, "Topic"):
				if err := registerTopic(ss, conn, sqsClient); err != nil {
					return err
				}
			default:
				log.WithField(ctx, "service", name).Error("Unknown service type")
				// but continue
			}
		}
	}

	swaggerDocument, err := swagger.Build(allServices)
	if err != nil {
		return err
	}

	if err := router.StaticJSON("/swagger.json", swaggerDocument); err != nil {
		return err
	}

	srv := http.Server{
		Handler: router,
		Addr:    fmt.Sprintf(":%d", envConfig.PublicPort),
	}

	return srv.ListenAndServe()
}

type SQSAPI interface {
}

func registerTopic(ss protoreflect.ServiceDescriptor, conn proxy.Invoker, sqsClient SQSAPI) error {
	return nil
}
