package pgproxy

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/pentops/log.go/log"
	"github.com/pentops/o5-runtime-sidecar/adapters/postgres"
	"github.com/pentops/o5-runtime-sidecar/adapters/postgres/pgserver"
)

type Listener struct {
	dbs     map[string]postgres.PGConnector
	bind    string
	network string
}

func NewListener(network, bind string, dbs map[string]postgres.PGConnector) (*Listener, error) {

	if network == "unix" {
		if err := os.MkdirAll(filepath.Dir(bind), 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory for unix socket: %w", err)
		}
		if err := os.Remove(bind); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to remove existing unix socket: %w", err)
		}
	}
	return &Listener{
		dbs:     dbs,
		network: network,
		bind:    bind,
	}, nil
}

func (l *Listener) Listen(ctx context.Context) error {
	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, l.network, l.bind)
	if err != nil {
		return err
	}

	log.WithField(ctx, "bind", l.bind).Info("pgproxy: Ready")

	go func() {
		<-ctx.Done()
		ln.Close()
		log.Debug(ctx, "pgproxy: context done, listener closed")
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go l.newConn(ctx, conn)
	}
}

func (ln *Listener) newConn(ctx context.Context, clientConn net.Conn) {
	defer clientConn.Close()
	ctx = log.WithField(ctx, "clientAddr", clientConn.RemoteAddr().String())
	client, err := pgserver.NewBackend(ctx, clientConn)
	if err != nil {
		log.WithError(ctx, err).Error("pg proxy handshake failure")
	}
	log.WithField(ctx, "data", client.Data).Info("client connected")
	defer client.Close()

	serverConfig, ok := ln.dbs[client.Data.Database]
	if !ok {
		client.Fatalf(ctx, "database %s not found", client.Data.Database)
		return
	}

	server, err := serverConfig.ConnectToServer(ctx)
	if err != nil {
		client.Fatal(ctx, "failed to connect to server")
		log.WithError(ctx, err).Error("pg proxy: error connecting to server")
		return
	}
	defer server.Close()

	err = client.SendReady()
	if err != nil {
		log.WithError(ctx, err).Error("failed to send ready message")
		server.Close()
		return
	}

	err = client.Passthrough(ctx, server.Frontend())
	if err != nil {
		log.WithError(ctx, err).Error("pgproxy: error in passthrough")
	}

}
