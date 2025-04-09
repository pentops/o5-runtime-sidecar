package pgoutbox

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_pb"
	"github.com/pentops/o5-runtime-sidecar/adapters/msgconvert"
	"github.com/pentops/o5-runtime-sidecar/sidecar"

	"github.com/pentops/log.go/log"
)

func TestOutbox(t *testing.T) {
	log.DefaultLogger.SetLevel(slog.LevelDebug)

	ctx := context.Background()

	db := getNewDB(ctx, t, "")
	defer db.Close(ctx)

	batcher := &testBatcher{
		chMsg: make(chan []*messaging_pb.Message),
	}

	conn := testConnector{
		dsn: db.Config().ConnString(),
	}

	conv := msgconvert.NewConverter(sidecar.AppInfo{})
	o, err := NewOutbox(conn, batcher, conv)
	if err != nil {
		t.Fatalf("failed to create outbox listener: %s", err)
	}

	runErr := make(chan error)

	outboxCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		runErr <- o.Run(outboxCtx)
	}()

	sendReceive := func(ids ...string) {
		_, err = db.Exec(ctx, "BEGIN")
		if err != nil {
			t.Fatalf("failed to begin transaction: %s", err)
		}

		for _, id := range ids {
			// send a message
			_, err = db.Exec(ctx, "INSERT INTO outbox (id, data, headers) VALUES ($1,$2,$3);", id, "{}", "")
			if err != nil {
				t.Fatalf("failed to insert message: %s", err)
			}
		}

		_, err = db.Exec(ctx, "COMMIT")
		if err != nil {
			t.Fatalf("failed to commit transaction: %s", err)
		}

		want := map[string]bool{}
		for _, id := range ids {
			want[id] = true
		}

		select {
		case batch := <-batcher.chMsg:
			for _, msg := range batch {
				_, ok := want[msg.MessageId]
				if !ok {
					t.Errorf("unexpected message: %s", msg.MessageId)
				}

				delete(want, msg.MessageId)
			}

			if len(want) > 0 {
				t.Errorf("missing messages: %v", want)
			}

		case <-time.After(time.Second * 5):
			t.Errorf("timed out waiting for messages")
		}
	}

	time.Sleep(time.Millisecond * 100)
	sendReceive(uuid.NewString(), uuid.NewString())

	time.Sleep(time.Millisecond * 100)
	sendReceive(uuid.NewString(), uuid.NewString())

	// boot the listener
	_, err = db.Exec(ctx, `
			SELECT pg_terminate_backend(pg_stat_activity.pid)
			FROM pg_stat_activity
			WHERE pg_stat_activity.datname = $1
			AND pid <> pg_backend_pid()`, dbName)
	if err != nil {
		t.Fatalf("failed to kill connections: %s", err)
	}

	time.Sleep(time.Millisecond * 100)
	sendReceive(uuid.NewString(), uuid.NewString())

	cancel()
	if err := <-runErr; err != nil {
		if !errors.Is(err, context.Canceled) {
			t.Errorf("listener error: %s ", err)
		}
	}
}
