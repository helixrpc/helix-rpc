package runtime

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
)

// ClientConnPool manages a pool of TCP connections to a specific address.
type ClientConnPool struct {
	addr  string
	conns chan net.Conn
	mu    sync.Mutex
}

func NewClientConnPool(addr string, maxIdle int) *ClientConnPool {
	return &ClientConnPool{
		addr:  addr,
		conns: make(chan net.Conn, maxIdle),
	}
}

// Get retrieves a connection from the pool, or dials a new one if the pool is empty.
func (p *ClientConnPool) Get() (net.Conn, error) {
	select {
	case conn := <-p.conns:
		return conn, nil
	default:
		// Dial new connection
		var conn net.Conn
		var err error
		if hasUnixPrefix(p.addr) || (len(p.addr) > 0 && (p.addr[0] == '/' || p.addr[0] == '.')) {
			conn, err = net.Dial("unix", stripUnixPrefix(p.addr))
		} else {
			conn, err = net.Dial("tcp", p.addr)
		}
		if err != nil {
			return nil, err
		}
		return conn, nil
	}
}

// Put returns a connection to the pool, or closes it if the pool is full.
func (p *ClientConnPool) Put(conn net.Conn) {
	if conn == nil {
		return
	}
	select {
	case p.conns <- conn:
	default:
		// Pool is full, close connection
		_ = conn.Close()
	}
}

// Close closes all idle connections in the pool.
func (p *ClientConnPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	close(p.conns)
	for conn := range p.conns {
		if conn != nil {
			_ = conn.Close()
		}
	}
	return nil
}

// Balancer defines an interface for client-side load balancing.
type Balancer interface {
	Next(targets []string) (string, error)
}

// RoundRobinBalancer implements simple thread-safe round-robin balancing.
type RoundRobinBalancer struct {
	counter uint64
}

func NewRoundRobinBalancer() *RoundRobinBalancer {
	return &RoundRobinBalancer{}
}

func (b *RoundRobinBalancer) Next(targets []string) (string, error) {
	if len(targets) == 0 {
		return "", fmt.Errorf("no targets available for load balancing")
	}
	val := atomic.AddUint64(&b.counter, 1)
	idx := int((val - 1) % uint64(len(targets)))
	return targets[idx], nil
}
