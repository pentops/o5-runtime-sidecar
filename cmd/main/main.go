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
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/pentops/o5-runtime-sidecar/protoread"
	"github.com/pentops/o5-runtime-sidecar/proxy"
	"gopkg.daemonl.com/envconf"

	"gopkg.daemonl.com/log"
)

var Version string

var config = struct {
	PublicPort  int    `env:"PUBLIC_PORT" default:"8080"`
	Service     string `env:"SERVICE_ENDPOINT"`
	StaticFiles string `env:"STATIC_FILES" default:""`
}{}

func main() {

	ctx := context.Background()
	ctx = log.WithFields(ctx, map[string]interface{}{
		"application": "userauth",
		"version":     Version,
	})

	if err := envconf.Parse(&config); err != nil {
		log.WithError(ctx, err).Error("Config Failure")
		os.Exit(1)
	}

	if err := run(ctx); err != nil {
		log.WithError(ctx, err).Error("Failed to serve")
		os.Exit(1)
	}
}

func run(ctx context.Context) error {

	// TODO: Register a real one?
	var s3Client s3iface.S3API

	conn, err := grpc.DialContext(ctx, config.Service, grpc.WithInsecure(), grpc.WithConnectParams(grpc.ConnectParams{
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

	router := proxy.NewRouter()

	if config.StaticFiles != "" {
		router.SetNotFoundHandler(http.FileServer(http.Dir(config.StaticFiles)))
	}
	// TODO: CORS
	// TODO: Auth
	// TODO: Logging
	// TODO: Metrics
	// TODO: Custom forwarding headers

	for _, ss := range services {
		name := string(ss.FullName())
		switch {
		case strings.HasSuffix(name, "Service"):
			if err := router.RegisterService(ss, conn); err != nil {
				return err
			}
		case strings.HasSuffix(name, "Topic"):
			if err := registerTopic(ss, conn, s3Client); err != nil {
				return err
			}
		default:
			log.WithField(ctx, "service", name).Error("Unknown service type")
			// but continue
		}
	}

	srv := http.Server{
		Handler: router,
		Addr:    fmt.Sprintf(":%d", config.PublicPort),
	}

	return srv.ListenAndServe()
}

func registerTopic(ss protoreflect.ServiceDescriptor, conn proxy.Invoker, s3Client s3iface.S3API) error {
	return nil
}
