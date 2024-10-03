package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/pentops/o5-runtime-sidecar/pgproxy"
	"github.com/pentops/runner/commander"
)

var Version string

func main() {

	cmdGroup := commander.NewCommandSet()

	cmdGroup.Add("pgproxy", commander.NewCommand(runPgProxy))
	cmdGroup.Add("pgurl", commander.NewCommand(runPgUrl))
	cmdGroup.Add("pgtoken", commander.NewCommand(runPgToken))

	cmdGroup.RunMain("o5-runtime-cli", Version)
}

func runPgProxy(ctx context.Context, args struct {
	Endpoint string `env:"PGPROXY_RDS_ENDPOINT"`
	Port     int    `env:"PGPROXY_PORT" default:"5432"`
}) error {

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	builder, err := pgproxy.NewCredBuilder(cfg.Credentials, args.Endpoint, cfg.Region)
	if err != nil {
		return err
	}

	listener, err := pgproxy.NewListener(builder)
	if err != nil {
		return err
	}

	return listener.Listen(ctx, fmt.Sprintf(":%d", args.Port))
}

func runPgUrl(ctx context.Context, args struct {
	Endpoint string `env:"PGPROXY_RDS_ENDPOINT"`
	Name     string `flag:"name"`
	User     string `flag:"user"`
}) error {

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	builder, err := pgproxy.NewCredBuilder(cfg.Credentials, args.Endpoint, cfg.Region)
	if err != nil {
		return err
	}

	url, err := builder.NewConnectionString(ctx, args.Name, args.User)
	if err != nil {
		return err
	}

	fmt.Println(url)
	return nil
}

func runPgToken(ctx context.Context, args struct {
	Endpoint string `env:"PGPROXY_RDS_ENDPOINT"`
	Name     string `flag:"name"`
	User     string `flag:"user"`
}) error {

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	builder, err := pgproxy.NewCredBuilder(cfg.Credentials, args.Endpoint, cfg.Region)
	if err != nil {
		return err
	}

	url, err := builder.NewToken(ctx, args.User)
	if err != nil {
		return err
	}

	fmt.Println(url)
	return nil
}
