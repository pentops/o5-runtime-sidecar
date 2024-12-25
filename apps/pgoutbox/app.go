package pgoutbox

import (
	"context"
	"fmt"

	"github.com/pentops/o5-runtime-sidecar/adapters/pgclient"
)

type OutboxConfig struct {
	PostgresOutboxURI []string `env:"POSTGRES_OUTBOX" default:""`
}

type App struct {
	Name string
	*Listener
}

func NewApps(envConfig OutboxConfig, parser Parser, sender Batcher, pgConfigs pgclient.ConfigSet) ([]*App, error) {
	var apps []*App
	for _, rawVar := range envConfig.PostgresOutboxURI {
		conn, err := pgConfigs.GetConnector(rawVar)
		if err != nil {
			return nil, fmt.Errorf("building postgres connection: %w", err)
		}

		app, err := NewApp(conn, sender, parser)
		if err != nil {
			return nil, fmt.Errorf("creating outbox listener: %w", err)
		}
		apps = append(apps, app)
	}
	return apps, nil
}

func NewApp(conn pgclient.PGConnector, batcher Batcher, parser Parser) (*App, error) {
	name := conn.Name()
	ll, err := NewListener(conn, batcher, parser)
	if err != nil {
		return nil, fmt.Errorf("failed to create outbox listener: %w", err)
	}

	return &App{
		Name:     fmt.Sprintf("outbox-%s", name),
		Listener: ll,
	}, nil
}

func (ol *App) Run(ctx context.Context) error {
	return ol.Listener.Listen(ctx)
}
