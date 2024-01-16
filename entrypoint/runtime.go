package entrypoint

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/pentops/jwtauth/jwks"
	"github.com/pentops/log.go/log"
	"github.com/pentops/o5-runtime-sidecar/outbox"
	"github.com/pentops/o5-runtime-sidecar/protoread"
	"github.com/pentops/o5-runtime-sidecar/sqslink"
	"github.com/pentops/runner"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var NothingToDoError = errors.New("no services configured")

type Runtime struct {
	queueWorker     *sqslink.Worker
	sender          *outbox.SNSBatcher
	jwks            *jwks.JWKSManager
	adapter         *adapterServer
	routerServer    *routerServer
	outboxListeners []*outboxListener

	connections []io.Closer
	endpoints   []string
}

func NewRuntime() *Runtime {
	return &Runtime{}
}

func (rt *Runtime) Close() error {
	for _, conn := range rt.connections {
		if err := conn.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (rt *Runtime) buildRunGroup() (*runner.Group, error) {

	runGroup := runner.NewGroup(
		runner.WithName("runtime"),
		runner.WithCancelOnSignals(),
	)

	didAnything := false

	if rt.jwks != nil {
		// doesn't count as doing anything
		runGroup.Add("jwks", rt.jwks.Run)
	}

	for _, outbox := range rt.outboxListeners {
		didAnything = true
		runGroup.Add(outbox.Name, outbox.Run)
	}

	if rt.routerServer != nil {
		// TODO: Metrics
		didAnything = true

		runGroup.Add("router", rt.routerServer.Run)
	}

	if rt.queueWorker != nil {
		didAnything = true
		runGroup.Add("worker", rt.queueWorker.Run)
	}

	if rt.adapter != nil {
		didAnything = true
		runGroup.Add("adapter", rt.adapter.Run)
	}

	if !didAnything {
		return nil, NothingToDoError
	}
	return runGroup, nil
}

func (rt *Runtime) Run(ctx context.Context) error {
	log.Debug(ctx, "Sidecar Running")
	defer rt.Close()

	for _, endpoint := range rt.endpoints {
		endpoint := endpoint
		if err := rt.registerEndpoint(ctx, endpoint); err != nil {
			return fmt.Errorf("register endpoint %s: %w", endpoint, err)
		}
	}

	runGroup, err := rt.buildRunGroup()
	if err != nil {
		return err
	}

	if err := runGroup.Run(ctx); err != nil {
		log.WithError(ctx, err).Error("Error in goroutines")
		return err
	}

	log.Info(ctx, "Sidecar Stopped with no error")
	return nil
}

func (rt *Runtime) registerEndpoint(ctx context.Context, endpoint string) error {

	conn, err := grpc.DialContext(ctx, endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	rt.connections = append(rt.connections, conn)

	services, err := protoread.FetchServices(ctx, conn)
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}

	for _, ss := range services {
		name := string(ss.FullName())
		switch {
		case strings.HasSuffix(name, "Service"), strings.HasSuffix(name, "Sandbox"):
			if rt.routerServer == nil {
				return fmt.Errorf("service %s requires a public port", name)
			}
			if err := rt.routerServer.RegisterService(ctx, ss, conn); err != nil {
				return fmt.Errorf("register service %s: %w", name, err)
			}
		case strings.HasSuffix(name, "Topic"):
			if rt.queueWorker == nil {
				return fmt.Errorf("topic %s requires an SQS URL", name)
			}
			if err := rt.queueWorker.RegisterService(ctx, ss, conn); err != nil {
				return fmt.Errorf("register worker %s: %w", name, err)
			}
		default:
			log.WithField(ctx, "service", name).Error("Unknown service type")
			// but continue
		}
	}

	return nil
}
