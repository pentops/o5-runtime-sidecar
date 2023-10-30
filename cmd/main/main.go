package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/pentops/custom-proto-api/jsonapi"
	"github.com/pentops/custom-proto-api/swagger"
	"github.com/pentops/o5-runtime-sidecar/protoread"
	"github.com/pentops/o5-runtime-sidecar/proxy"
	"github.com/pentops/o5-runtime-sidecar/sqslink"
	"gopkg.daemonl.com/envconf"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"gopkg.daemonl.com/log"
)

var Version string

type EnvConfig struct {
	PublicPort  int    `env:"PUBLIC_PORT" default:"0"`
	Service     string `env:"SERVICE_ENDPOINT" default:""`
	StaticFiles string `env:"STATIC_FILES" default:""`
	SQSURL      string `env:"SQS_URL" default:""`
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

	var worker *sqslink.Worker
	if envConfig.SQSURL != "" {
		sqsClient := sqs.NewFromConfig(awsConfig)
		worker = sqslink.NewWorker(sqsClient, envConfig.SQSURL)
	}

	var router *proxy.Router
	if envConfig.PublicPort != 0 {
		router = proxy.NewRouter()
	}

	if envConfig.StaticFiles != "" {
		if router == nil {
			return fmt.Errorf("static files configured but no public port")
		}

		router.SetNotFoundHandler(http.FileServer(http.Dir(envConfig.StaticFiles)))
	}

	codecOptions := jsonapi.Options{
		ShortEnums: &jsonapi.ShortEnumsOption{
			UnspecifiedSuffix: "UNSPECIFIED",
			StrictUnmarshal:   true,
		},
		WrapOneof: true,
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
				if router == nil {
					return fmt.Errorf("service %s requires a public port", name)
				}
				if err := router.RegisterService(ss, conn); err != nil {
					return err
				}
			case strings.HasSuffix(name, "Topic"):
				if worker == nil {
					return fmt.Errorf("topic %s requires an SQS URL", name)
				}
				if err := worker.RegisterService(ss, conn); err != nil {
					return err
				}
			default:
				log.WithField(ctx, "service", name).Error("Unknown service type")
				// but continue
			}
		}
	}

	if router == nil && worker == nil {
		return fmt.Errorf("no router and no worker. Nothing to do")
	}

	eg, ctx := errgroup.WithContext(ctx)

	if router != nil {
		swaggerDocument, err := swagger.Build(codecOptions, allServices)
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

		go func() {
			<-ctx.Done()
			srv.Shutdown(ctx)
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

	return eg.Wait()

}
