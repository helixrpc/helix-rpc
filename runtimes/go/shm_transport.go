package runtime

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"sync"
	"syscall"
)

// SHMConn implements net.Conn using memory-mapped files for data transfer
// and standard TCP sockets for signaling event wakeups.
type SHMConn struct {
	net.Conn // the signaling connection
	mmapData []byte
	mmapFile *os.File
	mu       sync.Mutex
}

func NewSHMConn(signalConn net.Conn, shmPath string, size int, isOwner bool) (*SHMConn, error) {
	var file *os.File
	var err error

	if isOwner {
		file, err = os.OpenFile(shmPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
		if err != nil {
			return nil, err
		}
		if err = file.Truncate(int64(size)); err != nil {
			file.Close()
			return nil, err
		}
	} else {
		file, err = os.OpenFile(shmPath, os.O_RDWR, 0666)
		if err != nil {
			return nil, err
		}
	}

	data, err := syscall.Mmap(int(file.Fd()), 0, size, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		file.Close()
		return nil, err
	}

	return &SHMConn{
		Conn:     signalConn,
		mmapData: data,
		mmapFile: file,
	}, nil
}

func (c *SHMConn) Read(b []byte) (int, error) {
	// Read 8-byte metadata header (offset, length) from the signaling socket
	header := make([]byte, 8)
	if _, err := ioReadFull(c.Conn, header); err != nil {
		return 0, err
	}

	offset := binary.BigEndian.Uint32(header[0:4])
	length := binary.BigEndian.Uint32(header[4:8])

	if int(offset+length) > len(c.mmapData) {
		return 0, fmt.Errorf("shm read out of bounds: offset=%d length=%d mmap_size=%d", offset, length, len(c.mmapData))
	}

	// Copy from shared memory diretamente into the destination slice
	n := copy(b, c.mmapData[offset:offset+length])
	return n, nil
}

func (c *SHMConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	length := uint32(len(b))
	if int(length) > len(c.mmapData) {
		return 0, fmt.Errorf("data length exceeds shm capacity: %d > %d", length, len(c.mmapData))
	}

	// For simplicity, always write starting at offset 0 (ping-pong model per connection)
	const offset uint32 = 0

	// Write payload into memory mapping
	copy(c.mmapData[offset:offset+length], b)

	// Send notification (offset, length) via signaling socket
	header := make([]byte, 8)
	binary.BigEndian.PutUint32(header[0:4], offset)
	binary.BigEndian.PutUint32(header[4:8], length)

	if _, err := c.Conn.Write(header); err != nil {
		return 0, err
	}

	return len(b), nil
}

func (c *SHMConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	_ = syscall.Munmap(c.mmapData)
	if c.mmapFile != nil {
		_ = c.mmapFile.Close()
	}
	return c.Conn.Close()
}

// Helper to ensure full reads
func ioReadFull(r net.Conn, buf []byte) (int, error) {
	n := 0
	for n < len(buf) {
		read, err := r.Read(buf[n:])
		if err != nil {
			return n, err
		}
		n += read
	}
	return n, nil
}

// SHMListener wraps net.Listener to yield SHM connections
type SHMListener struct {
	net.Listener
	shmPath string
	shmSize int
}

func NewSHMListener(tcpListener net.Listener, shmPath string, shmSize int) *SHMListener {
	return &SHMListener{
		Listener: tcpListener,
		shmPath:  shmPath,
		shmSize:  shmSize,
	}
}

func (l *SHMListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return NewSHMConn(conn, l.shmPath, l.shmSize, true)
}
