package pgproxy

import (
	"context"
	"fmt"
	"strings"

	"github.com/pentops/o5-runtime-sidecar/adapters/postgres"
)

type ProxyConfig struct {
	PostgresProxy     []string `env:"POSTGRES_IAM_PROXY" default:""`
	PostgresProxyBind string   `env:"POSTGRES_PROXY_BIND" default:"/socket/postgres"`
}

type App struct {
	listener *Listener
}

func NewApp(cfg ProxyConfig, pgConfigs postgres.ConfigSet) (*App, error) {

	connectors := map[string]postgres.PGConnector{}
	for _, rawVar := range cfg.PostgresProxy {
		conn, err := pgConfigs.GetConnector(rawVar)
		if err != nil {
			return nil, fmt.Errorf("building postgres connection: %w", err)
		}
		connectors[conn.Name()] = conn
	}
	var net string
	bind := cfg.PostgresProxyBind
	if strings.HasPrefix(bind, "/") {
		net = "unix"
	} else {
		net = "tcp"
	}

	listener, err := NewListener(net, bind, connectors)
	if err != nil {
		return nil, fmt.Errorf("failed to create listener: %w", err)
	}

	return &App{
		listener: listener,
	}, nil
}

func (pp *App) Run(ctx context.Context) error {
	return pp.listener.Listen(ctx)
}
