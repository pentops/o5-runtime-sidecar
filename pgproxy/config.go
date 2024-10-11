package pgproxy

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/lib/pq"
)

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

func TryParsePGString(name string) (string, bool, error) {
	// Full URL structure postgres://user:password@host:port/dbname
	// 'Connection String' - key=value style
	if strings.HasPrefix(name, "postgres://") {
		pp, err := url.Parse(name)
		if err != nil {
			return "", false, fmt.Errorf("parsing URL: %w", err)
		}
		name := strings.TrimPrefix(pp.Path, "/")
		if name == "" || strings.Contains(name, "/") {
			return "", false, fmt.Errorf("no database name in URL")
		}
		return name, true, nil
	}

	if strings.Contains(name, "=") {
		parts := strings.Split(name, " ")
		for _, part := range parts {
			if part == "" {
				continue
			}
			kv := strings.Split(part, "=")
			if len(kv) != 2 {
				return "", false, fmt.Errorf("invalid key=value pair: %q", part)
			}
			if kv[0] == "dbname" {
				return kv[1], true, nil
			}
		}

		return "", false, fmt.Errorf("no dbname found in connection string")
	}

	return "", false, nil
}
