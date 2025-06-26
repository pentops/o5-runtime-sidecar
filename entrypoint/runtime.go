package entrypoint

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/pentops/log.go/log"
	"github.com/pentops/o5-runtime-sidecar/adapters/grpcreflect"

	"github.com/pentops/o5-runtime-sidecar/adapters/msgconvert"
	"github.com/pentops/o5-runtime-sidecar/apps/bridge"
	"github.com/pentops/o5-runtime-sidecar/apps/httpserver"
	"github.com/pentops/o5-runtime-sidecar/apps/pgoutbox"
	"github.com/pentops/o5-runtime-sidecar/apps/pgproxy"
	"github.com/pentops/o5-runtime-sidecar/apps/queueworker"
	"github.com/pentops/runner"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var ErrNothingToDo = errors.New("no services configured")

type Runtime struct {
	sender Publisher

	queueWorker     *queueworker.App
	adapter         *bridge.App
	routerServer    *httpserver.Router
	outboxListeners []*pgoutbox.App
	postgresProxy   *pgproxy.App

	msgConverter *msgconvert.Converter

	reflectionClients []*grpcreflect.ReflectionClient
	endpoints         []string
	endpointWait      chan struct{}
}

func NewRuntime() *Runtime {
	return &Runtime{}
}

func (rt *Runtime) Close() error {
	for _, conn := range rt.reflectionClients {
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
			prClient, err := rt.connectEndpoint(endpoint)
			if err != nil {
				return fmt.Errorf("connect to endpoint %s: %w", endpoint, err)
			}

			rt.reflectionClients = append(rt.reflectionClients, prClient)

			if err := rt.registerEndpoint(ctx, prClient); err != nil {
				return fmt.Errorf("register endpoint %s: %w", prClient.Name(), err)
			}
		}

		if len(rt.reflectionClients) == 1 {
			// TODO: Handle multiple clients
			rt.msgConverter.SetReflectionClient(rt.reflectionClients[0])
		}

		return nil
	})

	for _, o := range rt.outboxListeners {
		didAnything = true
		runGroup.Add(o.Name, o.Run)
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
		return ErrNothingToDo
	}

	if err := runGroup.Wait(); err != nil {
		log.WithError(ctx, err).Error("Error in goroutines")
		return err
	}

	log.Info(ctx, "Sidecar Stopped with no error")
	return nil
}

func (rt *Runtime) connectEndpoint(endpoint string) (*grpcreflect.ReflectionClient, error) {
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}

	return grpcreflect.NewClient(conn), nil
}

func (rt *Runtime) registerEndpoint(ctx context.Context, prClient *grpcreflect.ReflectionClient) error {
	ss, err := prClient.FetchServices(ctx)
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}

	for _, s := range ss {
		name := string(s.FullName())

		switch {
		case strings.HasSuffix(name, "Service"), strings.HasSuffix(name, "Sandbox"):
			if rt.routerServer == nil {
				return fmt.Errorf("service %s requires a public port", name)
			}

			if err := rt.routerServer.RegisterService(ctx, s, prClient); err != nil {
				return fmt.Errorf("register service %s: %w", name, err)
			}

		case strings.HasSuffix(name, "Topic"):
			if rt.queueWorker == nil {
				return fmt.Errorf("topic %s requires an SQS URL", name)
			}

			if err := rt.queueWorker.RegisterService(ctx, s, prClient); err != nil {
				return fmt.Errorf("register worker %s: %w", name, err)
			}

		default:
			log.WithField(ctx, "service", name).Error("Unknown service type")
			// but continue
		}
	}

	return nil
}
