package entrypoint

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	jsonapi_codec "github.com/pentops/jsonapi/codec"
	"github.com/pentops/jsonapi/gen/j5/source/v1/source_j5pb"
	"github.com/pentops/jsonapi/proxy"
	"github.com/pentops/jwtauth/jwks"
	"github.com/pentops/log.go/log"
	"github.com/pentops/o5-go/messaging/v1/messaging_tpb"
	"github.com/pentops/o5-runtime-sidecar/adapter"
	"github.com/pentops/o5-runtime-sidecar/jwtauth"
	"github.com/pentops/o5-runtime-sidecar/outbox"
	"github.com/pentops/o5-runtime-sidecar/protoread"
	"github.com/pentops/o5-runtime-sidecar/sqslink"
	"github.com/pentops/runner"
	"github.com/rs/cors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type Config struct {
	// Port to expose to the external LB. 0 disables
	PublicAddr string `env:"PUBLIC_ADDR" default:""`

	// Port to expose locally to the running service(s). 0 disables
	AdapterAddr string `env:"ADAPTER_ADDR" default:""`

	Service     []string `env:"SERVICE_ENDPOINT" default:""`
	StaticFiles string   `env:"STATIC_FILES" default:""`
	SQSURL      string   `env:"SQS_URL" default:""`

	PostgresOutboxURI string `env:"POSTGRES_OUTBOX" default:""`
	SNSPrefix         string `env:"SNS_PREFIX" default:""`

	CORSOrigins []string `env:"CORS_ORIGINS" default:""`

	JWKS []string `env:"JWKS" default:""`
}

func FromConfig(envConfig Config, awsConfig AWSProvider) (*Runtime, error) {

	rt := NewRuntime()

	if envConfig.PostgresOutboxURI != "" || envConfig.SQSURL != "" {
		if envConfig.SNSPrefix == "" {
			return nil, fmt.Errorf("SNS prefix required when using Postgres outbox or subscribing to SQS")
		}
		rt.Sender = outbox.NewSNSBatcher(awsConfig.SNS(), envConfig.SNSPrefix)
	}

	if envConfig.PostgresOutboxURI != "" {
		if err := rt.AddOutbox(envConfig.PostgresOutboxURI); err != nil {
			return nil, fmt.Errorf("add outbox: %w", err)
		}
	}

	if envConfig.AdapterAddr != "" {
		if err := rt.AddAdapter(envConfig.AdapterAddr); err != nil {
			return nil, fmt.Errorf("add adapter: %w", err)
		}
	}

	if envConfig.SQSURL != "" {
		rt.Worker = sqslink.NewWorker(awsConfig.SQS(), envConfig.SQSURL, rt.Sender)
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

		if len(envConfig.CORSOrigins) > 0 {
			router.Use(cors.New(cors.Options{
				AllowedOrigins:   envConfig.CORSOrigins,
				AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE"},
				AllowedHeaders:   []string{"*"},
				AllowCredentials: true,
			}).Handler)
		}

		if envConfig.StaticFiles != "" {
			router.SetNotFoundHandler(http.FileServer(http.Dir(envConfig.StaticFiles)))
		}

		if err := rt.AddRouter(envConfig.PublicAddr, router); err != nil {
			return nil, fmt.Errorf("add router: %w", err)
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

type GRPCServer struct {
	addr      string
	server    *grpc.Server
	listening chan struct{}
}

func (gg *GRPCServer) Run(ctx context.Context) error {
	lis, err := net.Listen("tcp", gg.addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	gg.addr = lis.Addr().String()
	close(gg.listening)

	log.WithField(ctx, "addr", gg.addr).Info("Listening")

	go func() {
		<-ctx.Done()
		gg.server.GracefulStop()
	}()

	return gg.server.Serve(lis)
}

func (gg *GRPCServer) Addr() string {
	<-gg.listening
	return gg.addr
}

type HTTPServer struct {
	addr      string
	listening chan struct{}
	handler   http.Handler
}

func NewHTTPServer(addr string, handler http.Handler) *HTTPServer {
	return &HTTPServer{
		handler:   handler,
		addr:      addr,
		listening: make(chan struct{}),
	}
}

func (hs *HTTPServer) Run(ctx context.Context) error {

	lis, err := net.Listen("tcp", hs.addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	srv := http.Server{
		Handler: hs.handler,
		Addr:    hs.addr,
	}

	hs.addr = lis.Addr().String()
	close(hs.listening)

	go func() {
		<-ctx.Done()
		if err := srv.Shutdown(ctx); err != nil {
			log.WithError(ctx, err).Error("Error shutting down server")
		}
	}()

	return srv.Serve(lis)
}

func (hs *HTTPServer) Addr() string {
	<-hs.listening
	return hs.addr
}

type Runtime struct {
	router *proxy.Router
	Worker *sqslink.Worker
	Sender *outbox.SNSBatcher
	JWKS   *jwks.JWKSManager

	Adapter      *GRPCServer
	RouterServer *HTTPServer

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

		if rt.JWKS != nil {
			runGroup.Add("jwks", rt.JWKS.Run)
			// Wait for keys to be loaded before starting the server
		}

		runGroup.Add("router", func(ctx context.Context) error {
			if rt.JWKS != nil {
				if err := rt.JWKS.WaitForKeys(ctx); err != nil {
					return fmt.Errorf("failed to load JWKS: %w", err)
				}
			}
			return rt.RouterServer.Run(ctx)
		})
	}

	if rt.Worker != nil {
		didAnything = true
		runGroup.Add("worker", rt.Worker.Run)
	}

	if rt.Adapter != nil {
		didAnything = true
		runGroup.Add("adapter", rt.Adapter.Run)
	}

	if !didAnything {
		return NothingToDoError
	}

	if err := runGroup.Run(ctx); err != nil {
		log.WithError(ctx, err).Error("Error in goroutines")
		return err
	}

	log.Info(ctx, "Sidecar Stopped with no error")
	return nil

}

var NothingToDoError = errors.New("no services configured")

func (rt *Runtime) AddRouter(bind string, router *proxy.Router) error {
	if rt.router != nil {
		return fmt.Errorf("router already configured")
	}

	rt.router = router
	rt.RouterServer = NewHTTPServer(bind, router)
	return nil
}

func (rt *Runtime) AddAdapter(bind string) error {
	if rt.Sender == nil {
		return fmt.Errorf("adapter requires a sender")
	}

	messageBridge := adapter.NewMessageBridge(rt.Sender)
	server := grpc.NewServer()
	messaging_tpb.RegisterMessageBridgeTopicServer(server, messageBridge)
	reflection.Register(server)

	rt.Adapter = &GRPCServer{
		addr:   bind,
		server: server,
	}

	return nil
}

func (rt *Runtime) AddOutbox(outboxURI string) error {
	if rt.Sender == nil {
		return fmt.Errorf("outbox requires a sender")
	}

	rt.outboxURIs = append(rt.outboxURIs, outboxURI)
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
