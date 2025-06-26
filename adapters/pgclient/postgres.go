package pgclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/iancoleman/strcase"
)

type PGConnector interface {
	Name() string
	DSN(context.Context) (string, error)
}

type ConfigSet interface {
	GetConnector(raw string) (PGConnector, error)
}

func looksLikeJSONString(name string) bool {
	return strings.HasPrefix(name, "{") && strings.HasSuffix(name, "}")
}

var reValidDBEnvName = regexp.MustCompile(`^[A-Z0-9_]+$`)

type AWSProvider interface {
	Credentials() aws.CredentialsProvider
	Region() string
}

type pgConnSet struct {
	configs     ConfigProvider
	awsProvider AWSProvider
	credBuilder *CredBuilder
	connectors  map[string]PGConnector
}

type ConfigProvider interface {
	GetConfig(name string) (string, bool)
}

type EnvProvider struct{}

func (EnvProvider) GetConfig(name string) (string, bool) {
	envVarName := "DB_CREDS_" + strcase.ToScreamingSnake(name)
	// the Name passed in should be just the DB name with matching env var
	envCreds := os.Getenv(envVarName)
	if envCreds == "" {
		return "", false
	}

	return envCreds, true
}

func NewConnectorSet(awsProvider AWSProvider, configs ConfigProvider) *pgConnSet {
	return &pgConnSet{
		configs:     configs,
		awsProvider: awsProvider,
		connectors:  make(map[string]PGConnector),
	}
}

func (ss *pgConnSet) direct(name, raw string) (PGConnector, error) {
	if conn, ok := ss.connectors[name]; ok {
		return conn, nil
	}

	conn := NewDirectConnector(name, raw)
	ss.connectors[name] = conn

	return conn, nil
}

func (ss *pgConnSet) aurora(name string, config *AuroraConfig) (PGConnector, error) {
	if conn, ok := ss.connectors[name]; ok {
		return conn, nil
	}

	if ss.credBuilder == nil {
		ss.credBuilder = NewCredBuilder(ss.awsProvider.Credentials(), ss.awsProvider.Region())
	}

	if err := ss.credBuilder.AddConfig(name, config); err != nil {
		return nil, fmt.Errorf("adding config for %q: %w", name, err)
	}

	conn, err := NewAuroraConnector(name, ss.credBuilder)
	if err != nil {
		return nil, err
	}

	ss.connectors[name] = conn

	return conn, nil
}

func (ss *pgConnSet) GetConnector(raw string) (PGConnector, error) {
	name, ok, err := tryParsePGString(raw)
	if err != nil {
		return nil, fmt.Errorf("parsing postgres string: %w", err)
	}

	if ok {
		// Credentials were passed directly in the env, use as-is
		return ss.direct(name, raw)
	}

	if !reValidDBEnvName.MatchString(raw) {
		return nil, fmt.Errorf("invalid DB ref/name: %q", raw)
	}

	envVarName := "DB_CREDS_" + strcase.ToScreamingSnake(raw)
	dbName := strcase.ToSnake(raw)

	// the Name passed in should be just the DB name with matching env var
	envCreds := os.Getenv(envVarName)

	if envCreds == "" {
		// safe to log since the regex makes it very hard to store a password...
		return nil, fmt.Errorf("no credentials found - expecting $%s", envVarName)
	}

	_, ok, err = tryParsePGString(envCreds)
	if err != nil {
		return nil, fmt.Errorf("parsing postgres string from $%s: %w", envVarName, err)
	}

	if ok {
		// use the raw name as the connection name, but the parsed DSN
		return ss.direct(dbName, envCreds)
	}

	if !looksLikeJSONString(envCreds) {
		return nil, fmt.Errorf("invalid DB credentials in $%s, expecing a DSN or JSON value", envVarName)
	}

	config := &AuroraConfig{}
	if err := json.Unmarshal([]byte(envCreds), config); err != nil {
		return nil, fmt.Errorf("invalid JSON in $%s: %w", envVarName, err)
	}

	return ss.aurora(dbName, config)
}

func tryParsePGString(name string) (string, bool, error) {
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
		parts := strings.SplitSeq(name, " ")
		for part := range parts {
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
