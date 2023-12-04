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
	for {
		services, err := fetchServices(ctx, conn)
		if err == nil {
			log.WithField(ctx, "services", len(services)).Info("fetched services")
			return services, nil
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
}

func fetchServices(ctx context.Context, conn *grpc.ClientConn) ([]protoreflect.ServiceDescriptor, error) {

	client := grpc_reflection_v1.NewServerReflectionClient(conn)

	cc, err := client.ServerReflectionInfo(ctx)
	if err != nil {
		return nil, err
	}

	roundTrip := func(req *grpc_reflection_v1.ServerReflectionRequest) (*grpc_reflection_v1.ServerReflectionResponse, error) {
		if err := cc.Send(req); err != nil {
			return nil, err
		}
		return cc.Recv()
	}

	resp, err := roundTrip(&grpc_reflection_v1.ServerReflectionRequest{
		MessageRequest: &grpc_reflection_v1.ServerReflectionRequest_ListServices{},
	})
	if err != nil {
		return nil, err
	}

	ds := &descriptorpb.FileDescriptorSet{}
	serviceNames := make([]string, 0)

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
