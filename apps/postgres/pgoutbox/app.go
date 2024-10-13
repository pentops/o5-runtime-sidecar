package pgoutbox

import (
	"context"
	"fmt"

	"github.com/pentops/o5-runtime-sidecar/adapters/postgres"
	"github.com/pentops/o5-runtime-sidecar/sidecar"
)

type OutboxConfig struct {
	PostgresOutboxURI []string `env:"POSTGRES_OUTBOX" default:""`
}

type App struct {
	Name string
	*Listener
}

func NewApps(envConfig OutboxConfig, srcConfig sidecar.AppInfo, sender Batcher, pgConfigs postgres.ConfigSet) ([]*App, error) {
	var apps []*App
	for _, rawVar := range envConfig.PostgresOutboxURI {
		conn, err := pgConfigs.GetConnector(rawVar)
		if err != nil {
			return nil, fmt.Errorf("building postgres connection: %w", err)
		}

		app, err := NewApp(conn, sender, srcConfig)
		if err != nil {
			return nil, fmt.Errorf("creating outbox listener: %w", err)
		}
		apps = append(apps, app)
	}
	return apps, nil
}

func NewApp(conn postgres.PGConnector, batcher Batcher, source sidecar.AppInfo) (*App, error) {
	name := conn.Name()
	ll, err := NewListener(conn, batcher, source)
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
