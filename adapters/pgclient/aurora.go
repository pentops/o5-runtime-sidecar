package pgclient

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/feature/rds/auth"
)

type AuroraConfig struct {
	Endpoint string `json:"endpoint"` // Address and Port
	Port     int    `json:"port"`
	DBName   string `json:"dbName"`
	DBUser   string `json:"dbUser"`
}

type auroraConnector struct {
	creds AuthClient
	name  string
}

func NewAuroraConnector(name string, creds AuthClient) (PGConnector, error) {
	return &auroraConnector{
		creds: creds,
		name:  name,
	}, nil
}

func (ac *auroraConnector) Name() string {
	return ac.name
}

func (ac *auroraConnector) DSN(ctx context.Context) (string, error) {
	return ac.creds.ConnectionString(ctx, ac.name)
}

type CredBuilder struct {
	//creds    aws.CredentialsProvider
	configs map[string]*AuroraConfig
	//region   string
	provider AWSProvider
}

// func NewCredBuilder(creds aws.CredentialsProvider, region string) *CredBuilder {
func NewCredBuilder(provider AWSProvider) *CredBuilder {
	cb := &CredBuilder{
		provider: provider,
		configs:  make(map[string]*AuroraConfig),
	}

	return cb
}

func fixConfig(config *AuroraConfig) error {
	parts := strings.Split(config.Endpoint, ":")
	if len(parts) == 1 {
		if config.Port == 0 {
			config.Port = 5432
		}
	} else if len(parts) == 2 {
		endpoint, portStr := parts[0], parts[1]

		portInt, err := strconv.Atoi(portStr)
		if err != nil {
			return fmt.Errorf("invalid endpoint %s: %w", config.Endpoint, err)
		}

		if config.Port == 0 {
			config.Port = portInt
		} else if config.Port != portInt {
			return fmt.Errorf("port in endpoint doesn't match specified port")
		}

		config.Endpoint = endpoint
	} else {
		return fmt.Errorf("invalid endpoint %s", config.Endpoint)
	}

	return nil
}

func (cb *CredBuilder) AddConfig(name string, config *AuroraConfig) error {
	if _, ok := cb.configs[name]; ok {
		return fmt.Errorf("config %q already exists", name)
	}

	if err := fixConfig(config); err != nil {
		return err
	}

	cb.configs[name] = config

	return nil
}

func (cb *CredBuilder) NewToken(ctx context.Context, lookupName string) (string, error) {
	config, ok := cb.configs[lookupName]
	if !ok {
		return "", fmt.Errorf("no config found for %q", lookupName)
	}

	return cb.newToken(ctx, config)
}

func (cb *CredBuilder) newToken(ctx context.Context, config *AuroraConfig) (string, error) {
	region := cb.provider.Region()
	creds, err := cb.provider.Credentials(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get aws credentials: %w", err)
	}

	authenticationToken, err := auth.BuildAuthToken(
		ctx, fmt.Sprintf("%s:%d", config.Endpoint, config.Port), region, config.DBUser, creds)
	if err != nil {
		return "", fmt.Errorf("failed to create authentication token: %w", err)
	}

	return authenticationToken, nil
}

func (cb *CredBuilder) ConnectionString(ctx context.Context, lookupName string) (string, error) {
	config, ok := cb.configs[lookupName]
	if !ok {
		return "", fmt.Errorf("no config found for %q", lookupName)
	}

	// TODO: Cache the token.

	token, err := cb.newToken(ctx, config)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("user=%s password=%s host=%s port=%d dbname=%s",
		config.DBUser,
		token,
		config.Endpoint,
		config.Port,
		config.DBName,
	), nil
}
