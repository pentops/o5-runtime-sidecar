package pgproxy

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/pentops/log.go/log"
)

type Pipe struct {
	connector PGConnector

	// LibPQ doesn't pass through a context on dial. We are emulating a server
	// anyway, so pretend this is the context of a 'Listen()' call.
	ctx context.Context
}

func NewPipe(ctx context.Context, connector PGConnector) (*Pipe, error) {
	if connector == nil {
		return nil, fmt.Errorf("connector is nil")
	}
	return &Pipe{
		connector: connector,
		ctx:       ctx,
	}, nil
}

func (pp *Pipe) runPipe() (net.Conn, error) {
	toServer, toClient := net.Pipe()

	go pp.runClient(toClient)

	return toServer, nil
}

func (cc *Pipe) Dial(network, address string) (net.Conn, error) {
	return cc.runPipe()
}

func (cc *Pipe) DialTimeout(network, address string, timeout time.Duration) (net.Conn, error) {
	return cc.runPipe()
}

func (pp *Pipe) runClient(toClient net.Conn) {
	ctx := pp.ctx
	log.Debug(ctx, "runPipe")
	client, err := backendForClient(ctx, toClient)
	if err != nil {
		log.WithError(ctx, err).Error("pgproxy: error creating backend")
		return
	}
	log.WithField(ctx, "data", client.Data).Info("client connected")
	defer client.Close()

	server, err := pp.connector.ConnectToServer(ctx)
	if err != nil {
		client.Fatal(ctx, "failed to connect to server")
		log.WithError(ctx, err).Error("pgproxy: error connecting to server")
		return
	}
	defer server.Close()

	err = client.SendReady()
	if err != nil {
		log.WithError(ctx, err).Error("failed to send ready message")
		server.Close()
		return
	}

	err = passthrough(ctx, client.backend, server.frontend)
	if err != nil {
		log.WithError(ctx, err).Error("pgproxy: error in passthrough")
	}
}
