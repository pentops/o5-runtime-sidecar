package pgproxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/pentops/log.go/log"
	"github.com/pentops/o5-runtime-sidecar/adapters/pgclient"
	"github.com/pentops/o5-runtime-sidecar/apps/pgproxy/pgserver"
)

type Listener struct {
	dbs     map[string]pgclient.PGConnector
	bind    string
	network string
}

func NewListener(network, bind string, dbs map[string]pgclient.PGConnector) (*Listener, error) {
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

func (ll *Listener) Listen(ctx context.Context) error {
	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, ll.network, ll.bind)
	if err != nil {
		return err
	}

	log.WithField(ctx, "bind", ll.bind).Info("pgproxy: Ready")

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

		go ll.newConn(ctx, conn)
	}
}

func (ll *Listener) newConn(ctx context.Context, clientConn net.Conn) {
	defer clientConn.Close()
	ctx = log.WithField(ctx, "clientAddr", clientConn.RemoteAddr().String())

	client, err := pgserver.NewBackend(ctx, clientConn)
	if err != nil {
		log.WithError(ctx, err).Error("pg proxy handshake failure")
	}

	log.WithField(ctx, "data", client.Data).Info("client connected")
	defer client.Close()

	serverConfig, ok := ll.dbs[client.Data.Database]
	if !ok {
		client.Fatalf(ctx, "database %s not found", client.Data.Database)
		return
	}

	dsn, err := serverConfig.DSN(ctx)
	if err != nil {
		client.Fatal(ctx, "failed to get DSN")
		log.WithError(ctx, err).Error("pg proxy: error getting DSN")
		return
	}

	server, err := dialFrontend(ctx, dsn)
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

const TxStatusIdle = 'I'

type Frontend struct {
	frontend *pgproto3.Frontend
	conn     io.Closer
}

func (f *Frontend) Close() {
	f.conn.Close()
}

func (f *Frontend) Frontend() *pgproto3.Frontend {
	return f.frontend
}

func dialFrontend(ctx context.Context, endpoint string) (*Frontend, error) {
	// Use PGX to connect and log in
	cfg, err := pgconn.ParseConfig(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parsing backend config: %w", err)
	}
	ctx = log.WithField(ctx, "host", cfg.Host)

	log.Debug(ctx, "connecting to server")

	conn, err := pgconn.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", cfg.Host, err)
	}

	log.Debug(ctx, "connected to server")

	// Exit the PGX wrapper, we only needed it for auth.

	err = conn.SyncConn(ctx)
	if err != nil {
		return nil, fmt.Errorf("syncing connection: %w", err)
	}

	hj, err := conn.Hijack()
	if err != nil {
		return nil, fmt.Errorf("hijacking connection: %w", err)
	}

	if hj.TxStatus != TxStatusIdle {
		return nil, fmt.Errorf("expected tx status 'I' (idle), got %c", hj.TxStatus)
	}

	log.Debug(ctx, "hijacked connection")
	return &Frontend{
		conn:     hj.Conn,
		frontend: hj.Frontend,
	}, nil
}
