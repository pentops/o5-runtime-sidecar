package testproto

import (
	"fmt"
	"testing"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

func BuildHTTPMethod(name string, rule *annotations.HttpRule) *descriptorpb.MethodDescriptorProto {
	mm := &descriptorpb.MethodDescriptorProto{
		Name:       proto.String(name),
		InputType:  proto.String(fmt.Sprintf("%sRequest", name)),
		OutputType: proto.String(fmt.Sprintf("%sResponse", name)),
		Options:    &descriptorpb.MethodOptions{},
	}
	proto.SetExtension(mm.Options, annotations.E_Http, rule)
	return mm
}

func FilesToServiceDescriptors(t testing.TB, fileDescriptors ...*descriptorpb.FileDescriptorProto) []protoreflect.ServiceDescriptor {
	t.Helper()

	files, err := protodesc.NewFiles(&descriptorpb.FileDescriptorSet{
		File: fileDescriptors,
	})
	if err != nil {
		t.Fatal(err)
	}

	services := make([]protoreflect.ServiceDescriptor, 0)
	for _, file := range fileDescriptors {
		for _, service := range file.Service {
			name := fmt.Sprintf("%s.%s", *file.Package, *service.Name)
			sd, err := files.FindDescriptorByName(protoreflect.FullName(name))
			if err != nil {
				t.Fatalf("finding service %s: %s", name, err)
			}

			services = append(services, sd.(protoreflect.ServiceDescriptor))
		}
	}
	return services
}
