package messaging

import (
	"context"
	"fmt"

	"github.com/pentops/log.go/log"
	"github.com/pentops/o5-messaging/gen/o5/messaging/v1/messaging_pb"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const GenericTopic = "/o5.messaging.v1.topic.GenericMessageTopic/Generic"

type ErrNoHandlerMatched string

func (e ErrNoHandlerMatched) Error() string {
	return fmt.Sprintf("no handler matched for %q", string(e))
}

type Handler interface {
	HandleMessage(context.Context, *messaging_pb.Message) error
}

type HandlerFunc func(context.Context, *messaging_pb.Message) error

func (hf HandlerFunc) HandleMessage(ctx context.Context, msg *messaging_pb.Message) error {
	return hf(ctx, msg)
}

type Router struct {
	handlers        map[string]Handler
	fallbackHandler Handler
}

func NewRouter() *Router {
	return &Router{
		handlers: make(map[string]Handler),
	}
}

func (rr *Router) RegisterService(ctx context.Context, service protoreflect.ServiceDescriptor, invoker AppLink) error {
	methods := service.Methods()
	for ii := range methods.Len() {
		method := methods.Get(ii)
		if err := rr.registerMethod(ctx, method, invoker); err != nil {
			return err
		}
	}
	return nil
}

func (rr *Router) registerMethod(ctx context.Context, method protoreflect.MethodDescriptor, invoker AppLink) error {
	serviceName := method.Parent().(protoreflect.ServiceDescriptor).FullName()
	fullName := fmt.Sprintf("/%s/%s", serviceName, method.Name())

	if fullName == GenericTopic {
		log.WithField(ctx, "service", fullName).Info("Registering Generic Fallback")
		rr.fallbackHandler = &genericHandler{
			invoker: invoker,
		}

	} else {
		log.WithField(ctx, "service", fullName).Info("Registering Worker Service")
		ss := &service{
			requestMessage: method.Input(),
			fullName:       fullName,
			invoker:        invoker,
		}
		rr.handlers[ss.fullName] = ss
	}
	return nil
}

func (rr *Router) RegisterHandler(fullMethod string, handler Handler) {
	rr.handlers[fullMethod] = handler
}

func (rr *Router) HandleMessage(ctx context.Context, parsed *messaging_pb.Message) error {
	ctx = log.WithFields(ctx, map[string]any{
		"grpc-service": parsed.GrpcService,
		"grpc-method":  parsed.GrpcMethod,
		"message-id":   parsed.MessageId,
		"topic":        parsed.DestinationTopic,
	})
	log.Debug(ctx, "Message Handler: Begin")

	fullServiceName := fmt.Sprintf("/%s/%s", parsed.GrpcService, parsed.GrpcMethod)
	handler, ok := rr.handlers[fullServiceName]
	if !ok {
		if rr.fallbackHandler != nil {
			log.Debug(ctx, "Message Handler: Using fallback handler")
			handler = rr.fallbackHandler
		} else {
			return ErrNoHandlerMatched(fullServiceName)
		}
	}

	return handler.HandleMessage(ctx, parsed)
}
