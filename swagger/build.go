package swagger

import (
	"fmt"
	"regexp"
	"strings"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

func Build(services []protoreflect.ServiceDescriptor) (*Document, error) {
	b := builder{
		document: &Document{
			OpenAPI: "3.0.0",
			Components: Components{
				Schemas:         make(map[string]*SchemaItem),
				SecuritySchemes: make(map[string]interface{}),
			},
		},
		paths: make(map[string]*PathItem),
	}

	for _, service := range services {
		if err := b.addService(service); err != nil {
			return nil, err
		}
	}

	return b.document, nil

}

type builder struct {
	document *Document

	paths map[string]*PathItem
}

func (bb *builder) addService(src protoreflect.ServiceDescriptor) error {
	methods := src.Methods()
	name := string(src.FullName())
	for ii := 0; ii < methods.Len(); ii++ {
		method := methods.Get(ii)
		if err := bb.registerMethod(name, method); err != nil {
			return err
		}
	}
	return nil
}

var rePathParameter = regexp.MustCompile(`\{([^\}]+)\}`)

func (bb *builder) registerMethod(serviceName string, method protoreflect.MethodDescriptor) error {

	methodOptions := method.Options().(*descriptorpb.MethodOptions)
	httpOpt := proto.GetExtension(methodOptions, annotations.E_Http).(*annotations.HttpRule)

	var httpMethod string
	var httpPath string

	if httpOpt == nil {
		return fmt.Errorf("missing http rule for method %s", method.Name())
	}
	switch pt := httpOpt.Pattern.(type) {
	case *annotations.HttpRule_Get:
		httpMethod = "get"
		httpPath = pt.Get
	case *annotations.HttpRule_Post:
		httpMethod = "post"
		httpPath = pt.Post
	case *annotations.HttpRule_Put:
		httpMethod = "put"
		httpPath = pt.Put
	case *annotations.HttpRule_Delete:
		httpMethod = "delete"
		httpPath = pt.Delete
	case *annotations.HttpRule_Patch:
		httpMethod = "patch"
		httpPath = pt.Patch

	default:
		return fmt.Errorf("unsupported http method %T", pt)
	}

	operation := &Operation{
		OperationHeader: OperationHeader{
			Method:      httpMethod,
			Path:        httpPath,
			OperationID: fmt.Sprintf("/%s/%s", serviceName, method.Name()),
		},
	}

	okResponse, err := bb.buildSchemaObject(method.Output())
	if err != nil {
		return err
	}

	operation.Responses = &ResponseSet{{
		Code:        200,
		Description: "OK",
		Content: OperationContent{
			JSON: &OperationSchema{
				Schema: *okResponse,
			},
		},
	}}

	request, err := bb.buildSchemaObject(method.Input())
	if err != nil {
		return err
	}

	requestObject := request.ItemType.(ObjectItem)

	for _, paramStr := range rePathParameter.FindAllString(httpPath, -1) {
		name := paramStr[1 : len(paramStr)-1]
		parts := strings.SplitN(name, ".", 2)
		if len(parts) > 1 {
			return fmt.Errorf("path parameter %q is not a top level field", name)
		}

		prop, ok := requestObject.GetProperty(parts[0])
		if !ok {
			return fmt.Errorf("path parameter %q not found in request object", name)
		}

		prop.Skip = true
		operation.Parameters = append(operation.Parameters, Parameter{
			Name:     name,
			In:       "path",
			Required: true,
			Schema:   prop.SchemaItem,
		})
	}

	if httpOpt.Body == "" {
		// TODO: This should probably be based on the annotation setting of body
		for _, param := range requestObject.Properties {
			operation.Parameters = append(operation.Parameters, Parameter{
				Name:     param.Name,
				In:       "query",
				Required: false,
				Schema:   param.SchemaItem,
			})
		}
	} else if httpOpt.Body == "*" {
		operation.RequestBody = &RequestBody{
			Required: true,
			Content: OperationContent{
				JSON: &OperationSchema{
					Schema: *request,
				},
			},
		}
	} else {
		return fmt.Errorf("unsupported body type %q", httpOpt.Body)
	}

	path, ok := bb.paths[httpPath]
	if !ok {
		path = &PathItem{}
		bb.paths[httpPath] = path
		bb.document.Paths = append(bb.document.Paths, path)
	}
	path.AddOperation(operation)

	return nil
}

func (bb *builder) buildSchemaObject(src protoreflect.MessageDescriptor) (*SchemaItem, error) {

	obj := ObjectItem{
		ProtoMessageName: string(src.FullName()),
		Properties:       make([]*ObjectProperty, 0, src.Fields().Len()),
	}

	for ii := 0; ii < src.Fields().Len(); ii++ {
		field := src.Fields().Get(ii)
		prop, err := bb.buildSchemaProperty(field)
		if err != nil {
			return nil, err
		}
		obj.Properties = append(obj.Properties, prop)
	}

	description := commentDescription(src, string(src.Name()))

	return &SchemaItem{
		Description: description,
		ItemType:    obj,
	}, nil
}

func commentDescription(src protoreflect.Descriptor, fallback string) string {
	sourceLocation := src.ParentFile().SourceLocations().ByDescriptor(src)
	return buildComment(sourceLocation, fallback)
}

func buildComment(sourceLocation protoreflect.SourceLocation, fallback string) string {
	allComments := make([]string, 0)
	if sourceLocation.LeadingComments != "" {
		allComments = append(allComments, strings.Split(sourceLocation.LeadingComments, "\n")...)
	}
	if sourceLocation.TrailingComments != "" {
		allComments = append(allComments, strings.Split(sourceLocation.TrailingComments, "\n")...)
	}

	// Trim leading whitespace
	commentsOut := make([]string, 0, len(allComments))
	for _, comment := range allComments {
		comment = strings.TrimSpace(comment)
		if comment == "" {
			continue
		}
		if strings.HasPrefix(comment, "#") {
			continue
		}
		commentsOut = append(commentsOut, comment)
	}

	if len(commentsOut) <= 0 {
		return fallback
	}
	return strings.Join(commentsOut, "\n")
}

func (bb *builder) buildSchemaProperty(src protoreflect.FieldDescriptor) (*ObjectProperty, error) {

	prop := &ObjectProperty{
		ProtoFieldName:   string(src.Name()),
		ProtoFieldNumber: int(src.Number()),
		Name:             string(src.JSONName()),
		Description:      commentDescription(src, ""),
	}

	// TODO: Validation / Rules
	// TODO: Oneof (meta again?)
	// TODO: Repeated
	// TODO: Map
	// TODO: Extra types (see below)

	switch src.Kind() {
	case protoreflect.BoolKind:
		prop.SchemaItem = SchemaItem{
			ItemType: BooleanItem{},
		}

	case protoreflect.EnumKind:
		prop.SchemaItem = SchemaItem{
			ItemType: EnumItem{},
		}

	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Uint32Kind:
		prop.SchemaItem = SchemaItem{
			ItemType: IntegerItem{
				Format: "int32",
			},
		}

	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Uint64Kind:
		prop.SchemaItem = SchemaItem{
			ItemType: IntegerItem{
				Format: "int64",
			},
		}

	case protoreflect.FloatKind, protoreflect.Sfixed64Kind, protoreflect.Fixed64Kind, protoreflect.DoubleKind:
		prop.SchemaItem = SchemaItem{
			ItemType: NumberItem{
				Format: "float",
			},
		}

	case protoreflect.StringKind:
		prop.SchemaItem = SchemaItem{
			Description: string(src.Name()),
			ItemType:    StringItem{},
		}

	case protoreflect.BytesKind:
		prop.SchemaItem = SchemaItem{
			ItemType: StringItem{
				Format: "byte",
			},
		}

	case protoreflect.MessageKind:
		// When called from a field of a message, this creates a ref. When built directly from a service RPC request or create, this code is not called, they are inlined with the buildSchemaObject call directly
		prop.SchemaItem = SchemaItem{
			Ref: fmt.Sprintf("#/components/schemas/%s", src.Message().FullName()),
		}
		if err := bb.addSchemaRef(src.Message()); err != nil {
			return nil, err
		}

	default:
		/* TODO:
		Sfixed32Kind Kind = 15
		Fixed32Kind  Kind = 7
		Sfixed64Kind Kind = 16
		Fixed64Kind  Kind = 6
		GroupKind    Kind = 10
		*/
		return nil, fmt.Errorf("unsupported field type %s", src.Kind())
	}

	return prop, nil

}

func (bb *builder) addSchemaRef(src protoreflect.MessageDescriptor) error {

	if _, ok := bb.document.Components.Schemas[string(src.FullName())]; ok {
		return nil
	}

	schema, err := bb.buildSchemaObject(src)
	if err != nil {
		return err
	}

	bb.document.Components.Schemas[string(src.FullName())] = schema

	return nil
}
