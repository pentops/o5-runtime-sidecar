package entrypoint

import (
	"context"
	"net/http"
	"testing"

	"github.com/pentops/flowtest/prototest"
	"github.com/pentops/j5/proxy"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type testRouter struct {
	registered []proxy.GRPCMethodConfig
}

func (tr *testRouter) RegisterGRPCMethod(ctx context.Context, config proxy.GRPCMethodConfig) error {
	tr.registered = append(tr.registered, config)
	return nil
}

func (tr *testRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {}

func getRegisteredService(t *testing.T, service protoreflect.ServiceDescriptor, mods ...func(*routerServer)) proxy.GRPCMethodConfig {
	mock := &testRouter{}
	server := newRouterServer(":0", mock)
	for _, mod := range mods {
		mod(server)
	}

	ctx := context.Background()
	if err := server.RegisterService(ctx, service, nil); err != nil {
		t.Fatal(err)
	}

	if len(mock.registered) != 1 {
		t.Fatalf("expected 1 method, got %d", len(mock.registered))
	}

	foo := mock.registered[0]
	return foo
}

var fooAuth = proxy.AuthHeadersFunc(func(ctx context.Context, req *http.Request) (map[string]string, error) {
	return map[string]string{
		"foo": "bar",
	}, nil
})

func assertHasFooAuth(t *testing.T, foo proxy.GRPCMethodConfig) {
	if foo.AuthHeaders == nil {
		t.Fatal("expected auth headers to be set")
	}

	got, _ := foo.AuthHeaders.AuthHeaders(context.Background(), nil)
	if got["foo"] != "bar" {
		t.Errorf("expected foo to be bar, got %s", got["foo"])
	}
}

func TestRouterNoAuth(t *testing.T) {
	descriptors := prototest.DescriptorsFromSource(t, map[string]string{
		"test.proto": `
		package test;

		service Test {
			rpc Foo (FooRequest) returns (FooResponse);
		}

		message FooRequest {}

		message FooResponse {}
		`,
	})

	service := descriptors.ServiceByName(t, "test.Test")

	t.Run("no global auth", func(t *testing.T) {
		foo := getRegisteredService(t, service)
		if foo.Method.FullName() != "test.Test.Foo" {
			t.Errorf("expected method to be test.Test, got %s", foo.Method.FullName())
		}
		if foo.AuthHeaders != nil {
			t.Errorf("expected auth headers to be nil, got %v", foo.AuthHeaders)
		}
	})

	t.Run("global auth", func(t *testing.T) {
		foo := getRegisteredService(t, service, func(server *routerServer) {
			server.globalAuth = fooAuth
		})

		assertHasFooAuth(t, foo)
	})
}

func TestRouterExplicitlyNoAuth(t *testing.T) {
	t.Skip("Currently broken")
	descriptors := prototest.DescriptorsFromSource(t, map[string]string{
		"test.proto": `
		package test;
		import "o5/auth/v1/annotations.proto";

		service Test {
			rpc Foo (FooRequest) returns (FooResponse) {
				option (o5.auth.v1.auth).none = {};
			};
		}

		message FooRequest {}

		message FooResponse {}
		`,
	})

	service := descriptors.ServiceByName(t, "test.Test")

	t.Run("no global auth", func(t *testing.T) {
		foo := getRegisteredService(t, service)
		if foo.Method.FullName() != "test.Test.Foo" {
			t.Errorf("expected method to be test.Test, got %s", foo.Method.FullName())
		}
		if foo.AuthHeaders != nil {
			t.Errorf("expected auth headers to be nil, got %v", foo.AuthHeaders)
		}
	})

	t.Run("global auth", func(t *testing.T) {
		foo := getRegisteredService(t, service, func(server *routerServer) {
			server.globalAuth = fooAuth
		})

		if foo.AuthHeaders != nil {
			t.Fatal("expected auth headers to be nil")
		}
	})
}

func TestRouterExplicitlyJWT(t *testing.T) {
	t.Skip("Currently broken")
	descriptors := prototest.DescriptorsFromSource(t, map[string]string{
		"test.proto": `
		package test;
		import "o5/auth/v1/annotations.proto";

		service Test {
			rpc Foo (FooRequest) returns (FooResponse) {
				option (o5.auth.v1.auth).jwt_bearer = {};
			};
		}

		message FooRequest {}

		message FooResponse {}
		`,
	})

	service := descriptors.ServiceByName(t, "test.Test")

	t.Run("no global auth", func(t *testing.T) {
		mock := &testRouter{}
		server := newRouterServer(":0", mock)

		ctx := context.Background()
		err := server.RegisterService(ctx, service, nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("global auth", func(t *testing.T) {
		foo := getRegisteredService(t, service, func(server *routerServer) {
			server.globalAuth = fooAuth
		})

		assertHasFooAuth(t, foo)
	})
}
func TestRouterExplicitlyJWTService(t *testing.T) {
	t.Skip("Currently broken")
	descriptors := prototest.DescriptorsFromSource(t, map[string]string{
		"test.proto": `
		package test;
		import "o5/auth/v1/annotations.proto";

		service Test {
			option (o5.auth.v1.default_auth).jwt_bearer = {};
			rpc Foo (FooRequest) returns (FooResponse) {};
		}

		message FooRequest {}

		message FooResponse {}
		`,
	})

	service := descriptors.ServiceByName(t, "test.Test")

	t.Run("no global auth", func(t *testing.T) {
		mock := &testRouter{}
		server := newRouterServer(":0", mock)

		ctx := context.Background()
		err := server.RegisterService(ctx, service, nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("global auth", func(t *testing.T) {
		foo := getRegisteredService(t, service, func(server *routerServer) {
			server.globalAuth = fooAuth
		})

		assertHasFooAuth(t, foo)
	})
}
