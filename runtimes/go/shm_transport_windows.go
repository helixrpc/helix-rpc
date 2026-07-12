//go:build windows

package runtime

import (
	"errors"
	"net"
)

// SHMConn is not supported on Windows.
type SHMConn struct {
	net.Conn
}

// NewSHMConn always returns an error on Windows because POSIX shared memory (mmap) is not available.
func NewSHMConn(signalConn net.Conn, shmPath string, size int, isOwner bool) (*SHMConn, error) {
	return nil, errors.New("shared memory transport (SHM) is not supported on Windows")
}

// SHMListener is not supported on Windows.
type SHMListener struct {
	net.Listener
}

// NewSHMListener always returns a stub listener on Windows.
func NewSHMListener(tcpListener net.Listener, shmPath string, shmSize int) *SHMListener {
	return &SHMListener{
		Listener: tcpListener,
	}
}

func (l *SHMListener) Accept() (net.Conn, error) {
	return nil, errors.New("shared memory transport (SHM) is not supported on Windows")
}
