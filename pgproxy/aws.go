package pgproxy

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/rds/auth"
)

type CredBuilder struct {
	creds    aws.CredentialsProvider
	endpoint string
	port     string
	region   string
}

func NewCredBuilder(creds aws.CredentialsProvider, endpoint string, region string) (*CredBuilder, error) {
	cb := &CredBuilder{
		creds:  creds,
		region: region,
	}
	parts := strings.Split(endpoint, ":")
	if len(parts) == 1 {
		cb.endpoint = endpoint
		cb.port = "5432"
	} else if len(parts) == 2 {
		cb.endpoint = parts[0]
		cb.port = parts[1]
	} else {
		return nil, fmt.Errorf("invalid endpoint")
	}

	return cb, nil
}

func (cb *CredBuilder) NewToken(ctx context.Context, userName string) (string, error) {

	authenticationToken, err := auth.BuildAuthToken(
		ctx, fmt.Sprintf("%s:%s", cb.endpoint, cb.port), cb.region, userName, cb.creds)
	if err != nil {
		return "", fmt.Errorf("failed to create authentication token: %w", err)
	}
	return authenticationToken, nil
}

func (cb *CredBuilder) NewConnectionString(ctx context.Context, dbName, userName string) (string, error) {
	token, err := cb.NewToken(ctx, userName)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("user=%s password=%s host=%s port=%s dbname=%s",
		userName,
		token,
		cb.endpoint,
		cb.port,
		dbName), nil

}
