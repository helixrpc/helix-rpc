package runtime

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"reflect"
	"strings"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

type GRPCHandler struct {
	services map[string]interface{}
}

func NewGRPCHandler() *GRPCHandler {
	return &GRPCHandler{
		services: make(map[string]interface{}),
	}
}

func (h *GRPCHandler) RegisterService(name string, impl interface{}) {
	h.services[name] = impl
}

func (h *GRPCHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// gRPC paths are typically /package_service.Service/Method
	path := r.URL.Path
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) != 2 {
		http.Error(w, "invalid gRPC path", http.StatusBadRequest)
		return
	}

	serviceName := parts[0]
	methodName := parts[1]

	serviceImpl, ok := h.services[serviceName]
	if !ok {
		w.Header().Set("grpc-status", "12") // UNIMPLEMENTED
		w.Header().Set("grpc-message", fmt.Sprintf("service %s not found", serviceName))
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Header.Get("Content-Type") == "application/json" {
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusInternalServerError)
			return
		}

		handlerVal := reflectValueOf(serviceImpl)
		handlerMethod := handlerVal.MethodByName(methodName)
		if !handlerMethod.IsValid() {
			http.Error(w, fmt.Sprintf("method %s not found", methodName), http.StatusNotFound)
			return
		}

		methodType := handlerMethod.Type()
		if methodType.NumIn() != 2 || methodType.NumOut() != 2 {
			http.Error(w, "invalid method signature", http.StatusInternalServerError)
			return
		}

		reqType := methodType.In(1)
		reqPtr := reflectNew(reqType)

		if err := json.Unmarshal(payload, reqPtr.Interface()); err != nil {
			http.Error(w, "failed to parse json request: "+err.Error(), http.StatusBadRequest)
			return
		}

		results := handlerMethod.Call([]reflectValue{
			reflectValueOf(r.Context()),
			reflectValueOf(reqPtr.Interface()),
		})

		var methodErr error
		if !results[1].IsNil() {
			methodErr = results[1].Interface().(error)
		}

		if methodErr != nil {
			http.Error(w, "method execution failed: "+methodErr.Error(), http.StatusInternalServerError)
			return
		}

		respBytes, err := json.Marshal(results[0].Interface())
		if err != nil {
			http.Error(w, "failed to marshal json response", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(respBytes)
		return
	}

	// Dynamic invocation via reflection for the MVP
	// In production, the generated stubs would register a typed handler map to avoid reflection.
	// For our basic blocks, let's look up the method and invoke it.
	// Go stubs implement Method(ctx, req) (resp, error)
	// Let's reflect on the handler
	type ProtoMarshaler interface {
		Marshal() ([]byte, error)
	}
	type ProtoUnmarshaler interface {
		Unmarshal(dAtA []byte) error
	}

	// Read client gRPC frame
	frameHeader := make([]byte, 5)
	if _, err := io.ReadFull(r.Body, frameHeader); err != nil {
		w.Header().Set("grpc-status", "13") // INTERNAL
		w.Header().Set("grpc-message", "failed to read frame header")
		w.WriteHeader(http.StatusOK)
		return
	}

	length := binary.BigEndian.Uint32(frameHeader[1:5])
	payload := make([]byte, length)
	if _, err := io.ReadFull(r.Body, payload); err != nil {
		w.Header().Set("grpc-status", "13")
		w.Header().Set("grpc-message", "failed to read frame payload")
		w.WriteHeader(http.StatusOK)
		return
	}

	// Invoke the service method using reflection
	var respBytes []byte
	var methodErr error

	// We look for a method on the implementation struct that matches the method name.
	// E.g., GetUserProfile(ctx, req) (*UserProfile, error)
	// We dynamically construct the request struct
	// Let's search by name
	importName := serviceName // e.g. "helix_example.UserProfileService"
	_ = importName

	// Let's find reflection type
	// Go generated stubs have interface methods.
	// E.g., GetUserProfile(ctx context.Context, req *UserProfile) (*UserProfile, error)
	// Let's call using reflection
	importName = strings.ReplaceAll(importName, ".", "_") // go package name uses underscore
	
	// We will implement dynamic dispatching:
	// Find the method. 
	// We inspect the method arguments. The 2nd argument (index 1) is the request type pointer.
	// We create a new instance of it, unmarshal payload into it, call the method, and get response.
	// Let's do that cleanly:
	var handlerVal = reflectValueOf(serviceImpl)
	var handlerMethod = handlerVal.MethodByName(methodName)
	if !handlerMethod.IsValid() {
		w.Header().Set("grpc-status", "12") // UNIMPLEMENTED
		w.Header().Set("grpc-message", fmt.Sprintf("method %s not found", methodName))
		w.WriteHeader(http.StatusOK)
		return
	}

	methodType := handlerMethod.Type()
	if methodType.NumIn() != 2 || methodType.NumOut() != 2 {
		w.Header().Set("grpc-status", "13")
		w.Header().Set("grpc-message", "invalid method signature")
		w.WriteHeader(http.StatusOK)
		return
	}

	reqType := methodType.In(1) // ptr to request struct
	// create new instance
	reqPtr := reflectNew(reqType)
	reqUnmarshaler, ok := reqPtr.Interface().(ProtoUnmarshaler)
	if !ok {
		w.Header().Set("grpc-status", "13")
		w.Header().Set("grpc-message", "request does not implement ProtoUnmarshaler")
		w.WriteHeader(http.StatusOK)
		return
	}

	if err := reqUnmarshaler.Unmarshal(payload); err != nil {
		w.Header().Set("grpc-status", "3") // INVALID_ARGUMENT
		w.Header().Set("grpc-message", err.Error())
		w.WriteHeader(http.StatusOK)
		return
	}

	// Call the method
	results := handlerMethod.Call([]reflectValue{
		reflectValueOf(r.Context()),
		reflectValueOf(reqUnmarshaler),
	})

	// Process output
	if !results[1].IsNil() {
		methodErr = results[1].Interface().(error)
	}

	if methodErr != nil {
		w.Header().Set("grpc-status", "13") // INTERNAL
		w.Header().Set("grpc-message", methodErr.Error())
		w.WriteHeader(http.StatusOK)
		return
	}

	respVal := results[0].Interface().(ProtoMarshaler)
	respBytes, err := respVal.Marshal()
	if err != nil {
		w.Header().Set("grpc-status", "13")
		w.Header().Set("grpc-message", "failed to marshal response")
		w.WriteHeader(http.StatusOK)
		return
	}

	// Write response gRPC frame
	w.Header().Set("Content-Type", "application/grpc")
	w.Header().Set("grpc-status", "0") // OK
	w.WriteHeader(http.StatusOK)

	// Write gRPC frame header: 1 byte compressed flag, 4 bytes big endian length
	respHeader := make([]byte, 5)
	respHeader[0] = 0 // uncompressed
	binary.BigEndian.PutUint32(respHeader[1:5], uint32(len(respBytes)))
	w.Write(respHeader)
	w.Write(respBytes)
}

