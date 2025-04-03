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
	"github.com/pentops/j5/gen/j5/auth/v1/auth_j5pb"
	"github.com/pentops/j5/gen/test/foo/v1/foo_testspb"
	codec "github.com/pentops/j5/lib/j5codec"
	"github.com/stretchr/testify/assert"
	"google.golang.org/genproto/googleapis/api/httpbody"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestGetHandlerMapping(t *testing.T) {

	serviceDesc := foo_testspb.File_test_foo_v1_service_foo_service_proto.
		Services().ByName("FooQueryService")

	rr := NewRouter()
	rr.globalAuth = AuthHeadersFunc(func(ctx context.Context, req *http.Request) (map[string]string, error) {
		return map[string]string{}, nil
	})

	method, err := rr.buildMethod(serviceDesc.Methods().ByName("GetFoo"), nil, &auth_j5pb.MethodAuthType_None{})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "/test/v1/foo/{id}", method.HTTPPath)
	assert.Equal(t, "GET", method.HTTPMethod)

	listMethod, err := rr.buildMethod(serviceDesc.Methods().ByName("ListFoos"), nil, &auth_j5pb.MethodAuthType_None{})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Basic", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test/v1/foo/idVal", nil)
		reqToService := &foo_testspb.GetFooRequest{}
		rw := roundTrip(method, req, reqToService, &foo_testspb.GetFooResponse{})
		if rw.Code != 200 {
			t.Fatalf("expected status code 200, got %d", rw.Code)
		}
		assert.Equal(t, "idVal", reqToService.Id)
	})

	t.Run("QueryString", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test/v1/foo/idVal?number=55", nil)
		reqToService := &foo_testspb.GetFooRequest{}
		rw := roundTrip(method, req, reqToService, &foo_testspb.GetFooResponse{})
		if rw.Code != 200 {
			t.Fatalf("expected status code 200, got %d", rw.Code)
		}
		assert.Equal(t, "idVal", reqToService.Id)
		assert.Equal(t, int64(55), reqToService.Number)
	})

	t.Run("RepeatedQueryString", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test/v1/foo/idVal?numbers=55&numbers=56", nil)
		reqToService := &foo_testspb.GetFooRequest{}
		rw := roundTrip(method, req, reqToService, &foo_testspb.GetFooResponse{})
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
		qs.Set("ab", `{"a":"aval","b":"bval"}`)
		req := httptest.NewRequest("GET", "/test/v1/foo/idVal?"+qs.Encode(), nil)
		reqToService := &foo_testspb.GetFooRequest{}
		rw := roundTrip(method, req, reqToService, &foo_testspb.GetFooResponse{})
		if rw.Code != 200 {
			t.Fatalf("expected status code 200, got %d: %s", rw.Code, rw.Body.String())
		}
		assert.Equal(t, "aval", reqToService.Ab.A)
		assert.Equal(t, "bval", reqToService.Ab.B)
	})

	t.Run("AltQueryString", func(t *testing.T) {
		qs := url.Values{}
		qs.Set("ab.a", "aval")
		qs.Set("ab.b", "bval")
		req := httptest.NewRequest("GET", "/test/v1/foo/idVal?"+qs.Encode(), nil)
		reqToService := &foo_testspb.GetFooRequest{}
		rw := roundTrip(method, req, reqToService, &foo_testspb.GetFooResponse{})
		if rw.Code != 200 {
			t.Fatalf("expected status code 200, got %d: %s", rw.Code, rw.Body.String())
		}
		assert.Equal(t, "aval", reqToService.Ab.A)
		assert.Equal(t, "bval", reqToService.Ab.B)
	})

	t.Run("lower_snake query", func(t *testing.T) {
		qs := url.Values{}
		qs.Set("multiple_word", "aval")
		req := httptest.NewRequest("GET", "/test/v1/foo/idVal?"+qs.Encode(), nil)
		reqToService := &foo_testspb.GetFooRequest{}
		rw := roundTrip(method, req, reqToService, &foo_testspb.GetFooResponse{})
		if rw.Code != 200 {
			t.Fatalf("expected status code 200, got %d: %s", rw.Code, rw.Body.String())
		}
		assert.Equal(t, "aval", reqToService.MultipleWord)
	})

	t.Run("camelCase query", func(t *testing.T) {
		qs := url.Values{}
		qs.Set("multipleWord", "aval")
		req := httptest.NewRequest("GET", "/test/v1/foo/idVal?"+qs.Encode(), nil)
		reqToService := &foo_testspb.GetFooRequest{}
		rw := roundTrip(method, req, reqToService, &foo_testspb.GetFooResponse{})
		if rw.Code != 200 {
			t.Fatalf("expected status code 200, got %d: %s", rw.Code, rw.Body.String())
		}
		assert.Equal(t, "aval", reqToService.MultipleWord)
	})

	t.Run("Protostate Query Message query string", func(t *testing.T) {
		qs := url.Values{}
		qs.Set("query", `{"filters":[{"field":{"name":"idVal","type":{"value":"f481d62c-72ff-487b-ba03-50a4a6da83b7"}}}]}`)
		req := httptest.NewRequest("GET", "/test/v1/foos?"+qs.Encode(), nil)
		reqToService := &foo_testspb.ListFoosRequest{}
		rw := roundTrip(listMethod, req, reqToService, &foo_testspb.GetFooResponse{})
		if rw.Code != 200 {
			t.Fatalf("expected status code 200, got %d: %s", rw.Code, rw.Body.String())
		}

		assert.Equal(t, 1, len(reqToService.Query.Filters))
		assert.Equal(t, "idVal", reqToService.Query.Filters[0].GetField().Name)
		assert.Equal(t, "f481d62c-72ff-487b-ba03-50a4a6da83b7", reqToService.Query.Filters[0].GetField().Type.GetValue())
	})
}

