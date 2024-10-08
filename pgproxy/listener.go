package pgproxy

import (
	"context"
	"net"

	"github.com/pentops/log.go/log"
)

type Listener struct {
	connector *Connector
	bind      string
}

func NewListener(connector *Connector, bind string) (*Listener, error) {
	return &Listener{
		connector: connector,
		bind:      bind,
	}, nil
}

func (l *Listener) Listen(ctx context.Context) error {
	ln, err := net.Listen("tcp", l.bind)
	if err != nil {
		return err
	}

	log.WithField(ctx, "bind", l.bind).Info("pgproxy: Ready")

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go l.newConn(ctx, conn)
	}
}

func (ln *Listener) newConn(ctx context.Context, clientConn net.Conn) {
	defer clientConn.Close()
	ctx = log.WithField(ctx, "clientAddr", clientConn.RemoteAddr().String())
	err := ln.connector.RunConn(ctx, clientConn)
	log.WithError(ctx, err).Error("pg proxy connection failure")
}
