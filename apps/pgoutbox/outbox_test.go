package pgoutbox

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_pb"
	"github.com/pentops/o5-runtime-sidecar/adapters/msgconvert"
	"github.com/pentops/o5-runtime-sidecar/sidecar"
	"github.com/pressly/goose"

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

func emptyDB(ctx context.Context, t *testing.T, name string) string {
	dbURL := os.Getenv("TEST_DB")
	if !strings.Contains(dbURL, "test") {
		t.Fatal("TEST_DB not a test database, it must contain the word 'test' somewhere in the connection string")
	}

	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect to db: %s", err)
	}
	defer conn.Close(ctx)

	conf, err := pgx.ParseConfig(dbURL)
	if err != nil {
		t.Fatalf("failed to parse db url: %s", err)
	}

	conf.Database = name

	_, err = conn.Exec(ctx, "DROP DATABASE "+name)
	if err != nil {
		t.Logf("failed to drop test database: %s", err)
	}

	_, err = conn.Exec(ctx, "CREATE DATABASE "+name)
	if err != nil {
		t.Fatalf("failed to create test database: %s", err)
	}

	return conf.ConnString()
}

func TestOutbox(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.DefaultLogger.SetLevel(slog.LevelDebug)

	dbURL := emptyDB(ctx, t, dbName)

	dbConn, err := sql.Open("postgres", dbURL)
	if err := goose.Up(dbConn, "./testdata/db"); err != nil {
		t.Fatal(err.Error())
	}

	source := sidecar.AppInfo{}
	batcher := &testBatcher{
		chMsg: make(chan []*messaging_pb.Message),
	}

	conn := testConnector{
		dsn: dbURL,
	}

	conv := msgconvert.NewConverter(source)
	ll, err := NewListener(conn, batcher, conv, false)
	if err != nil {
		t.Fatalf("failed to create outbox listener: %s", err)
	}

	chListener := make(chan error)

	go func() {
		err := ll.Listen(ctx)
		t.Logf("listener exited: %s", err)
		chListener <- err
	}()

	sendReceive := func(ids ...string) {
		_, err = dbConn.Exec("BEGIN")
		if err != nil {
			t.Fatalf("failed to begin transaction: %s", err)
		}

		for _, id := range ids {
			// send a message
			_, err = dbConn.Exec("INSERT INTO outbox (id, data, headers) VALUES ($1,$2,$3);", id, "{}", "")
			if err != nil {
				t.Fatalf("failed to insert message: %s", err)
			}
		}

		_, err = dbConn.Exec("COMMIT")
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
	_, err = dbConn.Exec(`
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

/*
func TestDelayedOutbox(t *testing.T) {
	log.DefaultLogger.SetLevel(slog.LevelDebug)
	dbURL := emptyDB(t, dbName+"_delayed")
	dbConn, err := sql.Open("postgres", dbURL)
	if err := goose.Up(dbConn, "./testdata/db_delayed"); err != nil {
		t.Fatal(err.Error())
	}

	source := sidecar.AppInfo{}
	batcher := &testBatcher{
		chMsg: make(chan []*messaging_pb.Message),
	}

	conn := testConnector{
		dsn: dbURL,
	}

	conv := msgconvert.NewConverter(source)
	ll, err := NewListener(conn, batcher, conv, true)
	if err != nil {
		t.Fatalf("failed to create outbox listener: %s", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chListener := make(chan error)

	go func() {
		err := ll.Listen(ctx)
		t.Logf("listener exited: %s", err)
		chListener <- err
	}()

	sendReceive := func(delay time.Time, ids ...string) {
		_, err = dbConn.Exec("BEGIN")
		if err != nil {
			t.Fatalf("failed to begin transaction: %s", err)
		}

		for _, id := range ids {
			// send a message
			_, err = dbConn.Exec("INSERT INTO outbox (id, data, headers, send_after) VALUES ($1,$2,$3,$4);", id, "{}", "", delay)
			if err != nil {
				t.Fatalf("failed to insert message: %s", err)
			}
		}

		_, err = dbConn.Exec("COMMIT")
		if err != nil {
			t.Fatalf("failed to commit transaction: %s", err)
		}

		want := map[string]bool{}
		for _, id := range ids {
			want[id] = true
		}

		var batch []*messaging_pb.Message

		select {
		case batch = <-batcher.chMsg:
		case <-time.After(time.Second * 5):
			t.Fatalf("timeout waiting for messages")
		}

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
	sendReceive(time.Now(), uuid.NewString(), uuid.NewString())

	time.Sleep(time.Millisecond * 100)
	sendReceive(time.Now(), uuid.NewString(), uuid.NewString())

	// boot the listener
	_, err = dbConn.Exec(`
		SELECT pg_terminate_backend(pg_stat_activity.pid)
		FROM pg_stat_activity
		WHERE pg_stat_activity.datname = $1
		AND pid <> pg_backend_pid()`, dbName+"_delayed")
	if err != nil {
		t.Fatalf("failed to kill connections: %s", err)
	}

	time.Sleep(time.Millisecond * 100)
	sendReceive(time.Now().Add(1*time.Second), uuid.NewString(), uuid.NewString())

	time.Sleep(time.Millisecond * 100)
	sendReceive(time.Now(), uuid.NewString(), uuid.NewString())

	cancel()
	if err := <-chListener; err != nil {
		if !errors.Is(err, context.Canceled) {
			t.Errorf("listener error: %s ", err)
		}
	}
}
*/
