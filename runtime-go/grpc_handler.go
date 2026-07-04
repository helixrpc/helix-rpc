package runtime

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

type ProtoMarshaler interface {
	Marshal() ([]byte, error)
}

type ProtoUnmarshaler interface {
	Unmarshal([]byte) error
}

type UnaryServerInfo struct {
	FullMethod string
}

type UnaryHandler func(ctx context.Context, req interface{}) (interface{}, error)

type UnaryServerInterceptor func(ctx context.Context, req interface{}, info *UnaryServerInfo, handler UnaryHandler) (resp interface{}, err error)

type ServerStream interface {
	Context() context.Context
	Recv(v interface{}) error
	Send(v interface{}) error
}

type serverStream struct {
	ctx context.Context
	w   http.ResponseWriter
	r   *http.Request
}

func (s *serverStream) Context() context.Context {
	return s.ctx
}

func (s *serverStream) Recv(v interface{}) error {
	frameHeader := make([]byte, 5)
	if _, err := io.ReadFull(s.r.Body, frameHeader); err != nil {
		return err
	}
	length := binary.BigEndian.Uint32(frameHeader[1:5])
	payload := make([]byte, length)
	if _, err := io.ReadFull(s.r.Body, payload); err != nil {
		return err
	}
	unmarshaler, ok := v.(ProtoUnmarshaler)
	if !ok {
		return fmt.Errorf("type does not implement ProtoUnmarshaler")
	}
	return unmarshaler.Unmarshal(payload)
}

