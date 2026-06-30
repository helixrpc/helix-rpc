package runtime

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

type MethodHandler func(ctx context.Context, dec func(interface{}) error) (interface{}, error)

type GRPCHandler struct {
	handlers map[string]MethodHandler
}

func NewGRPCHandler() *GRPCHandler {
	return &GRPCHandler{
		handlers: make(map[string]MethodHandler),
	}
}

func (h *GRPCHandler) RegisterMethod(path string, handler MethodHandler) {
	h.handlers[path] = handler
}

func (h *GRPCHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	handler, ok := h.handlers[path]
	if !ok {
		w.Header().Set("grpc-status", "12") // UNIMPLEMENTED
		w.Header().Set("grpc-message", fmt.Sprintf("path %s not found", path))
		w.WriteHeader(http.StatusOK)
		return
	}

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

		resp, err := handler(r.Context(), dec)
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

	resp, err := handler(r.Context(), dec)
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

func (s *Server) RegisterMethod(path string, handler MethodHandler) {
	s.grpcHandler.RegisterMethod(path, handler)
}

func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}

	s.channelListener = NewChannelListener(ln.Addr())

	h2s := &http2.Server{}
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
