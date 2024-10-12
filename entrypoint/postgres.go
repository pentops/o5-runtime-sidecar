package entrypoint

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/pentops/o5-runtime-sidecar/awsmsg"
	"github.com/pentops/o5-runtime-sidecar/outbox"
	"github.com/pentops/o5-runtime-sidecar/pgproxy"
)

func looksLikeJSONString(name string) bool {
	return strings.HasPrefix(name, "{") && strings.HasSuffix(name, "}")
}

var reValidDBEnvName = regexp.MustCompile(`^[A-Z0-9_]+$`)

type pgConnSet struct {
	awsProvider AWSProvider
	credBuilder *pgproxy.CredBuilder
	connectors  map[string]pgproxy.PGConnector
}

func newPGConnSet(awsProvider AWSProvider) *pgConnSet {
	return &pgConnSet{
		awsProvider: awsProvider,
		connectors:  make(map[string]pgproxy.PGConnector),
	}
}

func (ss *pgConnSet) direct(name, raw string) (pgproxy.PGConnector, error) {
	if conn, ok := ss.connectors[name]; ok {
		return conn, nil
	}
	conn := pgproxy.NewDirectConnector(name, raw)
	ss.connectors[name] = conn
	return conn, nil
}

func (ss *pgConnSet) aurora(name string, config *pgproxy.AuroraConfig) (pgproxy.PGConnector, error) {
	if conn, ok := ss.connectors[name]; ok {
		return conn, nil
	}

	if ss.credBuilder == nil {
		ss.credBuilder = pgproxy.NewCredBuilder(ss.awsProvider.Credentials(), ss.awsProvider.Region())
	}

	if err := ss.credBuilder.AddConfig(name, config); err != nil {
		return nil, fmt.Errorf("adding config for %q: %w", name, err)
	}

	conn, err := pgproxy.NewAuroraConnector(name, ss.credBuilder)
	if err != nil {
		return nil, err
	}
	ss.connectors[name] = conn
	return conn, nil
}

func (ss *pgConnSet) getConnector(raw string) (pgproxy.PGConnector, error) {

	name, ok, err := pgproxy.TryParsePGString(raw)
	if err != nil {
		return nil, fmt.Errorf("parsing postgres string: %w", err)
	}
	if ok {
		// Credentials were passed directly in the env, use as-is
		return ss.direct(name, raw)
	}

	if !reValidDBEnvName.MatchString(raw) {
		return nil, fmt.Errorf("invalid DB ref/name: %q", raw)
	}

	// the Name passed in should be just the DB name with matching env var
	envCreds := os.Getenv("DB_CREDS_" + raw)
	if envCreds == "" {
		// safe to log since the regex makes it very hard to store a password...
		return nil, fmt.Errorf("no credentials found - expecting $DB_CREDS_%s", raw)
	}
	_, ok, err = pgproxy.TryParsePGString(envCreds)
	if err != nil {
		return nil, fmt.Errorf("parsing postgres string from $DB_CREDS_%s: %w", raw, err)
	}
	if ok {
		// use the raw name as the connection name, but the parsed DSN
		return ss.direct(raw, envCreds)
	}

	if !looksLikeJSONString(envCreds) {
		return nil, fmt.Errorf("invalid DB credentials in $DB_CREDS_%s, expecing a DSN or JSON value", raw)
	}

	config := &pgproxy.AuroraConfig{}
	if err := json.Unmarshal([]byte(envCreds), config); err != nil {
		return nil, fmt.Errorf("invalid JSON in $DB_CREDS_%s: %w", raw, err)
	}

	return ss.aurora(raw, config)
}

type postgresProxy struct {
	listener *pgproxy.Listener
}

func newPostgresProxy(bind string, conns map[string]pgproxy.PGConnector) (*postgresProxy, error) {
	listener, err := pgproxy.NewListener("unix", bind, conns)
	if err != nil {
		return nil, fmt.Errorf("failed to create listener: %w", err)
	}

	return &postgresProxy{
		listener: listener,
	}, nil
}

func (pp *postgresProxy) Run(ctx context.Context) error {
	return pp.listener.Listen(ctx)
}

type outboxListener struct {
	Name string
	*outbox.Listener
}

func newOutboxListener(conn pgproxy.PGConnector, batcher outbox.Batcher, source awsmsg.SourceConfig) (*outboxListener, error) {
	name := conn.Name()
	ll, err := outbox.NewListener(conn, batcher, source)
	if err != nil {
		return nil, fmt.Errorf("failed to create outbox listener: %w", err)
	}

	return &outboxListener{
		Name:     fmt.Sprintf("outbox-%s", name),
		Listener: ll,
	}, nil
}

func (ol *outboxListener) Run(ctx context.Context) error {
	return ol.Listener.Listen(ctx)
}
