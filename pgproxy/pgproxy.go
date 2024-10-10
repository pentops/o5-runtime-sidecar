package pgproxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/pentops/log.go/log"
	"golang.org/x/sync/errgroup"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgproto3"
)

type AuthClient interface {
	NewConnectionString(ctx context.Context, dbName, userName string) (string, error)
}

type Connector struct {
	authClient AuthClient
}

func NewConnector(authClient AuthClient) (*Connector, error) {
	return &Connector{
		authClient: authClient,
	}, nil
}

func (cc *Connector) runPipe() (net.Conn, error) {
	fmt.Println("runPipe")
	a, b := net.Pipe()
	ctx := context.Background()
	go func() {
		err := cc.RunConn(ctx, b)
		if err != nil {
			log.WithError(ctx, err).Error("pgproxy: error running proxy")
		}
		fmt.Println("runPipe done")
	}()
	return a, nil
}

func (cc *Connector) Dial(network, address string) (net.Conn, error) {
	return cc.runPipe()
}

func (cc *Connector) DialTimeout(network, address string, timeout time.Duration) (net.Conn, error) {
	return cc.runPipe()
}

func (ln *Connector) RunConn(ctx context.Context, clientConn io.ReadWriter) error {

	// Client == Backend the PG Protocol implements what a server implements.
	client := pgproto3.NewBackend(clientConn, clientConn)
	beStartupData, err := clientHandshake(ctx, client)
	if err != nil {
		return fmt.Errorf("client handshake: %w", err)
	}
	ctx = log.WithFields(ctx, map[string]interface{}{
		"user":     beStartupData.User,
		"database": beStartupData.Database,
	})
	if beStartupData.ProtocolVersion != pgproto3.ProtocolVersionNumber {
		// TODO: The pgproto version is pre-set, we need to negotiate with the
		// client if required, or take over the whole auth flow with the server
		if beStartupData.ProtocolVersion < pgproto3.ProtocolVersionNumber {
			log.WithField(ctx, "version", beStartupData.ProtocolVersion).Warn("protocol version mismatch")
		} else {
			log.WithField(ctx, "version", beStartupData.ProtocolVersion).Error("protocol version not supported")
			return fmt.Errorf("protocol version not supported")
		}

	}
	log.WithField(ctx, "data", beStartupData).Info("client connected")

	server, err := ln.connectToServer(ctx, *beStartupData)
	if err != nil {
		log.WithError(ctx, err).Error("failed to connect to server")
		_ = beFatal(ctx, client, "failed to connect to server")
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer server.Close()

	log.Debug(ctx, "server connected")

	client.Send(&pgproto3.AuthenticationOk{})
	client.Send(&pgproto3.ReadyForQuery{TxStatus: TxStatusIdle})
	err = client.Flush()
	if err != nil {
		return fmt.Errorf("failed to send ready for query: %w", err)
	}

	err = passthrough(ctx, client, server.Frontend)

	if err != nil {
		return fmt.Errorf("failed to run stable passthrough: %w", err)
	}

	return nil
}

type Frontend struct {
	*pgproto3.Frontend
	conn io.Closer
}

func (f *Frontend) Close() {

	f.conn.Close()
}

func (ln *Connector) connectToServer(ctx context.Context, beStartupData StartupData) (*Frontend, error) {

	// TODO: Cache Tokens... but I don't know how to check TTL
	endpoint, err := ln.authClient.NewConnectionString(ctx, beStartupData.Database, beStartupData.User)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection string: %w", err)
	}

	// Use PGX to connect and log in

	log.WithField(ctx, "endpoint", endpoint).Debug("connecting to backend")
	cfg, err := pgconn.ParseConfig(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parsing backend config: %w", err)
	}

	log.WithField(ctx, "host", cfg.Host).Debug("connecting to backend")

	conn, err := pgconn.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", cfg.Host, err)
	}

	log.WithField(ctx, "host", cfg.Host).Debug("connected")
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

	log.Debug(ctx, "connected to backend")
	return &Frontend{
		conn:     hj.Conn,
		Frontend: hj.Frontend,
	}, nil
}

const TxStatusIdle = 'I'

type StartupData struct {
	User            string
	Database        string
	ProtocolVersion uint32
}

func beFatal(ctx context.Context, be *pgproto3.Backend, msg string) error {
	be.Send(&pgproto3.ErrorResponse{
		Severity: "FATAL",
		Message:  "no user in startup message",
	})
	be.Flush()
	return fmt.Errorf(msg)
}
func clientHandshake(ctx context.Context, be *pgproto3.Backend) (*StartupData, error) {
	msg, err := be.ReceiveStartupMessage()
	if err != nil {
		return nil, fmt.Errorf("failed to receive startup message: %w", err)
	}

	switch mt := msg.(type) {
	case *pgproto3.StartupMessage:
		user, ok := mt.Parameters["user"]
		if !ok {
			return nil, beFatal(ctx, be, "no user in startup message")
		}
		db, ok := mt.Parameters["database"]
		if !ok {
			db = user
		}

		if _, ok := mt.Parameters["replication"]; ok {
			return nil, beFatal(ctx, be, "replication is not supported")
		}

		if _, ok := mt.Parameters["options"]; ok {
			return nil, beFatal(ctx, be, "options is not supported")
		}

		return &StartupData{
			User:            user,
			Database:        db,
			ProtocolVersion: mt.ProtocolVersion,
		}, nil

	case *pgproto3.SSLRequest:
		return nil, beFatal(ctx, be, "ssl connections are not supported")
	case *pgproto3.CancelRequest:
		return nil, fmt.Errorf("cancel request received")
	default:
		return nil, fmt.Errorf("unknown message type: %T", mt)
	}
}

type sender[T any] interface {
	Send(T)
	Flush() error
}

type receiver[T any] interface {
	Receive() (T, error)
}

var ErrTerminate = fmt.Errorf("terminate")

func copyFrom[T pgproto3.Message](ctx context.Context, from receiver[T], to sender[T]) func() error {

	return func() error {
		var terminate bool
		for {
			msg, err := from.Receive()
			if err != nil {
				return fmt.Errorf("receive: %w", err)
			}
			// capture terminate, then send to the client, THEN break.
			_, terminate = pgproto3.Message(msg).(*pgproto3.Terminate)
			if terminate {
				log.WithField(ctx, "msg", msg).Info("received terminate message")
			} else {
				log.WithField(ctx, "msg", msg).Debug("received message")
			}
			to.Send(msg)
			err = to.Flush()
			if err != nil {
				return fmt.Errorf("flush: %w", err)
			}
			if terminate {
				// The normal, graceful termination procedure is that the frontend sends a Terminate message and immediately closes the connection. On receipt of this message, the backend closes the connection and terminates.
				return ErrTerminate
			}
			select {
			case <-ctx.Done():
				return nil
			default:
			}
		}
	}
}

func passthrough(ctx context.Context, client *pgproto3.Backend, server *pgproto3.Frontend) error {
	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(copyFrom(log.WithField(ctx, "conn", "fromServer"), server, client))
	eg.Go(copyFrom(log.WithField(ctx, "conn", "fromClient"), client, server))

	err := eg.Wait()
	if err == ErrTerminate {
		log.Info(ctx, "pgproxy: done with terminate")
		return nil
	} else if err != nil {
		log.WithError(ctx, err).Error("pgproxy: error copying messages")
		return err
	} else {
		log.Info(ctx, "pgproxy: done without error")
	}
	return nil
}
