package proxy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
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

func (rr *Router) StaticJSON(path string, document interface{}) error {
	jb, err := json.Marshal(document)
	if err != nil {
		return err
	}

	rr.router.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(jb) // nolint: errcheck
	})
	return nil
}

func (rr *Router) RegisterService(ss protoreflect.ServiceDescriptor, conn Invoker) error {
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
	handler, err := rr.buildMethod(serviceName, method, conn)
	if err != nil {
		return err
	}
	rr.router.Methods(handler.HTTPMethod).Path(handler.HTTPPath).Handler(handler)
	return nil
}

func (rr *Router) buildMethod(serviceName string, method protoreflect.MethodDescriptor, conn Invoker) (*Method, error) {
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
		return nil, fmt.Errorf("unsupported http method %T", pt)
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

	return handler, nil

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

func int32Mapper(fd protoreflect.FieldDescriptor, s string) (protoreflect.Value, error) {
	val, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return protoreflect.Value{}, err
	}
	return protoreflect.ValueOfInt32(int32(val)), nil
}

func uint32Mapper(fd protoreflect.FieldDescriptor, s string) (protoreflect.Value, error) {
	val, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return protoreflect.Value{}, err
	}
	return protoreflect.ValueOfUint32(uint32(val)), nil
}

func int64Mapper(fd protoreflect.FieldDescriptor, s string) (protoreflect.Value, error) {
	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return protoreflect.Value{}, err
	}
	return protoreflect.ValueOfInt64(val), nil
}

func uint64Mapper(fd protoreflect.FieldDescriptor, s string) (protoreflect.Value, error) {
	val, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return protoreflect.Value{}, err
	}
	return protoreflect.ValueOfUint64(val), nil
}

type mapperFromString func(protoreflect.FieldDescriptor, string) (protoreflect.Value, error)

var mappersFromString = map[protoreflect.Kind]mapperFromString{
	// Signed Int
	protoreflect.Int32Kind: int32Mapper,
	protoreflect.Int64Kind: int64Mapper,
	// Insigned Int
	protoreflect.Uint32Kind: uint32Mapper,
	protoreflect.Uint64Kind: uint64Mapper,
	// Signed Int - the other type but go doesn't care
	protoreflect.Sint32Kind: int32Mapper,
	protoreflect.Sint64Kind: int64Mapper,
	// Signed Fixed uses the whole byte space, again the same in Go
	protoreflect.Sfixed32Kind: int32Mapper,
	protoreflect.Sfixed64Kind: int64Mapper,
	// Unsigned Fixed uses the whole byte space, again the same in Go
	protoreflect.Fixed32Kind: uint32Mapper,
	protoreflect.Fixed64Kind: uint64Mapper,

	protoreflect.StringKind: func(fd protoreflect.FieldDescriptor, s string) (protoreflect.Value, error) {
		return protoreflect.ValueOfString(s), nil
	},
	protoreflect.BoolKind: func(fd protoreflect.FieldDescriptor, s string) (protoreflect.Value, error) {
		return protoreflect.ValueOfBool(s == "true"), nil
	},
	protoreflect.FloatKind: func(fd protoreflect.FieldDescriptor, s string) (protoreflect.Value, error) {
		val, err := strconv.ParseFloat(s, 32)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfFloat32(float32(val)), nil
	},
	protoreflect.DoubleKind: func(fd protoreflect.FieldDescriptor, s string) (protoreflect.Value, error) {
		val, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfFloat64(val), nil
	},
	protoreflect.BytesKind: func(fd protoreflect.FieldDescriptor, s string) (protoreflect.Value, error) {
		val, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfBytes(val), nil
	},
	protoreflect.EnumKind: func(fd protoreflect.FieldDescriptor, s string) (protoreflect.Value, error) {
		enumValue := fd.Enum().Values().ByName(protoreflect.Name(s))
		if enumValue == nil {
			return protoreflect.Value{}, fmt.Errorf("invalid enum value %q for field %q", s, fd.Name())
		}
		return protoreflect.ValueOfEnum(enumValue.Number()), nil
	},
}

func setFieldFromString(inputMessage *dynamicpb.Message, fd protoreflect.FieldDescriptor, provided string) error {
	tc, ok := mappersFromString[fd.Kind()]
	if !ok {
		return fmt.Errorf("unsupported field type %s", fd.Kind())
	}

	val, err := tc(fd, provided)
	if err != nil {
		return err
	}

	inputMessage.Set(fd, val)
	return nil
}

func setFieldFromStrings(inputMessage *dynamicpb.Message, fd protoreflect.FieldDescriptor, provided []string) error {

	if !fd.IsList() {
		if len(provided) > 1 {
			return fmt.Errorf("multiple values provided for non-repeated field %q", fd.Name())
		}
		return setFieldFromString(inputMessage, fd, provided[0])
	}

	tc, ok := mappersFromString[fd.Kind()]
	if !ok {
		return fmt.Errorf("unsupported field type %s", fd.Kind())
	}

	field := inputMessage.NewField(fd)
	list := field.List()

	for _, s := range provided {
		val, err := tc(fd, s)
		if err != nil {
			return err
		}
		list.Append(val)
	}

	inputMessage.Set(fd, field)

	return nil
}

func (mm *Method) mapRequest(r *http.Request) (protoreflect.Message, error) {
	inputMessage := dynamicpb.NewMessage(mm.Input)
	reqBody, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	if len(reqBody) > 0 {
		if err := protojson.Unmarshal(reqBody, inputMessage); err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	reqVars := mux.Vars(r)
	for key, provided := range reqVars {
		fd := mm.Input.Fields().ByName(protoreflect.Name(key))
		if err := setFieldFromString(inputMessage, fd, provided); err != nil {
			return nil, err
		}
	}

	query := r.URL.Query()
	for key, values := range query {
		fd := mm.Input.Fields().ByName(protoreflect.Name(key))
		if err := setFieldFromStrings(inputMessage, fd, values); err != nil {
			return nil, err
		}
	}

	return inputMessage, nil
}

func (mm *Method) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	inputMessage, err := mm.mapRequest(r)
	if err != nil {
		doUserError(ctx, w, err)
		return
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
		doUserError(ctx, w, err)
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

func doUserError(ctx context.Context, w http.ResponseWriter, err error) {
	// TODO: Handle specific gRPC trailer type errors
	if statusError, isStatusError := status.FromError(err); isStatusError {
		doStatusError(ctx, w, statusError)
		return
	}
	doError(ctx, w, err)
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
