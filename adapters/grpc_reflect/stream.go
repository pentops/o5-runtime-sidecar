package grpc_reflect

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/pentops/log.go/log"
	"google.golang.org/grpc/reflection/grpc_reflection_v1"
)

type reflStream struct {
	stream grpc_reflection_v1.ServerReflection_ServerReflectionInfoClient
}

func newStream(ctx context.Context, cl grpc_reflection_v1.ServerReflectionClient) (*reflStream, error) {
	var stream grpc_reflection_v1.ServerReflection_ServerReflectionInfoClient

	for {
		cc, err := cl.ServerReflectionInfo(ctx)
		if err == nil {
			stream = cc
			break
		}

		log.WithError(ctx, err).Warn("fetching services. Retrying")

		if errors.Is(err, context.Canceled) {
			return nil, err
		}

		select {
		case <-time.After(time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return &reflStream{
		stream: stream,
	}, nil
}

func (rs *reflStream) roundTrip(req *grpc_reflection_v1.ServerReflectionRequest) (*grpc_reflection_v1.ServerReflectionResponse, error) {
	if err := rs.stream.Send(req); err != nil {
		return nil, err
	}

	return rs.stream.Recv()
}

func (rs *reflStream) close() error {
	return rs.stream.CloseSend()
}

func (rs *reflStream) listServices(_ context.Context) (*grpc_reflection_v1.ListServiceResponse, error) {
	d, err := rs.roundTrip(&grpc_reflection_v1.ServerReflectionRequest{
		MessageRequest: &grpc_reflection_v1.ServerReflectionRequest_ListServices{},
	})
	if err != nil {
		return nil, err
	}

	resp := d.GetListServicesResponse()
	if resp == nil {
		return nil, errors.New("no list services response")
	}

	return resp, nil
}

func (rs *reflStream) fileContainingSymbol(_ context.Context, symbol string) (*grpc_reflection_v1.FileDescriptorResponse, error) {
	d, err := rs.roundTrip(&grpc_reflection_v1.ServerReflectionRequest{
		MessageRequest: &grpc_reflection_v1.ServerReflectionRequest_FileContainingSymbol{
			FileContainingSymbol: symbol,
		},
	})
	if err != nil {
		return nil, err
	}

	resp := d.GetFileDescriptorResponse()
	if resp == nil {
		return nil, fmt.Errorf("no file descriptor response for symbol %s", symbol)
	}

	return resp, nil
}

func (rs *reflStream) file(_ context.Context, path string) (*grpc_reflection_v1.FileDescriptorResponse, error) {
	d, err := rs.roundTrip(&grpc_reflection_v1.ServerReflectionRequest{
		MessageRequest: &grpc_reflection_v1.ServerReflectionRequest_FileByFilename{
			FileByFilename: path,
		},
	})
	if err != nil {
		return nil, err
	}

	resp := d.GetFileDescriptorResponse()
	if resp == nil {
		return nil, fmt.Errorf("no file descriptor response for file %s", path)
	}

	return resp, nil
}
