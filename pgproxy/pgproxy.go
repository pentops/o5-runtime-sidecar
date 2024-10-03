package pgproxy

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/pentops/log.go/log"
	"golang.org/x/sync/errgroup"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgproto3"
)

type Listener struct {
	rdsClient AuthClient
}

type AuthClient interface {
	NewConnectionString(ctx context.Context, dbName, userName string) (string, error)
}

func NewListener(rdsClient AuthClient) (*Listener, error) {
	return &Listener{
		rdsClient: rdsClient,
	}, nil
}

func (l *Listener) Listen(ctx context.Context, bind string) error {
	ln, err := net.Listen("tcp", bind)
	if err != nil {
		return err
	}

	log.WithField(ctx, "bind", bind).Info("pgproxy: Ready")

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

	// Client == Backend the PG Protocol implements what a server implements.
	client := pgproto3.NewBackend(clientConn, clientConn)
	beStartupData, err := clientHandshake(client)
	if err != nil {
		log.WithError(ctx, err).Error("failed to handshake with client")
		return
	}
	ctx = log.WithFields(ctx, map[string]interface{}{
		"user":     beStartupData.User,
		"database": beStartupData.Database,
	})
	log.WithField(ctx, "data", beStartupData).Info("client startup")

	server, err := ln.connectToServer(ctx, *beStartupData)
	if err != nil {
		log.WithError(ctx, err).Error("failed to connect to server")
	}
	defer server.Close()

	client.Send(&pgproto3.AuthenticationOk{})
	client.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
	err = client.Flush()
	if err != nil {
		log.WithError(ctx, err).Error("failed to send ready for query")
		return
	}

	err = passthrough(ctx, client, server.Frontend)
	if err != nil {
		log.WithError(ctx, err).Error("failed to copy steady state")
	}
}

type Frontend struct {
	*pgproto3.Frontend
	conn io.Closer
}

func (f *Frontend) Close() {
	f.conn.Close()
}

func (ln *Listener) connectToServer(ctx context.Context, beStartupData StartupData) (*Frontend, error) {

	// TODO: Cache Tokens... but I don't know how to check TTL
	endpoint, err := ln.rdsClient.NewConnectionString(ctx, beStartupData.Database, beStartupData.User)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection string: %w", err)
	}

	// Use PGX to connect and log in

	cfg, err := pgconn.ParseConfig(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parsing backend config: %w", err)
	}
	conn, err := pgconn.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", cfg.Host, err)
	}

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
	User     string
	Database string
}

func clientHandshake(be *pgproto3.Backend) (*StartupData, error) {
	msg, err := be.ReceiveStartupMessage()
	if err != nil {
		return nil, fmt.Errorf("failed to receive startup message: %w", err)
	}

	switch mt := msg.(type) {
	case *pgproto3.StartupMessage:
		user, ok := mt.Parameters["user"]
		if !ok {
			be.Send(&pgproto3.ErrorResponse{
				Severity: "FATAL",
				Message:  "no user in startup message",
			})
			return nil, fmt.Errorf("no user in startup message")
		}
		db, ok := mt.Parameters["database"]
		if !ok {
			db = user
		}

		if _, ok := mt.Parameters["replication"]; ok {
			be.Send(&pgproto3.ErrorResponse{
				Severity: "FATAL",
				Message:  "replication connections are not supported",
			})
			return nil, fmt.Errorf("replication connections are not supported")
		}

		if _, ok := mt.Parameters["options"]; ok {
			be.Send(&pgproto3.ErrorResponse{
				Severity: "FATAL",
				Message:  "options are not supported",
			})
			return nil, fmt.Errorf("options are not supported")
		}

		// mt.ProtocolVersion
		// TODO: The pgproto version is pre-set, we need to negotiate with the
		// client if required.

		return &StartupData{
			User:     user,
			Database: db,
		}, nil
	case *pgproto3.SSLRequest:
		be.Send(&pgproto3.ErrorResponse{Message: "SSL connections are not supported"})
		return nil, fmt.Errorf("ssl request received")
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

func copyFrom[T pgproto3.Message](ctx context.Context, from receiver[T], to chan T) func() error {

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
			select {
			case to <- msg:
			case <-ctx.Done():
				return nil
			}
			if terminate {
				// The normal, graceful termination procedure is that the frontend sends a Terminate message and immediately closes the connection. On receipt of this message, the backend closes the connection and terminates.
				return ErrTerminate
			}
		}
	}
}

func copyTo[T any](ctx context.Context, from chan T, to sender[T]) func() error {
	return func() error {
		for {
			select {
			case msg := <-from:
				to.Send(msg)
				err := to.Flush()
				if err != nil {
					return err
				}
			case <-ctx.Done():
				return nil
			}
		}
	}
}

func passthrough(ctx context.Context, client *pgproto3.Backend, server *pgproto3.Frontend) error {
	eg, ctx := errgroup.WithContext(ctx)

	clientMsgs := make(chan pgproto3.FrontendMessage)
	serverMsgs := make(chan pgproto3.BackendMessage)

	eg.Go(copyFrom(log.WithField(ctx, "conn", "fromServer"), server, serverMsgs))
	eg.Go(copyTo(ctx, serverMsgs, client))
	eg.Go(copyFrom(log.WithField(ctx, "conn", "fromClient"), client, clientMsgs))
	eg.Go(copyTo(ctx, clientMsgs, server))

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
