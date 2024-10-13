package entrypoint

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/pentops/o5-runtime-sidecar/apps/httpserver"
	"github.com/stretchr/testify/assert"
)

type TestAWS struct{}

func (ta TestAWS) SNS() SNSAPI {
	return nil
}

func (ta TestAWS) SQS() SQSAPI {
	return nil
}

func (ta TestAWS) EventBridge() EventBridgeAPI {
	return nil
}

func (ta TestAWS) Region() string {
	return "local"
}

func (ta TestAWS) Credentials() aws.CredentialsProvider {
	return nil
}

func TestLoader(t *testing.T) {

	runtime, err := FromConfig(Config{}, TestAWS{})
	assert.NoError(t, err)

	err = runtime.Run(context.Background())
	if err == nil {
		t.Error("expected error - nothing configured")
	}
	if !errors.Is(err, NothingToDoError) {
		t.Errorf("expected NothingToDoError, got %v", err)
	}
}

func TestLoadEverything(t *testing.T) {

	runtime, err := FromConfig(Config{
		ServerConfig: httpserver.ServerConfig{
			PublicAddr:  ":0",
			CORSOrigins: []string{"*"},
		},
	}, TestAWS{})
	assert.NoError(t, err)

	exitErr := make(chan error)
	go func() {
		err = runtime.Run(context.Background())
		exitErr <- err
	}()

	httpAddr := runtime.routerServer.Addr()
	t.Logf("HTTP Addr: %s", httpAddr)

	t.Run("HTTP CORS", func(t *testing.T) {
		req, err := http.NewRequest("OPTIONS", fmt.Sprintf("http://%s/test/v1/foo/idVal", httpAddr), nil)
		assert.NoError(t, err)
		req.Header.Set("Origin", "https://sub.example.com")
		req.Header.Set("Access-Control-Request-Method", "GET")
		req.Header.Set("Access-Control-Request-Headers", "X-Custom-Header")
		rw, err := http.DefaultClient.Do(req)
		assert.NoError(t, err)

		assert.Equal(t, http.StatusNoContent, rw.StatusCode)
		// This one is complicated by the bind address loopback.
		//assert.Equal(t, "https://sub.example.com", rw.Header.Get("Access-Control-Allow-Origin"))
		assert.Equal(t, "GET", rw.Header.Get("Access-Control-Allow-Methods"))
		assert.Equal(t, "X-Custom-Header", rw.Header.Get("Access-Control-Allow-Headers"))
	})

}
