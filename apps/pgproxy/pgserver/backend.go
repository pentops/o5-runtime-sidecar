package pgserver

import (
	"context"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/pentops/log.go/log"
)

type StartupData struct {
	User            string
	Database        string
	ProtocolVersion uint32
}

type Backend struct {
	Data    StartupData
	conn    io.ReadWriteCloser
	backend *pgproto3.Backend
}

func NewBackend(ctx context.Context, clientConn io.ReadWriteCloser) (*Backend, error) {
	// Client == Backend the PG Protocol implements what a server implements.
	client := pgproto3.NewBackend(clientConn, clientConn)
	be := &Backend{
		backend: client,
		conn:    clientConn,
	}

	err := be.clientHandshake(ctx)
	if err != nil {
		return nil, fmt.Errorf("client handshake: %w", err)
	}

	ctx = log.WithFields(ctx, map[string]any{
		"user":     be.Data.User,
		"database": be.Data.Database,
	})

	if be.Data.ProtocolVersion != pgproto3.ProtocolVersionNumber {
		// TODO: The pgproto version is pre-set, we need to negotiate with the
		// client if required, or take over the whole auth flow with the server
		if be.Data.ProtocolVersion < pgproto3.ProtocolVersionNumber {
			log.WithField(ctx, "version", be.Data.ProtocolVersion).Warn("protocol version mismatch")
		} else {
			log.WithField(ctx, "version", be.Data.ProtocolVersion).Error("protocol version not supported")
			return nil, fmt.Errorf("protocol version not supported")
		}
	}

	return be, nil
}

func (be *Backend) Close() error {
	return be.conn.Close()
}

func (be *Backend) Passthrough(ctx context.Context, frontend *pgproto3.Frontend) error {
	return passthrough(ctx, be.backend, frontend)
}

func (be *Backend) fatalErr(ctx context.Context, msg string, args ...any) error {
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}

	be.Fatal(ctx, msg)

	return fmt.Errorf("fatal client error: %s", msg)
}

func (be *Backend) Fatalf(ctx context.Context, msg string, args ...any) {
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}

	be.Fatal(ctx, msg)
}

func (be *Backend) Fatal(ctx context.Context, msg string) {
	log.WithField(ctx, "error", msg).Warn("sending fatal error to client")
	be.backend.Send(&pgproto3.ErrorResponse{
		Severity: "FATAL",
		Message:  msg,
	})

	err := be.backend.Flush()
	if err != nil {
		log.WithError(ctx, err).Error("failed to send fatal message to client")
	}
}

const TxStatusIdle = 'I'

func (be *Backend) SendReady() error {
	be.backend.Send(&pgproto3.AuthenticationOk{})
	be.backend.Send(&pgproto3.ReadyForQuery{TxStatus: TxStatusIdle})

	err := be.backend.Flush()
	if err != nil {
		return fmt.Errorf("failed to send ready for query: %w", err)
	}

	return nil
}

func (be *Backend) clientHandshake(ctx context.Context) error {
	msg, err := be.backend.ReceiveStartupMessage()
	if err != nil {
		return fmt.Errorf("failed to receive startup message: %w", err)
	}

	switch mt := msg.(type) {
	case *pgproto3.StartupMessage:
		user, ok := mt.Parameters["user"]
		if !ok {
			return be.fatalErr(ctx, "no user in startup message")
		}

		db, ok := mt.Parameters["database"]
		if !ok {
			db = user
		}

		if _, ok := mt.Parameters["replication"]; ok {
			return be.fatalErr(ctx, "replication is not supported")
		}

		if _, ok := mt.Parameters["options"]; ok {
			return be.fatalErr(ctx, "options is not supported")
		}

		be.Data = StartupData{
			User:            user,
			Database:        db,
			ProtocolVersion: mt.ProtocolVersion,
		}

		return nil

	case *pgproto3.SSLRequest:
		return be.fatalErr(ctx, "ssl connections are not supported")

	case *pgproto3.CancelRequest:
		return fmt.Errorf("cancel request received")

	default:
		return be.fatalErr(ctx, "unknown message type: %T", mt)
	}
}
