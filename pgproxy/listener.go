package pgproxy

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/pentops/log.go/log"
)

type Listener struct {
	dbs     map[string]PGConnector
	bind    string
	network string
}

func NewListener(network, bind string, dbs map[string]PGConnector) (*Listener, error) {
	return &Listener{
		dbs:     dbs,
		network: network,
		bind:    bind,
	}, nil
}

func (l *Listener) Listen(ctx context.Context) error {
	ln, err := net.Listen(l.network, l.bind)
	if err != nil {
		return err
	}

	log.WithField(ctx, "bind", l.bind).Info("pgproxy: Ready")

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go l.newConn(ctx, conn)
	}
}

func (ln *Listener) newConn(ctx context.Context, clientConn net.Conn) {
	defer clientConn.Close()
	ctx = log.WithField(ctx, "clientAddr", clientConn.RemoteAddr().String())
	client, err := backendForClient(ctx, clientConn)
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

	err = passthrough(ctx, client.backend, server.frontend)
	if err != nil {
		log.WithError(ctx, err).Error("pgproxy: error in passthrough")
	}

}

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

func (b *Backend) Close() error {
	return b.conn.Close()
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

func (b *Backend) Fatal(ctx context.Context, msg string) {
	log.WithField(ctx, "error", msg).Warn("sending fatal error to client")
	b.backend.Send(&pgproto3.ErrorResponse{
		Severity: "FATAL",
		Message:  msg,
	})
	err := b.backend.Flush()
	if err != nil {
		log.WithError(ctx, err).Error("failed to send fatal message to client")
	}
}

func (b *Backend) SendReady() error {
	b.backend.Send(&pgproto3.AuthenticationOk{})
	b.backend.Send(&pgproto3.ReadyForQuery{TxStatus: TxStatusIdle})
	err := b.backend.Flush()
	if err != nil {
		return fmt.Errorf("failed to send ready for query: %w", err)
	}
	return nil
}

func backendForClient(ctx context.Context, clientConn io.ReadWriteCloser) (*Backend, error) {

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

	ctx = log.WithFields(ctx, map[string]interface{}{
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
