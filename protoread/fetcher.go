package protoread

import (
	"context"

	"google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

type fetcher struct {
	files  *protoregistry.Files
	stream *reflStream
	ctx    context.Context
}

func newFetcher(ctx context.Context, stream *reflStream, files *protoregistry.Files) *fetcher {
	return &fetcher{
		files:  files,
		stream: stream,
		ctx:    ctx,
	}
}

func (ff *fetcher) FindDescriptorByName(name protoreflect.FullName) (protoreflect.Descriptor, error) {
	desc, err := ff.files.FindDescriptorByName(name)
	if err == nil {
		return desc, nil
	}

	fileResp, err := ff.stream.fileContainingSymbol(ff.ctx, string(name))
	if err != nil {
		return nil, err
	}

	if err := ff.registerFileResponse(fileResp); err != nil {
		return nil, err
	}

	return ff.files.FindDescriptorByName(name)
}

func (ff *fetcher) FindFileByPath(name string) (protoreflect.FileDescriptor, error) {
	desc, err := ff.files.FindFileByPath(name)
	if err == nil {
		return desc, nil
	}

	return ff.fetchAndIncludeFile(string(name))
}

func (ff *fetcher) registerFileResponse(fileResp *grpc_reflection_v1.FileDescriptorResponse) error {
	for _, rawFile := range fileResp.FileDescriptorProto {
		file := &descriptorpb.FileDescriptorProto{}
		if err := proto.Unmarshal(rawFile, file); err != nil {
			return err
		}

		// Skip if already registered
		if _, err := ff.files.FindFileByPath(file.GetName()); err == nil {
			continue
		}

		fs, err := protodesc.NewFile(file, ff)
		if err != nil {
			return err
		}

		if err := ff.files.RegisterFile(fs); err != nil {
			return err
		}
	}
	return nil
}

func (ff *fetcher) fetchAndIncludeFile(path string) (protoreflect.FileDescriptor, error) {
	fileResp, err := ff.stream.file(ff.ctx, path)
	if err != nil {
		return nil, err
	}

	if err := ff.registerFileResponse(fileResp); err != nil {
		return nil, err
	}

	return ff.files.FindFileByPath(path)
}
