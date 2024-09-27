package entrypoint

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/pentops/jwtauth/jwks"
	"github.com/pentops/log.go/log"
	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_tpb"
	"github.com/pentops/o5-runtime-sidecar/adapter"
	"github.com/pentops/o5-runtime-sidecar/awsmsg"
	"github.com/pentops/o5-runtime-sidecar/jwtauth"
	"github.com/pentops/o5-runtime-sidecar/outbox"
	"github.com/pentops/o5-runtime-sidecar/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type adapterServer struct {
	addr      string
	server    *grpc.Server
	listening chan struct{}
}

func newAdapterServer(bind string, sender awsmsg.Publisher, source awsmsg.SourceConfig) *adapterServer {
	messageBridge := adapter.NewMessageBridge(sender, source)
	server := grpc.NewServer()
	messaging_tpb.RegisterMessageBridgeTopicServer(server, messageBridge)
	reflection.Register(server)
	return &adapterServer{
		addr:   bind,
		server: server,
	}
}

func (gg *adapterServer) Run(ctx context.Context) error {
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

func (gg *adapterServer) Addr() string {
	<-gg.listening
	return gg.addr
}

type proxyRouter interface {
	RegisterGRPCService(ctx context.Context, service protoreflect.ServiceDescriptor, invoker proxy.AppConn) error
	ServeHTTP(http.ResponseWriter, *http.Request)
	SetGlobalAuth(proxy.AuthHeaders)
}

type routerServer struct {
	addr      string
	listening chan struct{}
	router    proxyRouter

	waitFor []func(context.Context) error
}

func newRouterServer(addr string, router proxyRouter) *routerServer {
	return &routerServer{
		router:    router,
		addr:      addr,
		listening: make(chan struct{}),
	}
}

func (hs *routerServer) WaitFor(cb func(context.Context) error) {
	hs.waitFor = append(hs.waitFor, cb)
}

func (hs *routerServer) RegisterService(ctx context.Context, service protoreflect.ServiceDescriptor, invoker proxy.AppConn) error {
	return hs.router.RegisterGRPCService(ctx, service, invoker)
}

func (hs *routerServer) SetJWKS(jwks *jwks.JWKSManager) {
	globalAuth := proxy.AuthHeadersFunc(jwtauth.JWKSAuthFunc(jwks))
	hs.router.SetGlobalAuth(globalAuth)
}

func (hs *routerServer) Run(ctx context.Context) error {
	for _, ch := range hs.waitFor {
		if err := ch(ctx); err != nil {
			return err
		}
	}

	lis, err := net.Listen("tcp", hs.addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	srv := http.Server{
		Handler: hs.router,
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

func (hs *routerServer) Addr() string {
	<-hs.listening
	return hs.addr
}

type outboxListener struct {
	Name string
	*outbox.Listener
}

func newOutboxListener(name string, uri string, batcher outbox.Batcher, source awsmsg.SourceConfig) (*outboxListener, error) {
	ll, err := outbox.NewListener(uri, batcher, source)
	if err != nil {
		return nil, fmt.Errorf("failed to create outbox listener: %w", err)
	}

	return &outboxListener{
		Name:     name,
		Listener: ll,
	}, nil
}

func (ol *outboxListener) Run(ctx context.Context) error {
	return ol.Listener.Listen(ctx)
}
