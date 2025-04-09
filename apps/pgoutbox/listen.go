package pgoutbox

import (
	"context"
	"fmt"

	"github.com/pentops/log.go/log"
)

func (o *Outbox) listen(ctx context.Context) error {
	conn, err := o.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquiring connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN outboxmessage"); err != nil {
		return err
	}

	for {
		log.Debug(ctx, "waiting for notification")

		_, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			log.WithError(ctx, err).Warn("listener error, reconnecting")

			conn.Release()
			o.pool.Reset()

			newConn, err := o.pool.Acquire(ctx)
			if err != nil {
				return err
			}
			conn = newConn

			if _, err := conn.Exec(ctx, "LISTEN outboxmessage"); err != nil {
				return err
			}

			log.Info(ctx, "reconnected to PG")
		} else {
			log.Debug(ctx, "received notification")
		}

		err = o.loopUntilEmpty(ctx, conn)
		if err != nil {
			return err
		}
	}
}
