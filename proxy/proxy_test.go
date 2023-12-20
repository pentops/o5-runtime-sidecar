package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/pentops/jsonapi/jsonapi"
	"github.com/pentops/o5-runtime-sidecar/testproto/gen/testpb"
	"github.com/rs/cors"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

var testCodecOptions = jsonapi.Options{
	ShortEnums: &jsonapi.ShortEnumsOption{
		UnspecifiedSuffix: "UNSPECIFIED",
		StrictUnmarshal:   true,
	},
	WrapOneof: true,
}

func TestCORS(t *testing.T) {
	ctx := context.Background()
	rr := NewRouter(testCodecOptions)

	rr.UseCORS(cors.Options{
		AllowedOrigins:   []string{"https://*.example.com"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	})

	if err := rr.RegisterService(ctx, testpb.File_test_v1_test_proto.Services().Get(0),
		TestInvoker[
			*testpb.GetFooRequest,
			*testpb.GetFooResponse,
		](func(req *testpb.GetFooRequest) (*testpb.GetFooResponse, error) {
			return &testpb.GetFooResponse{}, nil
		})); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("OPTIONS", "/test/v1/foo/idVal", nil)
	req.Header.Set("Origin", "https://sub.example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "X-Custom-Header")
	rw := httptest.NewRecorder()
	rr.ServeHTTP(rw, req)
	rw.Flush()

	assert.Equal(t, http.StatusNoContent, rw.Code)
	assert.Equal(t, "https://sub.example.com", rw.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "GET", rw.Header().Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "X-Custom-Header", rw.Header().Get("Access-Control-Allow-Headers"))

}

func TestGetHandlerMapping(t *testing.T) {

	fd := testpb.File_test_v1_test_proto.Services().Get(0).Methods().Get(0)

	rr := NewRouter(testCodecOptions)

	method, err := rr.buildMethod(fd, nil)
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

	t.Run("MessageQueryString", func(t *testing.T) {
		qs := url.Values{}
		qs.Set("query", `{"a":"aval","b":"bval"}`)
		req := httptest.NewRequest("GET", "/test/v1/foo/idVal?"+qs.Encode(), nil)
		reqToService := &testpb.GetFooRequest{}
		rw := roundTrip(method, req, reqToService, &testpb.GetFooResponse{})
		if rw.Code != 200 {
			t.Fatalf("expected status code 200, got %d: %s", rw.Code, rw.Body.String())
		}
		assert.Equal(t, "aval", reqToService.Query.A)
		assert.Equal(t, "bval", reqToService.Query.B)
	})

	t.Run("AltQueryString", func(t *testing.T) {
		qs := url.Values{}
		qs.Set("query.a", "aval")
		qs.Set("query.b", "bval")
		req := httptest.NewRequest("GET", "/test/v1/foo/idVal?"+qs.Encode(), nil)
		reqToService := &testpb.GetFooRequest{}
		rw := roundTrip(method, req, reqToService, &testpb.GetFooResponse{})
		if rw.Code != 200 {
			t.Fatalf("expected status code 200, got %d: %s", rw.Code, rw.Body.String())
		}
		assert.Equal(t, "aval", reqToService.Query.A)
		assert.Equal(t, "bval", reqToService.Query.B)
	})

}

func TestBodyHandlerMapping(t *testing.T) {

	fd := testpb.File_test_v1_test_proto.Services().Get(0).Methods().Get(1)

	rr := NewRouter(testCodecOptions)
	method, err := rr.buildMethod(fd, nil)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "/test/v1/foo", method.HTTPPath)
	assert.Equal(t, "POST", method.HTTPMethod)

	t.Run("Basic", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/test/v1/foo", strings.NewReader(`{"sString":"nameVal"}`))
		reqToService := &testpb.PostFooRequest{}
		rw := roundTrip(method, req, reqToService, &testpb.PostFooResponse{})
		if rw.Code != 200 {
			t.Fatalf("expected status code 200, got %d", rw.Code)
		}
		assert.Equal(t, "nameVal", reqToService.SString)
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
	})
}

type TestInvoker[REQ proto.Message, RES proto.Message] func(req REQ) (RES, error)

func (fn TestInvoker[REQ, RES]) Invoke(ctx context.Context, method string, protoReq, protoRes interface{}, opts ...grpc.CallOption) error {
	protoInvokeRequest, ok := protoReq.(REQ)
	if !ok {
		return fmt.Errorf("expected proto.Message, got %T", protoReq)
	}

	protoInvokeResponse, ok := protoRes.(RES)
	if !ok {
		return fmt.Errorf("expected proto.Message, got %T", protoRes)
	}

	gotRes, err := fn(protoInvokeRequest)
	if err != nil {
		return err
	}

	// Copy the passed in gRPC PRoto response to the HTTP Response mapper
	if err := protoCopy(protoInvokeResponse, gotRes); err != nil {
		return err
	}
	return nil
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
