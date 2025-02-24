package pgserver

import (
	"context"
	"fmt"

	"github.com/pentops/log.go/log"
	"golang.org/x/sync/errgroup"

	"github.com/jackc/pgx/v5/pgproto3"
)

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
