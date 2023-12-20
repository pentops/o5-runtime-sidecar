package entrypoint

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/pentops/jsonapi/jsonapi"
	"github.com/pentops/jwtauth/jwks"
	"github.com/pentops/log.go/log"
	"github.com/pentops/o5-runtime-sidecar/jwtauth"
	"github.com/pentops/o5-runtime-sidecar/outbox"
	"github.com/pentops/o5-runtime-sidecar/protoread"
	"github.com/pentops/o5-runtime-sidecar/proxy"
	"github.com/pentops/o5-runtime-sidecar/sqslink"
	"github.com/pentops/runner"
	"github.com/rs/cors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type Config struct {
	PublicPort  int      `env:"PUBLIC_PORT" default:"0"`
	Service     []string `env:"SERVICE_ENDPOINT" default:""`
	StaticFiles string   `env:"STATIC_FILES" default:""`
	SQSURL      string   `env:"SQS_URL" default:""`

	PostgresOutboxURI string `env:"POSTGRES_OUTBOX" default:""`
	SNSPrefix         string `env:"SNS_PREFIX" default:""`

	CORSOrigins []string `env:"CORS_ORIGINS" default:""`

	JWKS []string `env:"JWKS" default:""`
}

func FromConfig(envConfig Config, awsConfig aws.Config) (*Runtime, error) {

	rt := NewRuntime()

	if envConfig.PostgresOutboxURI != "" || envConfig.SQSURL != "" {
		if envConfig.SNSPrefix == "" {
			return nil, fmt.Errorf("SNS prefix required when using Postgres outbox or subscribing to SQS")
		}
		rt.Sender = outbox.NewSNSBatcher(sns.NewFromConfig(awsConfig), envConfig.SNSPrefix)
	}

	if envConfig.PostgresOutboxURI != "" {
		if err := rt.AddOutbox(envConfig.PostgresOutboxURI); err != nil {
			return nil, fmt.Errorf("add outbox: %w", err)
		}
	}

	if envConfig.SQSURL != "" {
		sqsClient := sqs.NewFromConfig(awsConfig)
		rt.Worker = sqslink.NewWorker(sqsClient, envConfig.SQSURL, rt.Sender)
	}

	if envConfig.PublicPort != 0 {
		codecOptions := jsonapi.Options{
			ShortEnums: &jsonapi.ShortEnumsOption{
				UnspecifiedSuffix: "UNSPECIFIED",
				StrictUnmarshal:   true,
			},
			WrapOneof: true,
		}

		router := proxy.NewRouter(codecOptions)
		if len(envConfig.CORSOrigins) > 0 {
			router.UseCORS(cors.Options{
				AllowedOrigins:   envConfig.CORSOrigins,
				AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE"},
				AllowedHeaders:   []string{"*"},
				AllowCredentials: true,
			})
		}

		if err := rt.AddRouter(envConfig.PublicPort, router); err != nil {
			return nil, fmt.Errorf("add router: %w", err)
		}
	}

	if envConfig.StaticFiles != "" {
		if err := rt.StaticFiles(envConfig.StaticFiles); err != nil {
			return nil, fmt.Errorf("static files: %w", err)
		}
	}

	if len(envConfig.JWKS) > 0 {
		if err := rt.AddJWKS(envConfig.JWKS...); err != nil {
			return nil, fmt.Errorf("add JWKS: %w", err)
		}
	}

	for _, endpoint := range envConfig.Service {
		if err := rt.AddEndpoint(endpoint); err != nil {
			return nil, fmt.Errorf("add endpoint %s: %w", endpoint, err)
		}
	}

	return rt, nil
}

type Runtime struct {
	router     *proxy.Router
	Worker     *sqslink.Worker
	Sender     *outbox.SNSBatcher
	JWKS       *jwks.JWKSManager
	PublicPort int

	httpServices []protoreflect.ServiceDescriptor

	outboxURIs []string

	connections []io.Closer
	endpoints   []string
}

func NewRuntime() *Runtime {
	return &Runtime{
		httpServices: make([]protoreflect.ServiceDescriptor, 0),
	}
}