// We write minimal interfaces to avoid importing "reflect" in a way that creates bloat.
// Standard reflection is fully appropriate here.

type reflectValue = reflect.Value

func reflectValueOf(i interface{}) reflectValue {
	return reflect.ValueOf(i)
}

func reflectNew(t reflect.Type) reflectValue {
	return reflect.New(t.Elem())
}

// ChannelListener implements net.Listener for routing sniffed TCP connections to HTTP/2 server
type ChannelListener struct {
	Conns  chan net.Conn
	Closed chan struct{}
	AddrVal net.Addr
}

func NewChannelListener(addr net.Addr) *ChannelListener {
	return &ChannelListener{
		Conns:   make(chan net.Conn, 1024),
		Closed:  make(chan struct{}),
		AddrVal: addr,
	}
}

func (l *ChannelListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.Conns:
		return c, nil
	case <-l.Closed:
		return nil, net.ErrClosed
	}
}

func (l *ChannelListener) Close() error {
	select {
	case <-l.Closed:
	default:
		close(l.Closed)
	}
	return nil
}

func (l *ChannelListener) Addr() net.Addr {
	return l.AddrVal
}

type Server struct {
	Addr           string
	SniffTimeout   time.Duration
	grpcHandler    *GRPCHandler
	channelListener *ChannelListener
	httpServer     *http.Server
	thriftProcessors []ThriftProcessor
}

func NewServer(addr string) *Server {
	return &Server{
		Addr:        addr,
		grpcHandler: NewGRPCHandler(),
	}
}

func (s *Server) RegisterService(name string, impl interface{}) {
	s.grpcHandler.RegisterService(name, impl)
}

func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}

	s.channelListener = NewChannelListener(ln.Addr())

	// Set up HTTP/2 server (H2C support for unencrypted HTTP/2)
	h2s := &http2.Server{}
	handler := h2c.NewHandler(s.grpcHandler, h2s)
	s.httpServer = &http.Server{
		Handler: handler,
	}

	go s.httpServer.Serve(s.channelListener)

	// Set up sniffing listener
	sniffer := NewSniffingListener(ln, func(conn net.Conn) {
		s.channelListener.Conns <- conn
	}, func(conn net.Conn, isCompact bool) {
		// Handle Thrift directly in a go-routine
		// For simplicity, we merge all registered thrift processors.
		// Since stubs implement ThriftProcessor, we will pass them here.
		// Let's invoke the registered thrift processor.
		for _, tp := range s.thriftProcessors {
			go HandleThriftConnection(conn, tp, isCompact)
		}
	})

	if s.SniffTimeout > 0 {
		sniffer.SniffTimeout = s.SniffTimeout
	}

	sniffer.Start()
	return nil
}

func (s *Server) RegisterThriftProcessor(tp ThriftProcessor) {
	s.thriftProcessors = append(s.thriftProcessors, tp)
}
