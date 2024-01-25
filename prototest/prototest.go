package prototest

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

type ResultSet struct {
	*protoregistry.Files
}

func (rs ResultSet) MessageByName(t testing.TB, name protoreflect.FullName) protoreflect.MessageDescriptor {
	t.Helper()
	md, err := rs.FindDescriptorByName(name)
	if err != nil {
		t.Fatal(err)
	}
	return md.(protoreflect.MessageDescriptor)
}

func (rs ResultSet) ServiceByName(t testing.TB, name protoreflect.FullName) protoreflect.ServiceDescriptor {
	t.Helper()
	md, err := rs.FindDescriptorByName(name)
	if err != nil {
		t.Fatal(err)
	}
	return md.(protoreflect.ServiceDescriptor)
}

func DescriptorsFromSource(t testing.TB, sourceFiles map[string]string) ResultSet {
	t.Helper()

	parser := protoparse.Parser{
		ImportPaths:           []string{""},
		IncludeSourceCodeInfo: false,
		// Load everything which the runtime has already leaded, includes all
		// the google, buf, psm etc packages, so long as something which uses
		// them has already beein imported
		LookupImport: desc.LoadFileDescriptor,
		Accessor: func(filename string) (io.ReadCloser, error) {
			if content, ok := sourceFiles[filename]; ok {
				return io.NopCloser(strings.NewReader(content)), nil
			}
			return nil, fmt.Errorf("file not found: %s", filename)
		},
	}

	customDesc, err := parser.ParseFiles("test.proto")
	if err != nil {
		t.Fatal(err)
	}

	realDesc := desc.ToFileDescriptorSet(customDesc...)
	descFiles, err := protodesc.NewFiles(realDesc)
	if err != nil {
		t.Fatal(err)
	}

	return ResultSet{
		Files: descFiles,
	}
}
