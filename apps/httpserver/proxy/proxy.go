package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/mux"
	"github.com/pentops/j5/gen/j5/auth/v1/auth_j5pb"
	"github.com/pentops/j5/gen/j5/ext/v1/ext_j5pb"
	"github.com/pentops/log.go/log"
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/genproto/googleapis/api/httpbody"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// AuthHeaders translates a request into headers to pass on to the remote server
// Errors which implement gRPC status will be returned to the client as HTTP
// errors, otherwise 500 with a log line
type AuthHeaders interface {
	AuthHeaders(context.Context, *http.Request) (map[string]string, error)
}

type AuthHeadersFunc func(context.Context, *http.Request) (map[string]string, error)

func (f AuthHeadersFunc) AuthHeaders(ctx context.Context, r *http.Request) (map[string]string, error) {
	return f(ctx, r)
}

type AppConn interface {
	// Invoke is designed for gRPC ClientConn.Invoke, the two interfaces should
	// be protos...
	Invoke(context.Context, string, interface{}, interface{}, ...grpc.CallOption) error

	JSONToProto(body []byte, msg protoreflect.Message) error
	QueryToProto(query url.Values, msg protoreflect.Message) error
	ProtoToJSON(msg protoreflect.Message) ([]byte, error)
}

type Router struct {
	router                 *mux.Router
	ForwardResponseHeaders map[string]bool
	ForwardRequestHeaders  map[string]bool
	globalAuth             AuthHeaders

	middleware []func(http.Handler) http.Handler
}

func NewRouter() *Router {
	router := mux.NewRouter()

	router.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}` + "\n")) // nolint: errcheck
		log.WithField(r.Context(), "httpURL", r.URL.String()).Debug("Not found")
	})

	return &Router{
		router: router,
		ForwardResponseHeaders: map[string]bool{
			"set-cookie": true,
			"x-version":  true,
		},
		ForwardRequestHeaders: map[string]bool{
			"cookie":          true,
			"origin":          true,
			"x-forwarded-for": true,
			"user-agent":      true,
		},
	}
}

func (rr *Router) SetGlobalAuth(auth AuthHeaders) {
	rr.globalAuth = auth
}

func (rr *Router) AddMiddleware(middleware func(http.Handler) http.Handler) {
	rr.middleware = append(rr.middleware, middleware)
}

func (rr *Router) SetNotFoundHandler(handler http.Handler) {
	rr.router.NotFoundHandler = handler
}

func (rr *Router) SetHealthCheck(path string, callback func() error) {
	rr.router.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		if err := callback(); err != nil {
			doError(r.Context(), w, err)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}` + "\n")) // nolint: errcheck
	})
}

