package entrypoint

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/pentops/jsonapi/proxy"
	"github.com/pentops/log.go/log"
	"github.com/pentops/o5-go/messaging/v1/messaging_tpb"
	"github.com/pentops/o5-runtime-sidecar/adapter"
	"github.com/pentops/o5-runtime-sidecar/outbox"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type adapterServer struct {
	addr      string
	server    *grpc.Server
	listening chan struct{}
}

func newAdapterServer(bind string, sender adapter.Sender) *adapterServer {
	messageBridge := adapter.NewMessageBridge(sender)
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

type routerServer struct {
	addr      string
	listening chan struct{}
	router    *proxy.Router

	waitFor []func(context.Context) error
}

func newRouterServer(addr string, router *proxy.Router) *routerServer {
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
	return hs.router.RegisterService(ctx, sd, conn)
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
	Name    string
	uri     string
	batcher outbox.Batcher
}

func newOutboxListener(name, uri string, batcher outbox.Batcher) *outboxListener {
	return &outboxListener{
		Name:    name,
		uri:     uri,
		batcher: batcher,
	}
}

func (ol *outboxListener) Run(ctx context.Context) error {
	return outbox.Listen(ctx, ol.uri, ol.batcher)
}
