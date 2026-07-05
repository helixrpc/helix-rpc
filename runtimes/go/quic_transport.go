package runtime

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// QUICTransportListener implements a high-performance UDP-based QUIC connection sniffer listener.
type QUICTransportListener struct {
	addr      *net.UDPAddr
	conn      *net.UDPConn
	streams   chan net.Conn
	closed    chan struct{}
	mu        sync.Mutex
	activeMap map[string]*QUICStreamConn
}

// NewQUICTransportListener creates a new UDP-based stream listener.
func NewQUICTransportListener(addrStr string) (*QUICTransportListener, error) {
	addr, err := net.ResolveUDPAddr("udp", addrStr)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}

	l := &QUICTransportListener{
		addr:      addr,
		conn:      conn,
		streams:   make(chan net.Conn, 1024),
		closed:    make(chan struct{}),
		activeMap: make(map[string]*QUICStreamConn),
	}
	go l.readLoop()
	return l, nil
}

func (l *QUICTransportListener) readLoop() {
	buf := make([]byte, 65535)
	for {
		n, remoteAddr, err := l.conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-l.closed:
				return
			default:
				continue
			}
		}

		if n < 4 {
			continue // Invalid packet
		}

		// Mock QUIC Stream Protocol:
		// First 4 bytes: Stream ID (uint32)
		// Rest: Payload
		streamID := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])
		key := fmt.Sprintf("%s:%d", remoteAddr.String(), streamID)

		l.mu.Lock()
		sConn, exists := l.activeMap[key]
		if !exists {
			// Create a virtual stream connection for this remote endpoint
			sConn = &QUICStreamConn{
				localAddr:  l.conn.LocalAddr(),
				remoteAddr: remoteAddr,
				streamID:   streamID,
				readChan:   make(chan []byte, 256),
				closed:     make(chan struct{}),
				writeFn: func(payload []byte) error {
					// Prepend Stream ID
					packet := make([]byte, 4+len(payload))
					packet[0] = byte(streamID >> 24)
					packet[1] = byte(streamID >> 16)
					packet[2] = byte(streamID >> 8)
					packet[3] = byte(streamID)
					copy(packet[4:], payload)
					_, err := l.conn.WriteToUDP(packet, remoteAddr)
					return err
				},
			}
			l.activeMap[key] = sConn
			l.streams <- sConn
		}
		l.mu.Unlock()

		payload := make([]byte, n-4)
		copy(payload, buf[4:n])
		select {
		case sConn.readChan <- payload:
		default:
			// Buffer full, drop packet (standard UDP behavior)
		}
	}
}

func (l *QUICTransportListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.streams:
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *QUICTransportListener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	select {
	case <-l.closed:
		return nil
	default:
		close(l.closed)
		_ = l.conn.Close()
		for _, s := range l.activeMap {
			s.Close()
		}
	}
	return nil
}

func (l *QUICTransportListener) Addr() net.Addr {
	return l.conn.LocalAddr()
}

// QUICStreamConn implements net.Conn over a virtual UDP multiplexed stream.
type QUICStreamConn struct {
	localAddr  net.Addr
	remoteAddr net.Addr
	streamID   uint32
	readChan   chan []byte
	closed     chan struct{}
	writeFn    func([]byte) error
	leftover   []byte
	mu         sync.Mutex
}

func (c *QUICStreamConn) Read(b []byte) (n int, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.leftover) > 0 {
		n = copy(b, c.leftover)
		c.leftover = c.leftover[n:]
		return n, nil
	}

	select {
	case packet := <-c.readChan:
		n = copy(b, packet)
		if n < len(packet) {
			c.leftover = packet[n:]
		}
		return n, nil
	case <-c.closed:
		return 0, net.ErrClosed
	}
}

func (c *QUICStreamConn) Write(b []byte) (n int, err error) {
	err = c.writeFn(b)
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

func (c *QUICStreamConn) Close() error {
	select {
	case <-c.closed:
	default:
		close(c.closed)
	}
	return nil
}

func (c *QUICStreamConn) LocalAddr() net.Addr                { return c.localAddr }
func (c *QUICStreamConn) RemoteAddr() net.Addr               { return c.remoteAddr }
func (c *QUICStreamConn) SetDeadline(t time.Time) error      { return nil }
func (c *QUICStreamConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *QUICStreamConn) SetWriteDeadline(t time.Time) error { return nil }
