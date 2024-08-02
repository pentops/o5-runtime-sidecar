package entrypoint

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/pentops/j5/gen/j5/ext/v1/ext_j5pb"
	"github.com/pentops/j5/proxy"
	"github.com/pentops/log.go/log"
	"github.com/pentops/o5-auth/gen/o5/auth/v1/auth_pb"
	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_tpb"
	"github.com/pentops/o5-runtime-sidecar/adapter"
	"github.com/pentops/o5-runtime-sidecar/awsmsg"
	"github.com/pentops/o5-runtime-sidecar/outbox"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
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
	RegisterGRPCMethod(ctx context.Context, method proxy.GRPCMethodConfig) error
	ServeHTTP(http.ResponseWriter, *http.Request)
}

type routerServer struct {
	addr       string
	listening  chan struct{}
	router     proxyRouter
	globalAuth proxy.AuthHeaders

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

func (hs *routerServer) RegisterService(ctx context.Context, sd protoreflect.ServiceDescriptor, conn *grpc.ClientConn) error {

	defaultAuth := authNotSpecified
	if hs.globalAuth != nil {
		defaultAuth = authJwtBearer
	}

	serviceExt := proto.GetExtension(sd.Options(), ext_j5pb.E_Service).(*ext_j5pb.ServiceOptions)
	if serviceExt != nil {
		if serviceExt.DefaultAuth != nil {
			switch serviceExt.DefaultAuth.AuthMethod.(type) {
			case *ext_j5pb.AuthMethodOptions_JwtBearer:
				defaultAuth = authNone
			case *ext_j5pb.AuthMethodOptions_None:
				defaultAuth = authJwtBearer
			}
		}
	}

	methods := sd.Methods()
	for ii := 0; ii < methods.Len(); ii++ {
		method := methods.Get(ii)
		if err := hs.RegisterMethod(ctx, method, conn, defaultAuth); err != nil {
			return fmt.Errorf("failed to register grpc method: %w", err)
		}
	}

	return nil
}

type authMethod int

const (
	authNotSpecified authMethod = iota
	authNone
	authJwtBearer
)

func (hs *routerServer) RegisterMethod(ctx context.Context, md protoreflect.MethodDescriptor, conn *grpc.ClientConn, auth authMethod) error {
	methodConfig := proxy.GRPCMethodConfig{
		Method:  md,
		Invoker: conn,
	}

	methodOptions := md.Options().(*descriptorpb.MethodOptions)

	if j5Method := proto.GetExtension(methodOptions, ext_j5pb.E_Method).(*ext_j5pb.MethodOptions); j5Method != nil {
		switch j5Method.Auth.AuthMethod.(type) {
		case *ext_j5pb.AuthMethodOptions_None:
			auth = authNone // explicit auth method
		case *ext_j5pb.AuthMethodOptions_JwtBearer:
			auth = authJwtBearer // explicit auth method
		}
	} else if authOpt := proto.GetExtension(methodOptions, auth_pb.E_Auth).(*auth_pb.AuthMethodOptions); authOpt != nil {
		switch authOpt.AuthMethod.(type) {
		case *auth_pb.AuthMethodOptions_None:
			auth = authNone
		case *auth_pb.AuthMethodOptions_JwtBearer:
			auth = authJwtBearer
		}
	}

	switch auth {

	case authJwtBearer:
		if hs.globalAuth == nil {
			return fmt.Errorf("auth method specified as JWT, no JWKS configured")
		}
		methodConfig.AuthHeaders = hs.globalAuth

	case authNone:
		methodConfig.AuthHeaders = nil

	case authNotSpecified:
		if hs.globalAuth != nil {
			methodConfig.AuthHeaders = hs.globalAuth
		}

	default:
		return fmt.Errorf("auth method not supported (%d)", auth)
	}

	return hs.router.RegisterGRPCMethod(ctx, methodConfig)
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