func (rr *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var handler http.Handler = rr.router
	for _, mw := range rr.middleware {
		handler = mw(handler)
	}
	handler.ServeHTTP(w, r)
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

type GRPCMethodConfig struct {
	AuthHeaders AuthHeaders
	Invoker     AppConn
	Method      protoreflect.MethodDescriptor
}

func (rr *Router) RegisterGRPCService(ctx context.Context, sd protoreflect.ServiceDescriptor, invoker AppConn) error {

	var defaultAuth auth_j5pb.IsMethodAuthTypeWrappedType = &auth_j5pb.MethodAuthType_None{}
	if rr.globalAuth != nil {
		defaultAuth = &auth_j5pb.MethodAuthType_JWTBearer{}
	}

	serviceExt := proto.GetExtension(sd.Options(), ext_j5pb.E_Service).(*ext_j5pb.ServiceOptions)
	if serviceExt != nil {
		if serviceExt.DefaultAuth != nil {
			defaultAuth = serviceExt.DefaultAuth.Get()
			log.WithFields(ctx, map[string]interface{}{
				"authMethod": defaultAuth.TypeKey(),
				"service":    sd.FullName(),
			}).Debug("Service Default Auth")
		}
	}

	methods := sd.Methods()
	for ii := 0; ii < methods.Len(); ii++ {
		method := methods.Get(ii)
		if err := rr.registerMethod(ctx, method, invoker, defaultAuth); err != nil {
			return fmt.Errorf("failed to register grpc method: %w", err)
		}
	}

	return nil
}

func (rr *Router) buildMethod(md protoreflect.MethodDescriptor, conn AppConn, auth auth_j5pb.IsMethodAuthTypeWrappedType) (*grpcMethod, error) {

	methodOptions := md.Options().(*descriptorpb.MethodOptions)

	if j5Method := proto.GetExtension(methodOptions, ext_j5pb.E_Method).(*ext_j5pb.MethodOptions); j5Method != nil {
		if j5Method.Auth != nil {
			auth = j5Method.Auth.Get()
		}
	}

	serviceName := md.Parent().(protoreflect.ServiceDescriptor).FullName()
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

	handler := &grpcMethod{
		// the 'FullName' method of MethodDescriptor returns this in the wrong format, i.e. all dots.
		FullName:               fmt.Sprintf("/%s/%s", serviceName, md.Name()),
		Input:                  md.Input(),
		Output:                 md.Output(),
		AppCon:                 conn,
		HTTPMethod:             httpMethod,
		HTTPPath:               httpPath,
		ForwardResponseHeaders: maps.Clone(rr.ForwardResponseHeaders),
		ForwardRequestHeaders:  rr.ForwardRequestHeaders,
		authHeaders:            nil, // Set in a bit
	}

	switch authType := auth.(type) {
	case *auth_j5pb.MethodAuthType_None:
		handler.authMethodName = "none"

	case *auth_j5pb.MethodAuthType_JWTBearer:
		if rr.globalAuth == nil {
			return nil, fmt.Errorf("auth method specified as JWT, no JWKS configured")
		}
		handler.authHeaders = rr.globalAuth
		handler.authMethodName = "jwt-bearer"

	case *auth_j5pb.MethodAuthType_Cookie:
		handler.authMethodName = "cookie"
		handler.ForwardRequestHeaders["cookie"] = true

	case *auth_j5pb.MethodAuthType_Custom:
		handler.authMethodName = fmt.Sprintf("custom, headers: %s", strings.Join(authType.PassThroughHeaders, ", "))

		for _, custom := range authType.PassThroughHeaders {
			handler.ForwardRequestHeaders[custom] = true
		}

	default:
		return nil, fmt.Errorf("auth method not supported (%d)", auth)
	}

	return handler, nil
}

func (rr *Router) registerMethod(ctx context.Context, md protoreflect.MethodDescriptor, conn AppConn, auth auth_j5pb.IsMethodAuthTypeWrappedType) error {
	handler, err := rr.buildMethod(md, conn, auth)
	if err != nil {
		return err
	}

	rr.router.Methods(handler.HTTPMethod).Path(handler.HTTPPath).Handler(handler)
	log.WithFields(ctx, map[string]interface{}{
		"method":     handler.HTTPMethod,
		"path":       handler.HTTPPath,
		"grpc":       handler.FullName,
		"authMethod": handler.authMethodName,
	}).Info("Registered HTTP Method")
	return nil

}

type grpcMethod struct {
	FullName               string
	Input                  protoreflect.MessageDescriptor
	Output                 protoreflect.MessageDescriptor
	AppCon                 AppConn
	HTTPMethod             string
	HTTPPath               string
	ForwardResponseHeaders map[string]bool
	ForwardRequestHeaders  map[string]bool
	authHeaders            AuthHeaders
	authMethodName         string
}

func (mm *grpcMethod) mapRequest(r *http.Request) (protoreflect.Message, error) {
	inputMessage := dynamicpb.NewMessage(mm.Input)
	reqBody, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	if len(reqBody) > 0 {
		if err := mm.AppCon.JSONToProto(reqBody, inputMessage); err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	query := r.URL.Query()
	reqVars := mux.Vars(r)
	for key, provided := range reqVars {
		query.Set(key, provided)
	}
	if err := mm.AppCon.QueryToProto(query, inputMessage); err != nil {
		return nil, err
	}

	return inputMessage, nil
}

func (mm *grpcMethod) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ctx = log.WithFields(ctx, map[string]interface{}{
		"httpMethod": r.Method,
		"httpURL":    r.URL.String(),
		"gRPCMethod": mm.FullName,
	})

	inputMessage, err := mm.mapRequest(r)
	if err != nil {
		doUserError(ctx, w, err)
		return
	}

	var outputMessage proto.Message
	var httpBodyOutput *httpbody.HttpBody
	var dynamicOutput *dynamicpb.Message
	if mm.Output.FullName() == "google.api.HttpBody" {
		httpBodyOutput = &httpbody.HttpBody{}
		outputMessage = httpBodyOutput
	} else {
		outputMessage = dynamicpb.NewMessage(mm.Output)
		dynamicOutput = outputMessage.(*dynamicpb.Message)
	}

	md := map[string]string{}
	for key, v := range r.Header {
		key = strings.ToLower(key)
		if !mm.ForwardRequestHeaders[key] {
			continue
		}
		md[key] = v[0] // only one value in gRPC
	}
	ctx = log.WithField(ctx, "passthroughHeaders", md)

	if mm.authHeaders != nil {
		authHeaders, err := mm.authHeaders.AuthHeaders(ctx, r)
		if err != nil {
			doUserError(ctx, w, err)
			return
		}
		for key, val := range authHeaders {
			md[key] = val

		}
		ctx = log.WithField(ctx, "authHeaders", authHeaders)
	}

	// Send request header
	ctx = metadata.NewOutgoingContext(ctx, metadata.New(md))

	// Receive response header
	var responseHeader metadata.MD

	err = mm.AppCon.Invoke(ctx, mm.FullName, inputMessage, outputMessage, grpc.Header(&responseHeader))
	if err != nil {
		doUserError(ctx, w, err)
		return
	}

	headerOut := w.Header()

	var bytesOut []byte

	if httpBodyOutput != nil {
		headerOut.Set("Content-Type", httpBodyOutput.ContentType)
		bytesOut = httpBodyOutput.Data
	} else {
		bytesOut, err = mm.AppCon.ProtoToJSON(dynamicOutput)
		if err != nil {
			log.WithError(ctx, err).Error("Failed to marshal response")
			doError(ctx, w, err)
			return
		}

		headerOut.Set("Content-Type", "application/json")
	}

	for key, vals := range responseHeader {
		key = strings.ToLower(key)
		if !mm.ForwardResponseHeaders[key] {
			continue
		}
		for _, val := range vals {
			headerOut.Add(key, val)
		}
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(bytesOut); err != nil {
		log.WithError(ctx, err).Error("Failed to write response")
		return
	}
	log.Info(ctx, "Request completed")
}

func doUserError(ctx context.Context, w http.ResponseWriter, err error) {
	// TODO: Handle specific gRPC trailer type errors
	if statusError, isStatusError := status.FromError(err); isStatusError {
		log.WithField(ctx, "httpError", statusError).Info("User error")
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
	headerOut := w.Header()
	headerOut.Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	if _, err := w.Write(bytesOut); err != nil {
		log.WithError(ctx, err).Error("Failed to write error response")
		return
	}
}

func doStatusError(ctx context.Context, w http.ResponseWriter, statusError *status.Status) {
	bytesOut, err := json.Marshal(map[string]string{
		"error": statusError.Message(),
	})

	if err != nil {
		log.WithError(ctx, err).Error("Failed to marshal error response")
		http.Error(w, `{"error":"meta error marshalling error"}`, http.StatusInternalServerError)
		return
	}

	headerOut := w.Header()
	headerOut.Set("Content-Type", "application/json")

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
	codes.OK:                 http.StatusOK,
	codes.Unknown:            http.StatusInternalServerError,
	codes.Canceled:           http.StatusRequestTimeout,
	codes.InvalidArgument:    http.StatusBadRequest,
	codes.DeadlineExceeded:   http.StatusRequestTimeout,
	codes.NotFound:           http.StatusNotFound,
	codes.PermissionDenied:   http.StatusForbidden,
	codes.ResourceExhausted:  http.StatusTooManyRequests,
	codes.FailedPrecondition: http.StatusPreconditionFailed,
	codes.Aborted:            http.StatusPreconditionFailed,
	codes.Unavailable:        http.StatusServiceUnavailable,
	codes.AlreadyExists:      http.StatusConflict,
	codes.OutOfRange:         http.StatusBadRequest,
	codes.Unimplemented:      http.StatusNotImplemented,
	codes.Internal:           http.StatusInternalServerError,
	codes.DataLoss:           http.StatusInternalServerError,
	codes.Unauthenticated:    http.StatusUnauthorized,
}
