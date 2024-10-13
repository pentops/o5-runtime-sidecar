package pgclient

import (
	"context"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/pentops/log.go/log"
)

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

func dialPGX(ctx context.Context, endpoint string) (*Frontend, error) {
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
