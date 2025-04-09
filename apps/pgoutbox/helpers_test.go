package pgoutbox

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_pb"
	"github.com/pressly/goose/v3"
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

const dbName = "test_outbox_24564546765"

func getNewDB(ctx context.Context, t *testing.T, suffix string) *pgx.Conn {
	name := dbName + suffix

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

	q := fmt.Sprintf("DROP DATABASE IF EXISTS %s", name)

	_, err = conn.Exec(ctx, q)
	if err != nil {
		t.Logf("failed to drop test database: %s", err)
	}

	q = fmt.Sprintf("CREATE DATABASE %s", name)

	_, err = conn.Exec(ctx, q)
	if err != nil {
		t.Fatalf("failed to create test database: %s", err)
	}

	// Update the database name in the URL
	u.Path = name

	newConf, err := pgx.ParseConfig(u.String())
	if err != nil {
		t.Fatalf("failed to parse new db url: %s", err)
	}

	sql, err := goose.OpenDBWithDriver("pgx", newConf.ConnString())
	if err != nil {
		t.Fatalf("failed to open db: %s", err)
	}

	if err := goose.Up(sql, "./testdata/db"+suffix); err != nil {
		t.Fatal(err.Error())
	}

	newConn, err := pgx.ConnectConfig(ctx, newConf)
	if err != nil {
		t.Fatalf("failed to connect to db: %s", err)
	}

	return newConn
}
