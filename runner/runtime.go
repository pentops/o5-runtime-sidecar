package runner

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/pentops/jsonapi/jsonapi"
	"github.com/pentops/jwtauth/jwks"
	"github.com/pentops/log.go/log"
	"github.com/pentops/o5-runtime-sidecar/jwtauth"
	"github.com/pentops/o5-runtime-sidecar/outbox"
	"github.com/pentops/o5-runtime-sidecar/protoread"
	"github.com/pentops/o5-runtime-sidecar/proxy"
	"github.com/pentops/o5-runtime-sidecar/sqslink"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type Runtime struct {
	router     *proxy.Router
	Worker     *sqslink.Worker
	Sender     *outbox.SNSBatcher
	JWKS       *jwks.JWKSManager
	PublicPort int

	httpServices []protoreflect.ServiceDescriptor

	outboxURIs []string

	connections []io.Closer
	endpoints   []string
}

func NewRuntime() *Runtime {
	return &Runtime{
		httpServices: make([]protoreflect.ServiceDescriptor, 0),
	}
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
	eg, ctx := errgroup.WithContext(ctx)

	didAnything := false

	for _, uri := range rt.outboxURIs {
		didAnything = true
		uri := uri
		eg.Go(func() error {
			return outbox.Listen(ctx, uri, rt.Sender)
		})
	}

	for _, endpoint := range rt.endpoints {
		endpoint := endpoint
		if err := rt.registerEndpoint(ctx, endpoint); err != nil {
			return fmt.Errorf("register endpoint %s: %w", endpoint, err)
		}
	}

	if rt.router != nil {
		if rt.JWKS != nil {
			rt.router.AuthFunc = jwtauth.JWKSAuthFunc(rt.JWKS)
			// JWKS doesn't count as doint something without a router
			eg.Go(func() error {
				return rt.JWKS.Run(ctx)
			})
		}
		// TODO: CORS
		// TODO: Metrics

		didAnything = true

		srv := http.Server{
			Handler: rt.router,
			Addr:    fmt.Sprintf(":%d", rt.PublicPort),
		}

		if rt.JWKS != nil {
			// Wait for keys to be loaded before starting the server
			if err := rt.JWKS.WaitForKeys(ctx); err != nil {
				return fmt.Errorf("failed to load JWKS: %w", err)
			}
		}

		go func() {
			<-ctx.Done()
			srv.Shutdown(ctx) // nolint:errcheck
		}()

		eg.Go(func() error {
			return srv.ListenAndServe()
		})
	}

	if rt.Worker != nil {
		didAnything = true
		eg.Go(func() error {
			return rt.Worker.Run(ctx)
		})
	}

	if !didAnything {
		return fmt.Errorf("no services configured")
	}

	return eg.Wait()
}

func (rt *Runtime) AddRouter(port int, codecOptions jsonapi.Options) error {
	if rt.router != nil {
		return fmt.Errorf("router already configured")
	}

	rt.router = proxy.NewRouter(codecOptions)
	rt.PublicPort = port
	return nil
}

func (rt *Runtime) AddOutbox(ctx context.Context, outboxURI string) error {
	if rt.Sender == nil {
		return fmt.Errorf("outbox requires a sender")
	}

	rt.outboxURIs = append(rt.outboxURIs, outboxURI)
	return nil
}

func (rt *Runtime) StaticFiles(dirname string) error {
	if rt.router == nil {
		return fmt.Errorf("static files configured but no router")
	}

	rt.router.SetNotFoundHandler(http.FileServer(http.Dir(dirname)))
	return nil
}

func (rt *Runtime) AddJWKS(ctx context.Context, sources ...string) error {
	jwksManager := jwks.NewKeyManager()

	for _, source := range sources {
		log.WithField(ctx, "source", source).Info("Adding JWKS source URL")
	}

	if err := jwksManager.AddSourceURLs(sources...); err != nil {
		return err
	}

	rt.JWKS = jwksManager
	return nil
}

func (rt *Runtime) AddEndpoint(ctx context.Context, endpoint string) error {
	rt.endpoints = append(rt.endpoints, endpoint)
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
		case strings.HasSuffix(name, "Service"):
			if rt.router == nil {
				return fmt.Errorf("service %s requires a public port", name)
			}
			if err := rt.router.RegisterService(ss, conn); err != nil {
				return fmt.Errorf("register service %s: %w", name, err)
			}
			rt.httpServices = append(rt.httpServices, ss)
		case strings.HasSuffix(name, "Topic"):
			if rt.Worker == nil {
				return fmt.Errorf("topic %s requires an SQS URL", name)
			}
			if err := rt.Worker.RegisterService(ctx, ss, conn); err != nil {
				return fmt.Errorf("register worker %s: %w", name, err)
			}
		default:
			log.WithField(ctx, "service", name).Error("Unknown service type")
			// but continue
		}
	}

	return nil
}
