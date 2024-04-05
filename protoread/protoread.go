package protoread

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pentops/log.go/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// FetchServices fetches the full reflection descriptor of all exposed services from a grpc server
func FetchServices(ctx context.Context, conn *grpc.ClientConn) ([]protoreflect.ServiceDescriptor, error) {
	return fetchServices(ctx, conn)
}

func fetchServices(ctx context.Context, conn *grpc.ClientConn) ([]protoreflect.ServiceDescriptor, error) {
	client := grpc_reflection_v1.NewServerReflectionClient(conn)

	var stream grpc_reflection_v1.ServerReflection_ServerReflectionInfoClient
	for {
		cc, err := client.ServerReflectionInfo(ctx)
		if err == nil {
			stream = cc
			break
		}

		log.WithError(ctx, err).Error("fetching services. Retrying")

		if errors.Is(err, context.Canceled) {
			return nil, err
		}

		select {
		case <-time.After(time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	roundTrip := func(req *grpc_reflection_v1.ServerReflectionRequest) (*grpc_reflection_v1.ServerReflectionResponse, error) {
		if err := stream.Send(req); err != nil {
			return nil, err
		}
		return stream.Recv()
	}

	resp, err := roundTrip(&grpc_reflection_v1.ServerReflectionRequest{
		MessageRequest: &grpc_reflection_v1.ServerReflectionRequest_ListServices{},
	})
	if err != nil {
		return nil, err
	}

	ds := &descriptorpb.FileDescriptorSet{}
	serviceNames := make([]string, 0)

	fileSet := make(map[string]struct{})

	for _, service := range resp.GetListServicesResponse().Service {
		// don't register the reflection service
		if strings.HasPrefix(service.Name, "grpc.reflection") {
			continue
		}

		serviceNames = append(serviceNames, service.Name)

		fileResp, err := roundTrip(&grpc_reflection_v1.ServerReflectionRequest{
			MessageRequest: &grpc_reflection_v1.ServerReflectionRequest_FileContainingSymbol{
				FileContainingSymbol: service.Name,
			},
		})
		if err != nil {
			return nil, err
		}

		for _, rawFile := range fileResp.GetFileDescriptorResponse().FileDescriptorProto {
			file := &descriptorpb.FileDescriptorProto{}
			if err := proto.Unmarshal(rawFile, file); err != nil {
				return nil, err
			}
			if _, ok := fileSet[file.GetName()]; ok {
				continue
			}
			fileSet[file.GetName()] = struct{}{}
			ds.File = append(ds.File, file)
		}
	}

	files, err := protodesc.NewFiles(ds)
	if err != nil {
		return nil, err
	}

	services := make([]protoreflect.ServiceDescriptor, 0, len(serviceNames))

	for _, serviceName := range serviceNames {

		ssI, err := files.FindDescriptorByName(protoreflect.FullName(serviceName))
		if err != nil {
			return nil, err
		}

		ss, ok := ssI.(protoreflect.ServiceDescriptor)
		if !ok {
			return nil, fmt.Errorf("not a service")
		}

		services = append(services, ss)
	}

	return services, nil
}
