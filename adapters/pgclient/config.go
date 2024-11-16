package pgclient

import (
	"context"
)

type AuthClient interface {
	ConnectionString(ctx context.Context, lookupName string) (string, error)
}

type directConnector struct {
	dsn  string
	name string
}

func NewDirectConnector(name, dsn string) PGConnector {
	return &directConnector{
		name: name,
		dsn:  dsn,
	}
}

func (dc *directConnector) Name() string {
	return dc.name
}

func (dc *directConnector) DSN(context.Context) (string, error) {
	return dc.dsn, nil
}
