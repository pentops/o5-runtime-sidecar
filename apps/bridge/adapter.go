package bridge

import (
	"context"
	"fmt"
	"net"

	"github.com/pentops/log.go/log"
	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_tpb"
	"github.com/pentops/o5-runtime-sidecar/sidecar"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type BridgeConfig struct {
	// Port to expose locally to the running service(s). 0 disables
	AdapterAddr string `env:"ADAPTER_ADDR" default:""`
}

type App struct {
	addr      string
	server    *grpc.Server
	listening chan struct{}
}

func NewApp(bind string, sender Publisher, source sidecar.AppInfo) *App {
	messageBridge := NewMessageBridge(sender, source)
	server := grpc.NewServer()
	messaging_tpb.RegisterMessageBridgeTopicServer(server, messageBridge)
	reflection.Register(server)
	return &App{
		addr:   bind,
		server: server,
	}
}

func (gg *App) Run(ctx context.Context) error {
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

func (gg *App) Addr() string {
	<-gg.listening
	return gg.addr
}
