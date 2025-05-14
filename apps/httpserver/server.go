package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/pentops/j5/lib/proxy"
	"github.com/pentops/jwtauth/jwks"
	"github.com/pentops/log.go/log"
	"github.com/pentops/o5-runtime-sidecar/apps/httpserver/jwtauth"
	"github.com/pentops/o5-runtime-sidecar/sidecar"
	"golang.org/x/sync/errgroup"

	"github.com/rs/cors"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type proxyRouter interface {
	RegisterGRPCService(ctx context.Context, service protoreflect.ServiceDescriptor, invoker proxy.AppConn) error
	ServeHTTP(http.ResponseWriter, *http.Request)
	SetGlobalAuth(proxy.AuthHeaders)
}

type ServerConfig struct {
	// Port to expose to the external LB
	PublicAddr  string   `env:"PUBLIC_ADDR" default:""`
	JWKS        []string `env:"JWKS" default:""`
	StaticFiles string   `env:"STATIC_FILES" default:""`
	CORSOrigins []string `env:"CORS_ORIGINS" default:""`
}

type AppDetail struct {
	SidecarVersion string
}

func NewRouter(config ServerConfig, app sidecar.AppInfo) (*Router, error) {
	router := proxy.NewRouter()

	router.SetHealthCheck("/healthz", func() error {
		return nil
	})

	router.AddMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Sidecar-Version", app.SidecarVersion)
			next.ServeHTTP(w, r)
		})
	})

	if len(config.CORSOrigins) > 0 {
		router.AddMiddleware(cors.New(cors.Options{
			AllowedOrigins:   config.CORSOrigins,
			AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
			AllowedHeaders:   []string{"*"},
			AllowCredentials: true,
		}).Handler)
	}

	if config.StaticFiles != "" {
		router.SetNotFoundHandler(http.FileServer(http.Dir(config.StaticFiles)))
	}

	routerServer := &Router{
		router:    router,
		addr:      config.PublicAddr,
		listening: make(chan struct{}),
	}

	if len(config.JWKS) > 0 {
		jwksManager := jwks.NewKeyManager()
		if err := jwksManager.AddSourceURLs(config.JWKS...); err != nil {
			return nil, fmt.Errorf("configuring JWKS: %w", err)
		}

		routerServer.jwks = jwksManager
		globalAuth := proxy.AuthHeadersFunc(jwtauth.JWKSAuthFunc(jwksManager))
		routerServer.router.SetGlobalAuth(globalAuth)
	}

	return routerServer, nil
}

type Router struct {
	addr      string
	listening chan struct{}
	router    proxyRouter
	jwks      *jwks.JWKSManager
}

func (hs *Router) RegisterService(ctx context.Context, service protoreflect.ServiceDescriptor, invoker proxy.AppConn) error {
	return hs.router.RegisterGRPCService(ctx, service, invoker)
}

func (hs *Router) Run(ctx context.Context) error {

	eg, ctx := errgroup.WithContext(ctx)

	if hs.jwks != nil {

		eg.Go(func() error {
			return hs.jwks.Run(ctx)
		})
	}

	lis, err := net.Listen("tcp", hs.addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	srv := http.Server{
		Handler: hs.router,
		Addr:    hs.addr,
	}

	hs.addr = lis.Addr().String()
	close(hs.listening)

	go func() {
		<-ctx.Done()
		if err := srv.Shutdown(ctx); err != nil {
			log.WithError(ctx, err).Error("Error shutting down server")
		}
	}()

	eg.Go(func() error {
		err = srv.Serve(lis)
		if err == nil {
			return nil
		}
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return nil
	})

	err = eg.Wait()

	return err
}

func (hs *Router) Addr() string {
	<-hs.listening
	return hs.addr
}