func (s *serverStream) Send(v interface{}) error {
	marshaler, ok := v.(ProtoMarshaler)
	if !ok {
		return fmt.Errorf("type does not implement ProtoMarshaler")
	}
	payload, err := marshaler.Marshal()
	if err != nil {
		return err
	}

	header := make([]byte, 5)
	header[0] = 0 // uncompressed
	binary.BigEndian.PutUint32(header[1:5], uint32(len(payload)))

	if _, err := s.w.Write(header); err != nil {
		return err
	}
	if _, err := s.w.Write(payload); err != nil {
		return err
	}

	if flusher, ok := s.w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

type sseServerStream struct {
	ctx     context.Context
	w       http.ResponseWriter
	payload []byte
	read    bool
}

func (s *sseServerStream) Context() context.Context {
	return s.ctx
}

func (s *sseServerStream) Recv(v interface{}) error {
	if s.read {
		return io.EOF
	}
	s.read = true
	return json.Unmarshal(s.payload, v)
}

func (s *sseServerStream) Send(v interface{}) error {
	respBytes, err := json.Marshal(v)
	if err != nil {
		return err
	}
	msg := fmt.Sprintf("data: %s\n\n", string(respBytes))
	if _, err := s.w.Write([]byte(msg)); err != nil {
		return err
	}
	if flusher, ok := s.w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

type MethodInfo struct {
	Decoder       func(dec func(interface{}) error) (interface{}, error)
	Handler       func(ctx context.Context, req interface{}) (interface{}, error)
	Binder        func(req interface{}, params map[string]string) error
	IsStreaming   bool
	StreamHandler func(stream ServerStream) error
}

type RESTRoute struct {
	Method      string
	Pattern     string
	PathParts   []string
	HandlerPath string
}

type GRPCHandler struct {
	methods      map[string]MethodInfo
	restRoutes   []RESTRoute
	interceptors []UnaryServerInterceptor
	chained      UnaryServerInterceptor
	debugMux     *http.ServeMux
}

func NewGRPCHandler() *GRPCHandler {
	return &GRPCHandler{
		methods:  make(map[string]MethodInfo),
		debugMux: http.NewServeMux(),
	}
}

func (h *GRPCHandler) RegisterMethod(path string, methodInfo MethodInfo) {
	h.methods[path] = methodInfo
}

func (h *GRPCHandler) RegisterRESTRoute(method, pattern, handlerPath string) {
	h.restRoutes = append(h.restRoutes, RESTRoute{
		Method:      strings.ToUpper(method),
		Pattern:     pattern,
		PathParts:   splitPath(pattern),
		HandlerPath: handlerPath,
	})
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

	if strings.HasPrefix(path, "/__helix/") || path == "/metrics" || path == "/metrics/" {
		h.debugMux.ServeHTTP(w, r)
		return
	}

	methodPath := path
	var pathParams map[string]string

	if route, params := matchREST(r.Method, path, h.restRoutes); route != nil {
		methodPath = route.HandlerPath
		pathParams = params
		if pathParams == nil {
			pathParams = make(map[string]string)
		}
		for qKey, qVals := range r.URL.Query() {
			if len(qVals) > 0 {
				pathParams[qKey] = qVals[0]
			}
		}
	}

	methodInfo, ok := h.methods[methodPath]
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
	baseCtx := NewContext(r.Context(), md)
	ctx, cancel := contextWithDeadlineFromHeaders(baseCtx, r.Header)
	defer cancel()

	type ProtoMarshaler interface {
		Marshal() ([]byte, error)
	}
	type ProtoUnmarshaler interface {
		Unmarshal(dAtA []byte) error
	}

	contentType := r.Header.Get("Content-Type")
	grpcEncoding := r.Header.Get("grpc-encoding")

	// If HTTP/1.1 REST is caller, default to application/json if Content-Type is missing
	if contentType == "" {
		contentType = "application/json"
	}

	if strings.Contains(contentType, "application/json") && !methodInfo.IsStreaming {
		dec := func(v interface{}) error {
			payload, err := io.ReadAll(r.Body)
			if err != nil && err != io.EOF {
				return err
			}
			// If empty body, init with empty json object so unmarshal doesn't fail
			if len(payload) == 0 {
				payload = []byte("{}")
			}
			return json.Unmarshal(payload, v)
		}

		req, err := methodInfo.Decoder(dec)
		if err != nil {
			http.Error(w, "failed to decode request: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Bind path parameters statically
		if len(pathParams) > 0 && methodInfo.Binder != nil {
			if err := methodInfo.Binder(req, pathParams); err != nil {
				http.Error(w, "failed to bind path parameters: "+err.Error(), http.StatusBadRequest)
				return
			}
		}

		var resp interface{}
		if h.chained != nil {
			info := &UnaryServerInfo{FullMethod: methodPath}
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

	if methodInfo.IsStreaming {
		isSSE := strings.Contains(r.Header.Get("Accept"), "text/event-stream") || strings.Contains(contentType, "application/json")
		if isSSE {
			payload, err := io.ReadAll(r.Body)
			if err != nil && err != io.EOF {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if len(payload) == 0 {
				payload = []byte("{}")
			}

			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.WriteHeader(http.StatusOK)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}

			stream := &sseServerStream{
				ctx:     ctx,
				w:       w,
				payload: payload,
			}

			if err := methodInfo.StreamHandler(stream); err != nil {
				errBytes, _ := json.Marshal(map[string]string{"error": err.Error()})
				fmt.Fprintf(w, "data: %s\n\n", string(errBytes))
			}
			fmt.Fprintf(w, "data: [DONE]\n\n")
			return
		}

		w.Header().Set("Content-Type", "application/grpc")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}

		stream := &serverStream{
			ctx: ctx,
			w:   w,
			r:   r,
		}

		if err := methodInfo.StreamHandler(stream); err != nil {
			w.Header().Set("grpc-status", "13") // INTERNAL
			w.Header().Set("grpc-message", err.Error())
			return
		}

		w.Header().Set("grpc-status", "0") // OK
		return
	}

	// Default fallback: gRPC (HTTP/2)
	dec := func(v interface{}) error {
		frameHeader := make([]byte, 5)
		if _, err := io.ReadFull(r.Body, frameHeader); err != nil {
			return err
		}
		compressedFlag := frameHeader[0]
		length := binary.BigEndian.Uint32(frameHeader[1:5])
		payload := make([]byte, length)
		if _, err := io.ReadFull(r.Body, payload); err != nil {
			return err
		}
		// Decompress if compressed flag is set
		if compressedFlag == 1 && grpcEncoding != "" {
			comp := getCompressor(grpcEncoding)
			if comp != nil {
				decompressed, err := comp.Decompress(payload)
				if err != nil {
					return fmt.Errorf("failed to decompress payload: %w", err)
				}
				payload = decompressed
			}
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
		info := &UnaryServerInfo{FullMethod: methodPath}
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

	// Compress response if client sent grpc-encoding
	respPayload := payload
	compressFlag := byte(0)
	if grpcEncoding != "" {
		comp := getCompressor(grpcEncoding)
		if comp != nil {
			compressed, cerr := comp.Compress(payload)
			if cerr == nil {
				respPayload = compressed
				compressFlag = 1
			}
		}
	}

	respFrame := make([]byte, 5+len(respPayload))
	respFrame[0] = compressFlag
	binary.BigEndian.PutUint32(respFrame[1:5], uint32(len(respPayload)))

	w.Header().Set("Content-Type", "application/grpc")
	if compressFlag == 1 {
		w.Header().Set("grpc-encoding", grpcEncoding)
	}
	w.Header().Set("grpc-status", "0") // OK
	w.WriteHeader(http.StatusOK)

	w.Write(respFrame[:5])
	w.Write(respPayload)
}

func matchREST(method, path string, routes []RESTRoute) (*RESTRoute, map[string]string) {
	reqParts := splitPath(path)
	method = strings.ToUpper(method)

	for _, r := range routes {
		if r.Method != method || len(r.PathParts) != len(reqParts) {
			continue
		}
		match := true
		params := make(map[string]string)
		for i, part := range r.PathParts {
			if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
				paramName := part[1 : len(part)-1]
				params[paramName] = reqParts[i]
			} else if part != reqParts[i] {
				match = false
				break
			}
		}
		if match {
			return &r, params
		}
	}
	return nil, nil
}

func splitPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
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
	Health           *HealthChecker
	TLSConfig        *tls.Config
	grpcHandler      *GRPCHandler
	channelListener  *ChannelListener
	httpServer       *http.Server
	thriftProcessors []ThriftProcessor
	activeConns      sync.WaitGroup
	snifferListener  net.Listener
	mu               sync.Mutex
	inShutdown       bool
	balancer         *LeastConnBalancer
	balancerTargets  []string
	debugBreaker     *CircuitBreaker
}

type ServerConfig struct {
	SniffTimeout   time.Duration
	DisableHealth  bool
	DisableMetrics bool
	DisableDebugUI bool
	TLSConfig      *tls.Config
}

func NewServerWithConfig(addr string, config ServerConfig) *Server {
	var hc *HealthChecker
	handler := NewGRPCHandler()
	if !config.DisableHealth {
		hc = NewHealthChecker()
		RegisterHealthMethods(handler, hc)
	}

	s := &Server{
		Addr:         addr,
		SniffTimeout: config.SniffTimeout,
		Health:       hc,
		TLSConfig:    config.TLSConfig,
		grpcHandler:  handler,
	}

	if !config.DisableDebugUI || !config.DisableMetrics {
		MountDebugHandler(handler.debugMux, s)
	}
	if !config.DisableMetrics {
		MountMetricsHandler(handler.debugMux)
	}
	if !config.DisableDebugUI {
		MountDebugUI(handler.debugMux)
	}
	return s
}

func NewServer(addr string) *Server {
	return NewServerWithConfig(addr, ServerConfig{})
}

func (s *Server) RegisterMethod(path string, methodInfo MethodInfo) {
	s.grpcHandler.RegisterMethod(path, methodInfo)
}

func (s *Server) RegisterRESTRoute(method, pattern, handlerPath string) {
	s.grpcHandler.RegisterRESTRoute(method, pattern, handlerPath)
}

func (s *Server) AddInterceptor(interceptor UnaryServerInterceptor) {
	s.grpcHandler.AddInterceptor(interceptor)
}

func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.snifferListener = ln
	s.mu.Unlock()

	s.channelListener = NewChannelListener(ln.Addr())

	h2s := &http2.Server{
		MaxConcurrentStreams:         250,
		MaxUploadBufferPerConnection: 1024 * 1024 * 2, // 2MB
		MaxUploadBufferPerStream:     1024 * 1024,     // 1MB
		IdleTimeout:                  3 * time.Minute,
	}
	handler := h2c.NewHandler(s.grpcHandler, h2s)
	s.httpServer = &http.Server{
		Handler: handler,
	}

	go s.httpServer.Serve(s.channelListener)

	sniffer := NewSniffingListener(ln, func(conn net.Conn) {
		s.channelListener.Conns <- conn
	}, func(conn net.Conn, isCompact bool) {
		s.mu.Lock()
		if s.inShutdown {
			s.mu.Unlock()
			conn.Close()
			return
		}
		s.mu.Unlock()

		for _, tp := range s.thriftProcessors {
			s.activeConns.Add(1)
			go func(tpVal ThriftProcessor) {
				defer s.activeConns.Done()
				HandleThriftConnection(conn, tpVal, isCompact)
			}(tp)
		}
	})
	sniffer.TLSConfig = s.TLSConfig

	if s.SniffTimeout > 0 {
		sniffer.SniffTimeout = s.SniffTimeout
	}

	sniffer.Start()
	return nil
}

func (s *Server) RegisterThriftProcessor(tp ThriftProcessor) {
	s.thriftProcessors = append(s.thriftProcessors, tp)
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	if s.inShutdown {
		s.mu.Unlock()
		return nil
	}
	s.inShutdown = true
	s.mu.Unlock()

	var err error
	if s.snifferListener != nil {
		err = s.snifferListener.Close()
	}

	if s.channelListener != nil {
		s.channelListener.Close()
	}

	if s.httpServer != nil {
		if httpErr := s.httpServer.Shutdown(ctx); httpErr != nil {
			err = httpErr
		}
	}

	c := make(chan struct{})
	go func() {
		s.activeConns.Wait()
		close(c)
	}()

	select {
	case <-c:
	case <-ctx.Done():
		return ctx.Err()
	}

	return err
}
