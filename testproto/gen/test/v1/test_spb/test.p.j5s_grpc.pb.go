// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.4.0
// - protoc             (unknown)
// source: test/v1/service/test.p.j5s.proto

package test_spb

import (
	context "context"

	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.62.0 or later.
const _ = grpc.SupportPackageIsVersion8

const (
	FooService_GetFoo_FullMethodName = "/test.v1.service.FooService/GetFoo"
)

// FooServiceClient is the client API for FooService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type FooServiceClient interface {
	GetFoo(ctx context.Context, in *GetFooRequest, opts ...grpc.CallOption) (*GetFooResponse, error)
}

type fooServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewFooServiceClient(cc grpc.ClientConnInterface) FooServiceClient {
	return &fooServiceClient{cc}
}

func (c *fooServiceClient) GetFoo(ctx context.Context, in *GetFooRequest, opts ...grpc.CallOption) (*GetFooResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetFooResponse)
	err := c.cc.Invoke(ctx, FooService_GetFoo_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// FooServiceServer is the server API for FooService service.
// All implementations must embed UnimplementedFooServiceServer
// for forward compatibility
type FooServiceServer interface {
	GetFoo(context.Context, *GetFooRequest) (*GetFooResponse, error)
	mustEmbedUnimplementedFooServiceServer()
}

// UnimplementedFooServiceServer must be embedded to have forward compatible implementations.
type UnimplementedFooServiceServer struct {
}

func (UnimplementedFooServiceServer) GetFoo(context.Context, *GetFooRequest) (*GetFooResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetFoo not implemented")
}
func (UnimplementedFooServiceServer) mustEmbedUnimplementedFooServiceServer() {}

// UnsafeFooServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to FooServiceServer will
// result in compilation errors.
type UnsafeFooServiceServer interface {
	mustEmbedUnimplementedFooServiceServer()
}

func RegisterFooServiceServer(s grpc.ServiceRegistrar, srv FooServiceServer) {
	s.RegisterService(&FooService_ServiceDesc, srv)
}

func _FooService_GetFoo_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetFooRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FooServiceServer).GetFoo(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: FooService_GetFoo_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FooServiceServer).GetFoo(ctx, req.(*GetFooRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// FooService_ServiceDesc is the grpc.ServiceDesc for FooService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var FooService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "test.v1.service.FooService",
	HandlerType: (*FooServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetFoo",
			Handler:    _FooService_GetFoo_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "test/v1/service/test.p.j5s.proto",
}
