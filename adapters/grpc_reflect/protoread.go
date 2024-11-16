package grpc_reflect

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pentops/j5/codec"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

type ReflectionClient struct {
	reflectionClient grpc_reflection_v1.ServerReflectionClient
	conn             *grpc.ClientConn
	codec            *codec.Codec

	files *protoregistry.Files

	lock sync.RWMutex
}

func NewClient(conn *grpc.ClientConn) *ReflectionClient {
	client := grpc_reflection_v1.NewServerReflectionClient(conn)
	rc := &ReflectionClient{
		reflectionClient: client,
		conn:             conn,
	}
	rc.codec = codec.NewCodec(codec.WithResolver(rc))
	return rc
}

func (cl *ReflectionClient) Invoke(ctx context.Context, method string, req interface{}, res interface{}, opts ...grpc.CallOption) error {
	return grpc.Invoke(ctx, method, req, res, cl.conn, opts...)
}

func (cl *ReflectionClient) JSONToProto(jsonData []byte, msg protoreflect.Message) error {
	return cl.codec.JSONToProto(jsonData, msg)
}

func (cl *ReflectionClient) ProtoToJSON(msg protoreflect.Message) ([]byte, error) {
	return cl.codec.ProtoToJSON(msg)
}

func (cl *ReflectionClient) FindDescriptorByName(name protoreflect.FullName) (protoreflect.Descriptor, error) {
	cl.lock.RLock()
	desc, err := cl.files.FindDescriptorByName(name)
	cl.lock.RUnlock()
	if err == nil {
		return desc, nil
	}
	if !errors.Is(err, protoregistry.NotFound) {
		return nil, err
	}

	cl.lock.Lock()
	defer cl.lock.Unlock()

	// double check in case another goroutine has already fetched it
	// now we have the write lock, so can resolve
	desc, err = cl.files.FindDescriptorByName(name)
	if err == nil {
		return desc, nil
	}

	desc, err = cl.fetchAndInclude(name)
	if err != nil {
		return nil, err
	}
	return desc, nil
}

func (cl *ReflectionClient) FindMessageByName(name protoreflect.FullName) (protoreflect.MessageType, error) {
	desc, err := cl.FindDescriptorByName(name)
	if err != nil {
		return nil, err
	}
	descMsg, ok := desc.(protoreflect.MessageDescriptor)
	if !ok {
		return nil, fmt.Errorf("type %s is not a message", name)
	}
	return dynamicpb.NewMessageType(descMsg), nil
}

func (cl *ReflectionClient) fetchAndInclude(name protoreflect.FullName) (protoreflect.Descriptor, error) {
	// sadly we don't have access to a context here...
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := newStream(ctx, cl.reflectionClient)
	if err != nil {
		return nil, err
	}
	defer stream.close()

	ff := newFetcher(ctx, stream, cl.files)
	return ff.FindDescriptorByName(name)

}

// FetchServices fetches the full reflection descriptor of all exposed services from a grpc server
func (cl *ReflectionClient) FetchServices(ctx context.Context, conn *grpc.ClientConn) ([]protoreflect.ServiceDescriptor, error) {

	stream, err := newStream(ctx, cl.reflectionClient)
	if err != nil {
		return nil, err
	}

	defer stream.close()

	serviceResp, err := stream.listServices(ctx)
	if err != nil {
		return nil, err
	}

	ds := &descriptorpb.FileDescriptorSet{}
	serviceNames := make([]string, 0)

	fileSet := make(map[string]struct{})

	servicesAlreadySeen := make(map[string]struct{})

	for _, service := range serviceResp.Service {
		// don't register the reflection service
		if strings.HasPrefix(service.Name, "grpc.reflection") {
			continue
		}

		serviceNames = append(serviceNames, service.Name)

		if _, ok := servicesAlreadySeen[service.Name]; ok {
			continue
		}

		fileResp, err := stream.fileContainingSymbol(ctx, service.Name)
		if err != nil {
			return nil, err
		}

		for _, rawFile := range fileResp.FileDescriptorProto {
			file := &descriptorpb.FileDescriptorProto{}
			if err := proto.Unmarshal(rawFile, file); err != nil {
				return nil, err
			}
			if _, ok := fileSet[file.GetName()]; ok {
				continue
			}
			fileSet[file.GetName()] = struct{}{}
			for _, serviceInFile := range file.Service {
				serviceName := fmt.Sprintf("%s.%s", file.GetPackage(), serviceInFile.GetName())
				servicesAlreadySeen[serviceName] = struct{}{}
			}

			ds.File = append(ds.File, file)
		}
	}

	files, err := protodesc.NewFiles(ds)
	if err != nil {
		return nil, err
	}

	cl.files = files

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
