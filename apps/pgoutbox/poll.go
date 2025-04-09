package pgoutbox

import (
	"context"
	"fmt"
	"time"

	"github.com/pentops/log.go/log"
)

var pollInterval = 1 * time.Minute

func (o *Outbox) poll(ctx context.Context) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		log.Debug(ctx, "polling for delayed messages")

		select {
		case <-ctx.Done():
			log.Info(ctx, "context done, polling stopped")
			return nil

		case <-ticker.C:
			conn, err := o.pool.Acquire(ctx)
			if err != nil {
				return fmt.Errorf("pool: acquire: %w", err)
			}

			err = o.loopUntilEmpty(ctx, conn)
			if err != nil {
				conn.Release()
				return fmt.Errorf("poll: %w", err)
			}

			conn.Release()
		}
	}
}
