package pgoutbox

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_pb"
	"github.com/pentops/o5-runtime-sidecar/adapters/msgconvert"
	"github.com/pentops/o5-runtime-sidecar/sidecar"
	"github.com/pressly/goose/v3"

	"github.com/pentops/log.go/log"
)

type testBatcher struct {
	chMsg chan []*messaging_pb.Message
}

func (tb *testBatcher) PublishBatch(ctx context.Context, messages []*messaging_pb.Message) ([]string, error) {
	ids := make([]string, len(messages))
	for idx, msg := range messages {
		ids[idx] = msg.MessageId
	}
	go func() {
		tb.chMsg <- messages
	}()
	return ids, nil
}

type testConnector struct {
	dsn string
}

func (tc testConnector) DSN(ctx context.Context) (string, error) {
	return tc.dsn, nil
}

func (tc testConnector) Name() string {
	return "test"
}

var _ pgConnector = testConnector{}

const dbName = "test_outbox_24564546765"

func emptyDB(ctx context.Context, t *testing.T, name string) *pgx.ConnConfig {
	dbURL := os.Getenv("TEST_DB")
	if !strings.Contains(dbURL, "test") {
		t.Fatal("TEST_DB not a test database, it must contain the word 'test' somewhere in the connection string")
	}

	u, err := url.Parse(dbURL)
	if err != nil {
		t.Fatalf("failed to parse db url: %s", err)
	}

	conf, err := pgx.ParseConfig(u.String())
	if err != nil {
		t.Fatalf("failed to parse db url: %s", err)
	}

	conn, err := pgx.ConnectConfig(ctx, conf)
	if err != nil {
		t.Fatalf("failed to connect to db: %s", err)
	}
	defer conn.Close(ctx)

	_, err = conn.Exec(ctx, "DROP DATABASE "+name)
	if err != nil {
		t.Logf("failed to drop test database: %s", err)
	}

	_, err = conn.Exec(ctx, "CREATE DATABASE "+name)
	if err != nil {
		t.Fatalf("failed to create test database: %s", err)
	}

	// Update the database name in the URL
	u.Path = name

	newConf, err := pgx.ParseConfig(u.String())
	if err != nil {
		t.Fatalf("failed to parse new db url: %s", err)
	}

	return newConf
}

func TestOutbox(t *testing.T) {
	log.DefaultLogger.SetLevel(slog.LevelDebug)

	ctx := context.Background()

	dbconf := emptyDB(ctx, t, dbName)

	dbConn, err := pgx.ConnectConfig(ctx, dbconf)
	if err != nil {
		t.Fatalf("failed to connect to db: %s", err)
	}
	defer dbConn.Close(ctx)

	sql, err := goose.OpenDBWithDriver("pgx", dbconf.ConnString())
	if err != nil {
		t.Fatalf("failed to open db: %s", err)
	}

	if err := goose.Up(sql, "./testdata/db"); err != nil {
		t.Fatal(err.Error())
	}

	source := sidecar.AppInfo{}
	batcher := &testBatcher{
		chMsg: make(chan []*messaging_pb.Message),
	}

	conn := testConnector{
		dsn: dbconf.ConnString(),
	}

	conv := msgconvert.NewConverter(source)
	o, err := NewListener(conn, batcher, conv)
	if err != nil {
		t.Fatalf("failed to create outbox listener: %s", err)
	}

	chListener := make(chan error)

	outboxCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		err := o.Listen(outboxCtx)
		t.Logf("outbox exited: %s", err)
		chListener <- err
	}()

	sendReceive := func(ids ...string) {
		_, err = dbConn.Exec(ctx, "BEGIN")
		if err != nil {
			t.Fatalf("failed to begin transaction: %s", err)
		}

		for _, id := range ids {
			// send a message
			_, err = dbConn.Exec(ctx, "INSERT INTO outbox (id, data, headers) VALUES ($1,$2,$3);", id, "{}", "")
			if err != nil {
				t.Fatalf("failed to insert message: %s", err)
			}
		}

		_, err = dbConn.Exec(ctx, "COMMIT")
		if err != nil {
			t.Fatalf("failed to commit transaction: %s", err)
		}

		want := map[string]bool{}
		for _, id := range ids {
			want[id] = true
		}

		batch := <-batcher.chMsg
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
	}

	time.Sleep(time.Millisecond * 100)
	sendReceive(uuid.NewString(), uuid.NewString())

	time.Sleep(time.Millisecond * 100)
	sendReceive(uuid.NewString(), uuid.NewString())

	// boot the listener
	_, err = dbConn.Exec(ctx, `
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
	if err := <-chListener; err != nil {
		if !errors.Is(err, context.Canceled) {
			t.Errorf("listener error: %s ", err)
		}
	}
}
