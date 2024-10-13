package entrypoint

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/pentops/log.go/log"
	"github.com/pentops/o5-runtime-sidecar/adapters/grpc_reflect"
	"github.com/pentops/o5-runtime-sidecar/apps/bridge"
	"github.com/pentops/o5-runtime-sidecar/apps/httpserver"
	"github.com/pentops/o5-runtime-sidecar/apps/postgres/pgoutbox"
	"github.com/pentops/o5-runtime-sidecar/apps/postgres/pgproxy"
	"github.com/pentops/o5-runtime-sidecar/apps/queueworker"
	"github.com/pentops/runner"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var NothingToDoError = errors.New("no services configured")

type Runtime struct {
	sender Publisher

	queueWorker     *queueworker.App
	adapter         *bridge.App
	routerServer    *httpserver.Router
	outboxListeners []*pgoutbox.App
	postgresProxy   *pgproxy.App

	connections  []io.Closer
	endpoints    []string
	endpointWait chan struct{}
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

func (rt *Runtime) Run(ctx context.Context) error {
	log.Debug(ctx, "Sidecar Running")
	defer rt.Close()

	runGroup := runner.NewGroup(
		runner.WithName("runtime"),
		runner.WithCancelOnSignals(),
	)

	didAnything := false
	if rt.postgresProxy != nil {
		didAnything = true
		runGroup.Add("postgres-proxy", rt.postgresProxy.Run)
	}

	if rt.adapter != nil {
		didAnything = true
		runGroup.Add("adapter", rt.adapter.Run)
	}

	if err := runGroup.Start(ctx); err != nil {
		return fmt.Errorf("start goroutines: %w", err)
	}

	rt.endpointWait = make(chan struct{})
	runGroup.Add("register-endpoints", func(ctx context.Context) error {
		defer close(rt.endpointWait)
		for _, endpoint := range rt.endpoints {
			endpoint := endpoint
			if err := rt.registerEndpoint(ctx, endpoint); err != nil {
				return fmt.Errorf("register endpoint %s: %w", endpoint, err)
			}
		}
		return nil
	})

	for _, outbox := range rt.outboxListeners {
		didAnything = true
		runGroup.Add(outbox.Name, outbox.Run)
	}

	<-rt.endpointWait

	if rt.routerServer != nil {
		// TODO: Metrics
		didAnything = true

		runGroup.Add("router", rt.routerServer.Run)

	}

	if rt.queueWorker != nil {
		didAnything = true
		runGroup.Add("worker", rt.queueWorker.Run)
	}

	if !didAnything {
		return NothingToDoError
	}

	if err := runGroup.Wait(); err != nil {
		log.WithError(ctx, err).Error("Error in goroutines")
		return err
	}

	log.Info(ctx, "Sidecar Stopped with no error")
	return nil
}

func (rt *Runtime) registerEndpoint(ctx context.Context, endpoint string) error {

	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	rt.connections = append(rt.connections, conn)

	prClient := grpc_reflect.NewClient(conn)

	services, err := prClient.FetchServices(ctx, conn)
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
			if err := rt.routerServer.RegisterService(ctx, ss, prClient); err != nil {
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
