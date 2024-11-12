package pgclient

import (
	"context"
)

type AuthClient interface {
	ConnectionString(ctx context.Context, lookupName string) (string, error)
}

type PGConnector interface {
	Name() string
	DSN(ctx context.Context) (string, error)
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

/*

func (dc *directConnector) ConnectToServer(ctx context.Context) (*Frontend, error) {
	return dialFrontend(ctx, dc.dsn)
}

func (dc *directConnector) ConnectPGX(ctx context.Context) (*pgx.Conn, error) {
	return pgx.Connect(ctx, dc.dsn)
}

type NetDialer struct{}

func (NetDialer) Dial(network, address string) (net.Conn, error) {
	return net.Dial(network, address)
}

func (NetDialer) DialTimeout(network, address string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout(network, address, timeout)
}*/
