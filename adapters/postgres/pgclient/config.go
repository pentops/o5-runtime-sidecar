package pgclient

import (
	"context"
	"net"
	"time"

	"github.com/lib/pq"
)

type AuthClient interface {
	ConnectionString(ctx context.Context, lookupName string) (string, error)
}

type PGConnector interface {
	Name() string
	PQDialer(context.Context) (pq.Dialer, string, error)
	ConnectToServer(context.Context) (*Frontend, error)
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

func (dc *directConnector) PQDialer(_ context.Context) (pq.Dialer, string, error) {
	return NetDialer{}, dc.dsn, nil
}

func (dc *directConnector) Name() string {
	return dc.name
}

func (dc *directConnector) ConnectToServer(ctx context.Context) (*Frontend, error) {
	return dialPGX(ctx, dc.dsn)

}

type NetDialer struct{}

func (NetDialer) Dial(network, address string) (net.Conn, error) {
	return net.Dial(network, address)
}

func (NetDialer) DialTimeout(network, address string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout(network, address, timeout)
}