func TestBodyHandlerMapping(t *testing.T) {

	fd := foo_testspb.File_test_foo_v1_service_foo_service_proto.
		Services().ByName("FooCommandService").
		Methods().ByName("PostFoo")

	rr := NewRouter()
	method, err := rr.buildMethod(fd, nil, &auth_j5pb.MethodAuthType_None{})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "/test/v1/foo", method.HTTPPath)
	assert.Equal(t, "POST", method.HTTPMethod)

	t.Run("Basic", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/test/v1/foo", strings.NewReader(`{"id":"nameVal"}`))
		reqToService := &foo_testspb.PostFooRequest{}
		rw := roundTrip(method, req, reqToService, &foo_testspb.PostFooResponse{})
		if rw.Code != 200 {
			t.Fatalf("expected status code 200, got %d", rw.Code)
		}
		assert.Equal(t, "nameVal", reqToService.Id)
	})

	t.Run("BadJSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/test/v1/foo", strings.NewReader(`foobar`))
		reqToService := &foo_testspb.PostFooRequest{}
		rw := roundTrip(method, req, reqToService, &foo_testspb.PostFooResponse{})
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

func roundTrip(method *grpcMethod, req *http.Request, reqBody, resBody proto.Message) *httptest.ResponseRecorder {
	rw := &httptest.ResponseRecorder{
		Body: &bytes.Buffer{},
	}
	method.AppCon = &testInvoker{
		codec: codec.NewCodec(),
		invoke: func(ctx context.Context, method string, rawInvokeRequest, rawInvokeResponse interface{}, opts ...grpc.CallOption) error {
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
		},
	}

	// This indirect call maps the path parameters to request context
	router := mux.NewRouter()
	router.Methods(method.HTTPMethod).Path(method.HTTPPath).Handler(method)
	router.ServeHTTP(rw, req)
	rw.Flush()

	return rw
}

type testInvoker struct {
	invoke func(ctx context.Context, method string, req, res interface{}, opts ...grpc.CallOption) error
	codec  *codec.Codec
}

func (f *testInvoker) Invoke(ctx context.Context, method string, req, res interface{}, opts ...grpc.CallOption) error {
	return f.invoke(ctx, method, req, res, opts...)
}

func (f *testInvoker) JSONToProto(jsonData []byte, msg protoreflect.Message) error {
	return f.codec.JSONToProto(jsonData, msg)
}

func (f *testInvoker) QueryToProto(query url.Values, msg protoreflect.Message) error {
	return f.codec.QueryToProto(query, msg)
}

func (f *testInvoker) ProtoToJSON(msg protoreflect.Message) ([]byte, error) {
	return f.codec.ProtoToJSON(msg)
}

type MockInvoker struct {
	GotRequest   []byte
	SendResponse []byte
	*codec.Codec
}

func (m *MockInvoker) SetResponse(t testing.TB, msg proto.Message) {
	dd, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	m.SendResponse = dd
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

func TestRawBodyHandler(t *testing.T) {
	fd := foo_testspb.File_test_foo_v1_service_foo_service_proto.
		Services().ByName("FooDownloadService")

	rr := NewRouter()

	bodyData, err := proto.Marshal(&httpbody.HttpBody{
		ContentType: "application/octet-stream",
		Data:        []byte("foobar"),
	})
	if err != nil {
		t.Fatal(err)
	}
	invoker := &MockInvoker{
		SendResponse: bodyData,
		Codec:        codec.NewCodec(),
	}
	if err := rr.RegisterGRPCService(context.Background(), fd, invoker); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	rr.ServeHTTP(rec, httptest.NewRequest("GET", "/test/v1/foo/id/raw", nil))

	if rec.Code != 200 {
		t.Fatalf("expected status code 200, got %d", rec.Code)
	}

}

func TestAuthMethods(t *testing.T) {

	services := foo_testspb.File_test_foo_v1_service_foo_service_proto.
		Services()

	authHeaders := map[string]string{}

	called := false
	rr := NewRouter()
	rr.globalAuth = AuthHeadersFunc(func(ctx context.Context, req *http.Request) (map[string]string, error) {
		called = true
		return authHeaders, nil
	})

	bodyData, err := proto.Marshal(&httpbody.HttpBody{
		ContentType: "application/octet-stream",
		Data:        []byte("foobar"),
	})
	if err != nil {
		t.Fatal(err)
	}
	invoker := &MockInvoker{
		SendResponse: bodyData,
		Codec:        codec.NewCodec(),
	}
	for idx := 0; idx < services.Len(); idx++ {
		sd := services.Get(idx)
		if err := rr.RegisterGRPCService(context.Background(), sd, invoker); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("Basic No Auth OK", func(t *testing.T) {
		called = false
		invoker.SetResponse(t, &foo_testspb.GetFooResponse{})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/test/v1/foo/idVal", nil)
		rr.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("expected status code 200, got %d", rec.Code)
		}
		if called {
			t.Fatal("get foo called auth headers func")
		}
	})

	t.Run("Basic No Auth OK", func(t *testing.T) {
		called = false
		invoker.SetResponse(t, &foo_testspb.GetFooResponse{})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/test/v1/foos", nil)
		rr.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("expected status code 200, got %d", rec.Code)
		}
		if !called {
			t.Fatal("expected auth headers call")
		}

	})

}
