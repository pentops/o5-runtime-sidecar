package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"gopkg.daemonl.com/log"
)

type Invoker interface {
	Invoke(context.Context, string, interface{}, interface{}, ...grpc.CallOption) error
}

type Router struct {
	router                 *mux.Router
	ForwardResponseHeaders map[string]bool
	ForwardRequestHeaders  map[string]bool
}

func NewRouter() *Router {
	return &Router{
		router: mux.NewRouter(),
		ForwardResponseHeaders: map[string]bool{
			"set-cookie": true,
			"x-version":  true,
		},
		ForwardRequestHeaders: map[string]bool{
			"cookie": true,
		},
	}
}

func (rr *Router) SetNotFoundHandler(handler http.Handler) {
	rr.router.NotFoundHandler = handler
}

func (rr *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rr.router.ServeHTTP(w, r)
}

func (rr *Router) RegisterService(ss protoreflect.ServiceDescriptor, conn *grpc.ClientConn) error {
	methods := ss.Methods()
	name := string(ss.FullName())
	for ii := 0; ii < methods.Len(); ii++ {
		method := methods.Get(ii)
		if err := rr.registerMethod(name, method, conn); err != nil {
			return err
		}
	}
	return nil
}

func (rr *Router) registerMethod(serviceName string, method protoreflect.MethodDescriptor, conn Invoker) error {

	methodOptions := method.Options().(*descriptorpb.MethodOptions)
	httpOpt := proto.GetExtension(methodOptions, annotations.E_Http).(*annotations.HttpRule)

	var httpMethod string
	var httpPath string

	switch pt := httpOpt.Pattern.(type) {
	case *annotations.HttpRule_Get:
		httpMethod = "GET"
		httpPath = pt.Get
	case *annotations.HttpRule_Post:
		httpMethod = "POST"
		httpPath = pt.Post
	case *annotations.HttpRule_Put:
		httpMethod = "PUT"
		httpPath = pt.Put
	case *annotations.HttpRule_Delete:
		httpMethod = "DELETE"
		httpPath = pt.Delete
	case *annotations.HttpRule_Patch:
		httpMethod = "PATCH"
		httpPath = pt.Patch

	default:
		return fmt.Errorf("unsupported http method %T", pt)
	}

	handler := &Method{
		// the 'FullName' method of MethodDescriptor returns this in the wrong format, i.e. all dots.
		FullName:               fmt.Sprintf("/%s/%s", serviceName, method.Name()),
		Input:                  method.Input(),
		Output:                 method.Output(),
		Invoker:                conn,
		HTTPMethod:             httpMethod,
		HTTPPath:               httpPath,
		ForwardResponseHeaders: rr.ForwardResponseHeaders,
		ForwardRequestHeaders:  rr.ForwardRequestHeaders,
	}

	rr.router.Methods(httpMethod).Path(httpPath).Handler(handler)
	return nil

}

type Method struct {
	FullName               string
	Input                  protoreflect.MessageDescriptor
	Output                 protoreflect.MessageDescriptor
	Invoker                Invoker
	HTTPMethod             string
	HTTPPath               string
	ForwardResponseHeaders map[string]bool
	ForwardRequestHeaders  map[string]bool
}

func (mm *Method) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	inputMessage := dynamicpb.NewMessage(mm.Input)

	// TODO: Map {dynamic} request parameters
	// TODO: GET / no body

	reqBody, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if len(reqBody) > 0 {
		if err := protojson.Unmarshal(reqBody, inputMessage); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	outputMessage := dynamicpb.NewMessage(mm.Output)

	md := map[string]string{}
	for key, v := range r.Header {
		key = strings.ToLower(key)
		if !mm.ForwardRequestHeaders[key] {
			continue
		}
		md[key] = v[0] // only one value in gRPC
	}

	// Send request header
	ctx = metadata.NewOutgoingContext(ctx, metadata.New(md))

	// Receive response header
	var responseHeader metadata.MD

	err = mm.Invoker.Invoke(ctx, mm.FullName, inputMessage, outputMessage, grpc.Header(&responseHeader))
	if err != nil {
		// TODO: Handle specific gRPC trailer type errors
		if statusError, isStatusError := status.FromError(err); isStatusError {
			doStatusError(ctx, w, statusError)
			return
		}
		doError(ctx, w, err)
		return
	}

	bytesOut, err := protojson.Marshal(outputMessage)
	if err != nil {
		doError(ctx, w, err)
		return
	}

	headerOut := w.Header()
	headerOut.Set("Content-Type", "application/json")

	for key, vals := range responseHeader {
		key = strings.ToLower(key)
		if !mm.ForwardResponseHeaders[key] {
			continue
		}
		for _, val := range vals {
			w.Header().Add(key, val)
		}
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(bytesOut); err != nil {
		log.WithError(ctx, err).Error("Failed to write response")
		return
	}
}

func doError(ctx context.Context, w http.ResponseWriter, err error) {
	log.WithError(ctx, err).Error("Error handling request")
	body := map[string]string{
		"error": err.Error(),
	}
	bytesOut, err := json.Marshal(body)
	if err != nil {
		log.WithError(ctx, err).Error("Failed to marshal error response")
		http.Error(w, `{"error":"meta error marshalling error"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusInternalServerError)
	if _, err := w.Write(bytesOut); err != nil {
		log.WithError(ctx, err).Error("Failed to write error response")
		return
	}
}

func doStatusError(ctx context.Context, w http.ResponseWriter, statusError *status.Status) {
	log.WithError(ctx, statusError.Err()).Error("Error handling request")
	body := map[string]string{
		"error": statusError.Message(),
	}
	bytesOut, err := json.Marshal(body)
	if err != nil {
		log.WithError(ctx, err).Error("Failed to marshal error response")
		http.Error(w, `{"error":"meta error marshalling error"}`, http.StatusInternalServerError)
		return
	}

	httpStatus, ok := statusToHTTPCode[statusError.Code()]
	if !ok {
		httpStatus = http.StatusInternalServerError
	}
	w.WriteHeader(httpStatus)

	if _, err := w.Write(bytesOut); err != nil {
		log.WithError(ctx, err).Error("Failed to write error response")
		return
	}
}

var statusToHTTPCode = map[codes.Code]int{
	// TODO: These were autocompleted by AI, check if they are correct
	codes.OK:                 http.StatusOK,
	codes.Canceled:           http.StatusRequestTimeout,
	codes.Unknown:            http.StatusInternalServerError,
	codes.InvalidArgument:    http.StatusBadRequest,
	codes.DeadlineExceeded:   http.StatusGatewayTimeout,
	codes.NotFound:           http.StatusNotFound,
	codes.AlreadyExists:      http.StatusConflict,
	codes.PermissionDenied:   http.StatusForbidden,
	codes.ResourceExhausted:  http.StatusTooManyRequests,
	codes.FailedPrecondition: http.StatusPreconditionFailed,
	codes.Aborted:            http.StatusConflict,
	codes.OutOfRange:         http.StatusBadRequest,
	codes.Unimplemented:      http.StatusNotImplemented,
	codes.Internal:           http.StatusInternalServerError,
	codes.Unavailable:        http.StatusServiceUnavailable,
	codes.DataLoss:           http.StatusInternalServerError,
	codes.Unauthenticated:    http.StatusUnauthorized,
}
