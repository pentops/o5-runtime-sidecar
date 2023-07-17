package swagger

import (
	"encoding/json"
	"fmt"
	"testing"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

func buildHTTPMethod(name string, rule *annotations.HttpRule) *descriptorpb.MethodDescriptorProto {

	mm := &descriptorpb.MethodDescriptorProto{
		Name:       proto.String(name),
		InputType:  proto.String(fmt.Sprintf("%sRequest", name)),
		OutputType: proto.String(fmt.Sprintf("%sResponse", name)),
		Options:    &descriptorpb.MethodOptions{},
	}
	proto.SetExtension(mm.Options, annotations.E_Http, rule)
	return mm
}

func filesToSwagger(t testing.TB, fileDescriptors ...*descriptorpb.FileDescriptorProto) *Document {
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

	swaggerDoc, err := Build(services)
	if err != nil {
		t.Fatal(err)
	}
	return swaggerDoc
}

const (
	pathMessage = 4
	pathField   = 2
)

func TestBuild(t *testing.T) {

	swaggerDoc := filesToSwagger(t, &descriptorpb.FileDescriptorProto{
		Name:    proto.String("test.proto"),
		Package: proto.String("test"),
		Service: []*descriptorpb.ServiceDescriptorProto{{
			Name: proto.String("TestService"),
			Method: []*descriptorpb.MethodDescriptorProto{
				buildHTTPMethod("Test", &annotations.HttpRule{
					Pattern: &annotations.HttpRule_Get{
						Get: "/test/{test_field}",
					},
				}),
			},
		}},
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("TestRequest"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:   proto.String("test_field"),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
				Number: proto.Int32(1),
			}},
		}, {
			Name: proto.String("TestResponse"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:   proto.String("test_field"),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
				Number: proto.Int32(1),
			}, {
				Name:     proto.String("msg"),
				Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
				Number:   proto.Int32(2),
				TypeName: proto.String(".test.Nested"),
			}},
		}, {
			Name: proto.String("Nested"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:   proto.String("nested_field"),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
				Number: proto.Int32(1),
			}},
		}},
		SourceCodeInfo: &descriptorpb.SourceCodeInfo{
			Location: []*descriptorpb.SourceCodeInfo_Location{{
				LeadingComments: proto.String("Message Comment"),
				Path:            []int32{pathMessage, 2}, // 4 = Message, 2 = Nested
				Span:            []int32{1, 1, 1},        // Single line comment
			}, {
				LeadingComments: proto.String("Field Comment"),
				Path:            []int32{pathMessage, 2, pathField, 0}, // 4 = Message, 2 = Nested, 1 = Field
				Span:            []int32{2, 1, 2},                      // Single line comment
			}},
		},
	})

	bb, err := json.MarshalIndent(swaggerDoc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(bb))

	if _, ok := swaggerDoc.GetSchema("#/components/schemas/test.TestRequest"); ok {
		t.Fatal("TestRequest should not be registered as a schema, but was")
	}

	refSchema, ok := swaggerDoc.GetSchema("#/components/schemas/test.Nested")
	if !ok {
		t.Fatal("schema not found")
	}

	if tn := refSchema.ItemType.TypeName(); tn != "object" {
		t.Fatalf("unexpected type: %s", tn)
	}

	if refSchema.Description != "Message Comment" {
		t.Errorf("unexpected description: '%s'", refSchema.Description)
	}

	asObject := refSchema.ItemType.(ObjectItem)
	if len(asObject.Properties) != 1 {
		t.Fatalf("unexpected properties: %d", len(asObject.Properties))
	}

	f1 := asObject.Properties[0]
	if f1.Name != "nestedField" {
		t.Errorf("unexpected field name: '%s'", f1.Name)
	}

	if f1.Description != "Field Comment" {
		t.Errorf("unexpected description: '%s'", f1.Description)
	}

}

func TestCommentBuilder(t *testing.T) {

	for _, tc := range []struct {
		name     string
		leading  string
		trailing string
		expected string
	}{{
		name:     "leading",
		leading:  "comment",
		expected: "comment",
	}, {
		name:     "fallback",
		expected: "fallback",
	}, {
		name:     "both",
		leading:  "leading",
		trailing: "trailing",
		expected: "leading\ntrailing",
	}, {
		name:     "multiline",
		leading:  "line1\n  line2",
		trailing: "line3\n  line4",
		expected: "line1\nline2\nline3\nline4",
	}, {
		name:     "multiline commented",
		leading:  "#line1\nline2",
		expected: "line2",
	}, {
		name:     "commented fallback",
		leading:  "#line1",
		expected: "fallback",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			sl := protoreflect.SourceLocation{
				LeadingComments:  tc.leading,
				TrailingComments: tc.trailing,
			}

			got := buildComment(sl, "fallback")
			if got != tc.expected {
				t.Errorf("expected comment: '%s', got '%s'", tc.expected, got)
			}

		})
	}

}
