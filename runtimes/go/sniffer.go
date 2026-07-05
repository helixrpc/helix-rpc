package runtime

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"net"
	"sync"
	"time"
)

type SniffingListener struct {
	net.Listener
	SniffTimeout  time.Duration
	TLSConfig     *tls.Config
	gRPCHandler   func(net.Conn)
	thriftHandler func(net.Conn, bool) // connection, isCompact
}

func NewSniffingListener(inner net.Listener, grpcHandler func(net.Conn), thriftHandler func(net.Conn, bool)) *SniffingListener {
	return &SniffingListener{
		Listener:      inner,
		SniffTimeout:  100 * time.Millisecond,
		gRPCHandler:   grpcHandler,
		thriftHandler: thriftHandler,
	}
}

func (l *SniffingListener) Start() {
	var wg sync.WaitGroup
	for {
		conn, err := l.Accept()
		if err != nil {
			break
		}
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			_ = tcpConn.SetKeepAlive(true)
			_ = tcpConn.SetKeepAlivePeriod(3 * time.Minute)
		}
		wg.Add(1)
		go func(c net.Conn) {
			defer wg.Done()
			l.sniffAndRoute(c)
		}(conn)
	}
	wg.Wait()
}

func (l *SniffingListener) sniffAndRoute(conn net.Conn) {
	_ = conn.SetReadDeadline(time.Now().Add(l.SniffTimeout))
	br := bufio.NewReader(conn)
	magic, err := br.Peek(8)
	if err != nil {
		// Fallback to smaller peek in case payload is very short
		magic, err = br.Peek(4)
	}
	_ = conn.SetReadDeadline(time.Time{}) // Clear timeout

	if err != nil {
		conn.Close()
		return
	}

	// Detect TLS Handshake (first byte is 0x16)
	if len(magic) >= 1 && magic[0] == 0x16 && l.TLSConfig != nil {
		bufferedRaw := &BufferedConn{Conn: conn, r: br}
		tlsConn := tls.Server(bufferedRaw, l.TLSConfig)
		if err := tlsConn.Handshake(); err != nil {
			conn.Close()
			return
		}
		l.sniffAndRoute(tlsConn)
		return
	}

	wrappedConn := &BufferedConn{Conn: conn, r: br}

	// 1. Check HTTP/2 (gRPC) magic connection preface "PRI "
	if len(magic) >= 4 && bytes.Equal(magic[:4], []byte("PRI ")) {
		l.gRPCHandler(wrappedConn)
		return
	}

	// 2. Check Thrift Compact (Unframed) magic "0x82"
	if len(magic) >= 1 && magic[0] == 0x82 {
		l.thriftHandler(wrappedConn, true)
		return
	}

	// 3. Check Thrift Binary (Unframed) magic "0x80 0x01"
	if len(magic) >= 2 && magic[0] == 0x80 && magic[1] == 0x01 {
		l.thriftHandler(wrappedConn, false)
		return
	}

	// 4. Check Thrift Framed: First 4 bytes is frame size, next bytes are protocol magic
	if len(magic) >= 6 && magic[0] == 0x00 && magic[1] == 0x00 {
		// Check 5th byte (magic[4]) for Compact magic
		if magic[4] == 0x82 {
			l.thriftHandler(wrappedConn, true)
			return
		}
		// Check 5th & 6th bytes for Binary magic
		if magic[4] == 0x80 && magic[5] == 0x01 {
			l.thriftHandler(wrappedConn, false)
			return
		}
	}

	// 5. Check HTTP/1.1 (REST) - standard HTTP verbs (allocation-free check)
	if len(magic) >= 3 {
		m0, m1, m2 := magic[0], magic[1], magic[2]
		if (m0 == 'G' && m1 == 'E' && m2 == 'T') ||
			(m0 == 'P' && m1 == 'O' && m2 == 'S') ||
			(m0 == 'P' && m1 == 'U' && m2 == 'T') ||
			(m0 == 'D' && m1 == 'E' && m2 == 'L') {
			l.gRPCHandler(wrappedConn)
			return
		}
	}

	// Default fallback: close
	conn.Close()
}

type BufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *BufferedConn) Read(b []byte) (int, error) {
	return c.r.Read(b)
}
