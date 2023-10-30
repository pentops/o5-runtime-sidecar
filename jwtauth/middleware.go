package jwtauth

import (
	"context"
	"net/http"
	"strings"

	"github.com/pentops/o5-runtime-sidecar/proxy"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gopkg.in/square/go-jose.v2"
)

type JWKS interface {
	GetKeys(keyID string) ([]jose.JSONWebKey, error)
}

func JWKSAuthFunc(jwks JWKS) proxy.AuthFunc {
	return func(ctx context.Context, req *http.Request) (map[string]string, error) {

		authHeader := req.Header.Get("Authorization")
		if authHeader == "" {
			return nil, status.Error(codes.Unauthenticated, "missing authorization header")
		}
		if !strings.HasPrefix(authHeader, "Bearer ") {
			return nil, status.Error(codes.Unauthenticated, "invalid authorization header")
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")

		sig, err := jose.ParseSigned(token)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, err.Error())
		}

		var keyID string
		// find the first signature with a key id
		for _, sig := range sig.Signatures {
			if sig.Header.KeyID != "" {
				keyID = sig.Header.KeyID
				break
			}
		}

		if keyID == "" {
			return nil, status.Error(codes.Unauthenticated, "missing key id")
		}

		keys, err := jwks.GetKeys(keyID)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, err.Error())
		}

		var verifiedBytes []byte
		for _, key := range keys {
			verifiedBytes, err = sig.Verify(key)
			if err == nil {
				break
			}
		}

		if verifiedBytes == nil {
			return nil, status.Error(codes.Unauthenticated, "invalid signature")
		}

		return map[string]string{
			"X-Verified-JWT": string(verifiedBytes),
		}, nil
	}
}
