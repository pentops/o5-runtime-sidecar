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
	Endpoint   string `env:"PGPROXY_RDS_ENDPOINT"`
	ListenPort int    `env:"PGPROXY_PORT" default:"5432"`
	Name       string `flag:"name"`
	User       string `flag:"user"`
}) error {

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)

	}
	builder := pgproxy.NewCredBuilder(cfg.Credentials, cfg.Region)
	err = builder.AddConfig("default", &pgproxy.AuroraConfig{
		Endpoint: args.Endpoint,
		DbName:   args.Name,
		DbUser:   args.User,
	})
	if err != nil {
		return err
	}

	connector, err := pgproxy.NewAuroraConnector("default", builder)
	if err != nil {
		return err
	}

	listener, err := pgproxy.NewListener(fmt.Sprintf(":%d", args.ListenPort), map[string]pgproxy.PGConnector{
		"default": connector,
	})
	if err != nil {
		return err
	}

	return listener.Listen(ctx)
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

	builder := pgproxy.NewCredBuilder(cfg.Credentials, cfg.Region)
	err = builder.AddConfig("default", &pgproxy.AuroraConfig{
		Endpoint: args.Endpoint,
		DbName:   args.Name,
		DbUser:   args.User,
	})
	if err != nil {
		return err
	}

	url, err := builder.NewToken(ctx, "default")
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

	builder := pgproxy.NewCredBuilder(cfg.Credentials, cfg.Region)
	err = builder.AddConfig("default", &pgproxy.AuroraConfig{
		Endpoint: args.Endpoint,
		DbName:   args.Name,
		DbUser:   args.User,
	})
	if err != nil {
		return err
	}

	url, err := builder.NewToken(ctx, "default")
	if err != nil {
		return err
	}

	fmt.Println(url)
	return nil
}
