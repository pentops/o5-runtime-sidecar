package entrypoint

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/lib/pq"
	"github.com/pentops/o5-runtime-sidecar/awsmsg"
	"github.com/pentops/o5-runtime-sidecar/outbox"
	"github.com/pentops/o5-runtime-sidecar/pgproxy"
)

type AuroraIAMEnvVar struct {
	Endpoint string `json:"endpoint"` // Address and Port
	DbName   string `json:"dbName"`
	DbUser   string `json:"dbUser"`
}

type postgresConn struct {
	Name   string
	DSN    string
	Aurora *AuroraIAMEnvVar
}

func looksLikePGConnString(name string) bool {
	// Full URL structure postgres://user:password@host:port/dbname
	// 'Connection String' - key=value style
	return strings.HasPrefix(name, "postgres://") || strings.Contains(name, "=")
}

func looksLikeJSONString(name string) bool {
	return strings.HasPrefix(name, "{") && strings.HasSuffix(name, "}")
}

var reValidDBEnvName = regexp.MustCompile(`^[A-Z0-9_]+$`)

func buildPostgres(raw string) (*postgresConn, error) {
	if looksLikePGConnString(raw) {
		// Credentials were passed directly in the env, use as-is
		return &postgresConn{
			DSN: raw,
		}, nil
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
	if looksLikePGConnString(envCreds) {
		return &postgresConn{
			Name: raw,
			DSN:  envCreds,
		}, nil
	}

	if !looksLikeJSONString(envCreds) {
		return nil, fmt.Errorf("invalid DB credentials in $DB_CREDS_%s, expecing a DSN or JSON value", raw)
	}

	arg := AuroraIAMEnvVar{}
	if err := json.Unmarshal([]byte(envCreds), &arg); err != nil {
		return nil, fmt.Errorf("invalid JSON in $DB_CREDS_%s: %w", raw, err)
	}

	dsn := fmt.Sprintf("user=%s dbname=%s",
		arg.DbUser,
		arg.DbUser,
	)

	return &postgresConn{
		DSN:    dsn,
		Name:   raw,
		Aurora: &arg,
	}, nil
}

func buildPostgresConn(endpoint *AuroraIAMEnvVar, awsProvider AWSProvider) (pgproxy.AuthClient, error) {
	region := awsProvider.Region()
	creds := awsProvider.Credentials()

	builder, err := pgproxy.NewCredBuilder(creds, endpoint.Endpoint, region)
	if err != nil {
		return nil, err
	}

	return builder, nil
}

type postgresProxy struct {
	conn     *pgproxy.Connector
	listener *pgproxy.Listener
}

func newPostgresProxy(awsProvider AWSProvider, bind string, conns []*postgresConn) (*postgresProxy, error) {

	builders := pgproxy.CredMap{}

	for _, conn := range conns {
		if conn.Aurora == nil {
			return nil, fmt.Errorf("no aurora config for %q", conn.Name)
		}

		builder, err := buildPostgresConn(conn.Aurora, awsProvider)
		if err != nil {
			return nil, fmt.Errorf("building postgres connection: %w", err)
		}
		builders[conn.Aurora.DbName] = builder
	}

	connector, err := pgproxy.NewConnector(builders)
	if err != nil {
		return nil, err
	}

	listener, err := pgproxy.NewListener(connector, bind)
	if err != nil {
		return nil, fmt.Errorf("failed to create listener: %w", err)
	}

	return &postgresProxy{
		conn:     connector,
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

func newOutboxListener(conn *postgresConn, batcher outbox.Batcher, source awsmsg.SourceConfig, awsProvider AWSProvider) (*outboxListener, error) {

	var dialer pq.Dialer

	if conn.Aurora == nil {
		dialer = outbox.NetDialer{}
	} else {
		builder, err := buildPostgresConn(conn.Aurora, awsProvider)
		if err != nil {
			return nil, fmt.Errorf("building postgres connection: %w", err)
		}
		connector, err := pgproxy.NewConnector(builder)
		if err != nil {
			return nil, err
		}

		dialer = connector
	}

	ll, err := outbox.NewListener(conn.DSN, dialer, batcher, source)
	if err != nil {
		return nil, fmt.Errorf("failed to create outbox listener: %w", err)
	}

	return &outboxListener{
		Name:     fmt.Sprintf("outbox-%s", conn.Name),
		Listener: ll,
	}, nil
}

func (ol *outboxListener) Run(ctx context.Context) error {
	return ol.Listener.Listen(ctx)
}
