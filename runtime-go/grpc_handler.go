package runtime

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

type UnaryServerInfo struct {
	FullMethod string
}

type UnaryHandler func(ctx context.Context, req interface{}) (interface{}, error)

type UnaryServerInterceptor func(ctx context.Context, req interface{}, info *UnaryServerInfo, handler UnaryHandler) (resp interface{}, err error)

type MethodInfo struct {
	Decoder func(dec func(interface{}) error) (interface{}, error)
	Handler func(ctx context.Context, req interface{}) (interface{}, error)
}

type GRPCHandler struct {
	methods      map[string]MethodInfo
	interceptors []UnaryServerInterceptor
	chained      UnaryServerInterceptor
}

func NewGRPCHandler() *GRPCHandler {
	return &GRPCHandler{
		methods: make(map[string]MethodInfo),
	}
}

func (h *GRPCHandler) RegisterMethod(path string, methodInfo MethodInfo) {
	h.methods[path] = methodInfo
}

func (h *GRPCHandler) AddInterceptor(interceptor UnaryServerInterceptor) {
	h.interceptors = append(h.interceptors, interceptor)
	h.chained = chainInterceptors(h.interceptors)
}

func chainInterceptors(interceptors []UnaryServerInterceptor) UnaryServerInterceptor {
	if len(interceptors) == 0 {
		return nil
	}
	if len(interceptors) == 1 {
		return interceptors[0]
	}
	return func(ctx context.Context, req interface{}, info *UnaryServerInfo, handler UnaryHandler) (interface{}, error) {
		return interceptors[0](ctx, req, info, getChainHandler(interceptors, 0, info, handler))
	}
}

func getChainHandler(interceptors []UnaryServerInterceptor, curr int, info *UnaryServerInfo, finalHandler UnaryHandler) UnaryHandler {
	if curr == len(interceptors)-1 {
		return finalHandler
	}
	return func(ctx context.Context, req interface{}) (interface{}, error) {
		return interceptors[curr+1](ctx, req, info, getChainHandler(interceptors, curr+1, info, finalHandler))
	}
}

func (h *GRPCHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	methodInfo, ok := h.methods[path]
	if !ok {
		w.Header().Set("grpc-status", "12") // UNIMPLEMENTED
		w.Header().Set("grpc-message", fmt.Sprintf("path %s not found", path))
		w.WriteHeader(http.StatusOK)
		return
	}

	// Extract metadata from request headers
	md := make(MD)
	for k, v := range r.Header {
		md[strings.ToLower(k)] = v
	}
	ctx := NewContext(r.Context(), md)

	type ProtoMarshaler interface {
		Marshal() ([]byte, error)
	}
	type ProtoUnmarshaler interface {
		Unmarshal(dAtA []byte) error
	}

	contentType := r.Header.Get("Content-Type")

	if contentType == "application/json" {
		dec := func(v interface{}) error {
			payload, err := io.ReadAll(r.Body)
			if err != nil {
				return err
			}
			return json.Unmarshal(payload, v)
		}

		req, err := methodInfo.Decoder(dec)
		if err != nil {
			http.Error(w, "failed to decode request: "+err.Error(), http.StatusBadRequest)
			return
		}

		var resp interface{}
		if h.chained != nil {
			info := &UnaryServerInfo{FullMethod: path}
			resp, err = h.chained(ctx, req, info, methodInfo.Handler)
		} else {
			resp, err = methodInfo.Handler(ctx, req)
		}

		if err != nil {
			http.Error(w, "method execution failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		respBytes, err := json.Marshal(resp)
		if err != nil {
			http.Error(w, "failed to marshal json response", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(respBytes)
		return
	}

	// Default fallback: gRPC (HTTP/2)
	dec := func(v interface{}) error {
		frameHeader := make([]byte, 5)
		if _, err := io.ReadFull(r.Body, frameHeader); err != nil {
			return err
		}
		length := binary.BigEndian.Uint32(frameHeader[1:5])
		payload := make([]byte, length)
		if _, err := io.ReadFull(r.Body, payload); err != nil {
			return err
		}
		unmarshaler, ok := v.(ProtoUnmarshaler)
		if !ok {
			return fmt.Errorf("type does not implement ProtoUnmarshaler")
		}
		return unmarshaler.Unmarshal(payload)
	}

	req, err := methodInfo.Decoder(dec)
	if err != nil {
		w.Header().Set("grpc-status", "3") // INVALID_ARGUMENT
		w.Header().Set("grpc-message", err.Error())
		w.WriteHeader(http.StatusOK)
		return
	}

	var resp interface{}
	if h.chained != nil {
		info := &UnaryServerInfo{FullMethod: path}
		resp, err = h.chained(ctx, req, info, methodInfo.Handler)
	} else {
		resp, err = methodInfo.Handler(ctx, req)
	}

	if err != nil {
		w.Header().Set("grpc-status", "13") // INTERNAL
		w.Header().Set("grpc-message", err.Error())
		w.WriteHeader(http.StatusOK)
		return
	}

	marshaler, ok := resp.(ProtoMarshaler)
	if !ok {
		w.Header().Set("grpc-status", "13")
		w.Header().Set("grpc-message", "response does not implement ProtoMarshaler")
		w.WriteHeader(http.StatusOK)
		return
	}

	payload, err := marshaler.Marshal()
	if err != nil {
		w.Header().Set("grpc-status", "13")
		w.Header().Set("grpc-message", err.Error())
		w.WriteHeader(http.StatusOK)
		return
	}

	respFrame := make([]byte, 5+len(payload))
	respFrame[0] = 0 // uncompressed
	binary.BigEndian.PutUint32(respFrame[1:5], uint32(len(payload)))

	w.Header().Set("Content-Type", "application/grpc")
	w.Header().Set("grpc-status", "0") // OK
	w.WriteHeader(http.StatusOK)

	w.Write(respFrame[:5])
	w.Write(payload)
}

// ChannelListener implements net.Listener for routing sniffed TCP connections to HTTP/2 server
type ChannelListener struct {
	Conns   chan net.Conn
	Closed  chan struct{}
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
	Addr             string
	SniffTimeout     time.Duration
	grpcHandler      *GRPCHandler
	channelListener  *ChannelListener
	httpServer       *http.Server
	thriftProcessors []ThriftProcessor
}

func NewServer(addr string) *Server {
	return &Server{
		Addr:        addr,
		grpcHandler: NewGRPCHandler(),
	}
}

func (s *Server) RegisterMethod(path string, methodInfo MethodInfo) {
	s.grpcHandler.RegisterMethod(path, methodInfo)
}

func (s *Server) AddInterceptor(interceptor UnaryServerInterceptor) {
	s.grpcHandler.AddInterceptor(interceptor)
}

func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}

	s.channelListener = NewChannelListener(ln.Addr())

	h2s := &http2.Server{
		MaxConcurrentStreams:         250,
		MaxUploadBufferPerConnection: 1024 * 1024 * 2, // 2MB
		MaxUploadBufferPerStream:     1024 * 1024,     // 1MB
	}
	handler := h2c.NewHandler(s.grpcHandler, h2s)
	s.httpServer = &http.Server{
		Handler: handler,
	}

	go s.httpServer.Serve(s.channelListener)

	sniffer := NewSniffingListener(ln, func(conn net.Conn) {
		s.channelListener.Conns <- conn
	}, func(conn net.Conn, isCompact bool) {
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
