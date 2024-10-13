package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/cors"
	"github.com/stretchr/testify/assert"
)

func TestCORS(t *testing.T) {

	var handler http.Handler
	handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler = cors.New(cors.Options{
		AllowedOrigins:   []string{"https://*.example.com"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	}).Handler(handler)

	req := httptest.NewRequest("OPTIONS", "/test/v1/foo/idVal", nil)
	req.Header.Set("Origin", "https://sub.example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "X-Custom-Header")
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)
	rw.Flush()

	assert.Equal(t, http.StatusNoContent, rw.Code)
	assert.Equal(t, "https://sub.example.com", rw.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "GET", rw.Header().Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "X-Custom-Header", rw.Header().Get("Access-Control-Allow-Headers"))

}