func (rt *Runtime) Close() error {
	for _, conn := range rt.connections {
		if err := conn.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (rt *Runtime) Run(ctx context.Context) error {
	log.Debug(ctx, "Sidecar Running")
	defer rt.Close()

	runGroup := runner.NewGroup(
		runner.WithName("runtime"),
		runner.WithCancelOnSignals(),
	)

	didAnything := false

	for _, uri := range rt.outboxURIs {
		didAnything = true
		uri := uri
		runGroup.Add("outbox", func(ctx context.Context) error {
			return outbox.Listen(ctx, uri, rt.Sender)
		})
	}

	if rt.router != nil && rt.JWKS != nil {
		rt.router.AuthFunc = jwtauth.JWKSAuthFunc(rt.JWKS)
	}

	for _, endpoint := range rt.endpoints {
		endpoint := endpoint
		if err := rt.registerEndpoint(ctx, endpoint); err != nil {
			return fmt.Errorf("register endpoint %s: %w", endpoint, err)
		}
	}

	if rt.router != nil {
		// TODO: CORS
		// TODO: Metrics

		didAnything = true

		srv := http.Server{
			Handler: rt.router,
			Addr:    fmt.Sprintf(":%d", rt.PublicPort),
		}

		if rt.JWKS != nil {
			runGroup.Add("jwks", rt.JWKS.Run)
			// Wait for keys to be loaded before starting the server
		}

		go func() {
			<-ctx.Done()
			if err := srv.Shutdown(ctx); err != nil {
				log.WithError(ctx, err).Error("Error shutting down server")
			}
		}()

		runGroup.Add("router", func(ctx context.Context) error {
			if rt.JWKS != nil {
				if err := rt.JWKS.WaitForKeys(ctx); err != nil {
					return fmt.Errorf("failed to load JWKS: %w", err)
				}
			}
			return srv.ListenAndServe()
		})
	}

	if rt.Worker != nil {
		didAnything = true
		runGroup.Add("worker", func(ctx context.Context) error {
			return rt.Worker.Run(ctx)
		})
	}

	if !didAnything {
		return fmt.Errorf("no services configured")
	}

	if err := runGroup.Run(ctx); err != nil {
		log.WithError(ctx, err).Error("Error in goroutines")
		return err
	}

	log.Info(ctx, "Sidecar Stopped with no error")
	return nil

}

func (rt *Runtime) AddRouter(port int, router *proxy.Router) error {
	if rt.router != nil {
		return fmt.Errorf("router already configured")
	}

	rt.router = router
	rt.PublicPort = port
	return nil
}

func (rt *Runtime) AddOutbox(outboxURI string) error {
	if rt.Sender == nil {
		return fmt.Errorf("outbox requires a sender")
	}

	rt.outboxURIs = append(rt.outboxURIs, outboxURI)
	return nil
}

func (rt *Runtime) StaticFiles(dirname string) error {
	if rt.router == nil {
		return fmt.Errorf("static files configured but no router")
	}

	rt.router.SetNotFoundHandler(http.FileServer(http.Dir(dirname)))
	return nil
}

func (rt *Runtime) AddJWKS(sources ...string) error {
	jwksManager := jwks.NewKeyManager()

	if err := jwksManager.AddSourceURLs(sources...); err != nil {
		return err
	}

	rt.JWKS = jwksManager
	return nil
}

func (rt *Runtime) AddEndpoint(endpoint string) error {
	rt.endpoints = append(rt.endpoints, endpoint)
	return nil
}

func (rt *Runtime) registerEndpoint(ctx context.Context, endpoint string) error {

	conn, err := grpc.DialContext(ctx, endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	rt.connections = append(rt.connections, conn)

	services, err := protoread.FetchServices(ctx, conn)
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}

	for _, ss := range services {
		name := string(ss.FullName())
		switch {
		case strings.HasSuffix(name, "Service"), strings.HasSuffix(name, "Sandbox"):
			if rt.router == nil {
				return fmt.Errorf("service %s requires a public port", name)
			}
			if err := rt.router.RegisterService(ctx, ss, conn); err != nil {
				return fmt.Errorf("register service %s: %w", name, err)
			}
			rt.httpServices = append(rt.httpServices, ss)
		case strings.HasSuffix(name, "Topic"):
			if rt.Worker == nil {
				return fmt.Errorf("topic %s requires an SQS URL", name)
			}
			if err := rt.Worker.RegisterService(ctx, ss, conn); err != nil {
				return fmt.Errorf("register worker %s: %w", name, err)
			}
		default:
			log.WithField(ctx, "service", name).Error("Unknown service type")
			// but continue
		}
	}

	return nil
}