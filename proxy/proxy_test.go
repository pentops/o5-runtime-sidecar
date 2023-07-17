package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/pentops/o5-runtime-sidecar/testproto/gen/testpb"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

func TestGetHandlerMapping(t *testing.T) {

	fd := testpb.File_test_v1_test_proto.Services().Get(0).Methods().Get(0)

	rr := NewRouter()
	method, err := rr.buildMethod("foobar", fd, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "/test/v1/foo/{id}", method.HTTPPath)
	assert.Equal(t, "GET", method.HTTPMethod)

	t.Run("Basic", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test/v1/foo/idVal", nil)
		reqToService := &testpb.GetFooRequest{}
		rw := roundTrip(method, req, reqToService, &testpb.GetFooResponse{})
		if rw.Code != 200 {
			t.Fatalf("expected status code 200, got %d", rw.Code)
		}
		assert.Equal(t, "idVal", reqToService.Id)
	})

	t.Run("QueryString", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test/v1/foo/idVal?number=55", nil)
		reqToService := &testpb.GetFooRequest{}
		rw := roundTrip(method, req, reqToService, &testpb.GetFooResponse{})
		if rw.Code != 200 {
			t.Fatalf("expected status code 200, got %d", rw.Code)
		}
		assert.Equal(t, "idVal", reqToService.Id)
		assert.Equal(t, int64(55), reqToService.Number)
	})

	t.Run("RepeatedQueryString", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test/v1/foo/idVal?numbers=55&numbers=56", nil)
		reqToService := &testpb.GetFooRequest{}
		rw := roundTrip(method, req, reqToService, &testpb.GetFooResponse{})
		if rw.Code != 200 {
			t.Fatalf("expected status code 200, got %d", rw.Code)
		}
		assert.Equal(t, "idVal", reqToService.Id)
		assert.Len(t, reqToService.Numbers, 2)
		assert.Equal(t, float32(55), reqToService.Numbers[0])
		assert.Equal(t, float32(56), reqToService.Numbers[1])
	})

}

func TestBodyHandlerMapping(t *testing.T) {

	fd := testpb.File_test_v1_test_proto.Services().Get(0).Methods().Get(1)

	rr := NewRouter()
	method, err := rr.buildMethod("foobar", fd, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "/test/v1/foo", method.HTTPPath)
	assert.Equal(t, "POST", method.HTTPMethod)

	t.Run("Basic", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/test/v1/foo", strings.NewReader(`{"name":"nameVal"}`))
		reqToService := &testpb.PostFooRequest{}
		rw := roundTrip(method, req, reqToService, &testpb.PostFooResponse{})
		if rw.Code != 200 {
			t.Fatalf("expected status code 200, got %d", rw.Code)
		}
		assert.Equal(t, "nameVal", reqToService.Name)
	})

	t.Run("BadJSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/test/v1/foo", strings.NewReader(`foobar`))
		reqToService := &testpb.PostFooRequest{}
		rw := roundTrip(method, req, reqToService, &testpb.PostFooResponse{})
		if rw.Code != http.StatusBadRequest {
			t.Fatalf("expected BadRequest, got %d", rw.Code)
		}
		errResp := map[string]interface{}{}
		if err := json.Unmarshal(rw.Body.Bytes(), &errResp); err != nil {
			t.Fatal(err)
		}
		assert.Contains(t, errResp["error"], "foobar")
	})

}

func protoCopy(from, to proto.Message) error {
	fromBytes, err := proto.Marshal(from)
	if err != nil {
		return err
	}
	if err := proto.Unmarshal(fromBytes, to); err != nil {
		return err
	}
	return nil
}

func roundTrip(method *Method, req *http.Request, reqBody, resBody proto.Message) *httptest.ResponseRecorder {
	rw := &httptest.ResponseRecorder{
		Body: &bytes.Buffer{},
	}
	method.Invoker = InvokerFunc(func(ctx context.Context, method string, rawInvokeRequest, rawInvokeResponse interface{}, opts ...grpc.CallOption) error {
		protoInvokeRequest, ok := rawInvokeRequest.(proto.Message)
		if !ok {
			return fmt.Errorf("expected proto.Message, got %T", req)
		}
		// Copy the body, mapped from the HTTP Request, to the passed in gRPC Proto Request body
		if err := protoCopy(protoInvokeRequest, reqBody); err != nil {
			return err
		}

		protoInvokeResponse, ok := rawInvokeResponse.(proto.Message)
		if !ok {
			return fmt.Errorf("expected proto.Message, got %T", rawInvokeResponse)
		}
		// Copy the passed in gRPC PRoto response to the HTTP Response mapper
		if err := protoCopy(resBody, protoInvokeResponse); err != nil {
			return err
		}
		return nil
	})

	// This indirect call maps the path parameters to request context
	router := mux.NewRouter()
	router.Methods(method.HTTPMethod).Path(method.HTTPPath).Handler(method)
	router.ServeHTTP(rw, req)
	rw.Flush()

	return rw
}

type InvokerFunc func(ctx context.Context, method string, req, res interface{}, opts ...grpc.CallOption) error

func (f InvokerFunc) Invoke(ctx context.Context, method string, req, res interface{}, opts ...grpc.CallOption) error {
	return f(ctx, method, req, res, opts...)
}

type MockInvoker struct {
	GotRequest   []byte
	SendResponse []byte
}

func (m *MockInvoker) Invoke(ctx context.Context, method string, req, res interface{}, opts ...grpc.CallOption) error {
	protoReq, ok := req.(proto.Message)
	if !ok {
		return fmt.Errorf("expected proto.Message, got %T", req)
	}

	var err error

	m.GotRequest, err = proto.Marshal(protoReq)
	if err != nil {
		return err
	}

	protoRes, ok := res.(proto.Message)
	if !ok {
		return fmt.Errorf("expected proto.Message, got %T", res)
	}
	if err := proto.Unmarshal(m.SendResponse, protoRes); err != nil {
		return err
	}

	return nil
}
