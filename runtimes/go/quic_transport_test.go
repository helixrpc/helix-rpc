package runtime

import (
	"bytes"
	"net"
	"testing"
	"time"
)

func TestQUICTransport(t *testing.T) {
	listener, err := NewQUICTransportListener("127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create QUIC listener: %v", err)
	}
	defer listener.Close()

	addr := listener.Addr().(*net.UDPAddr)

	// Start a goroutine to accept the virtual stream on the server
	serverErrChan := make(chan error, 1)
	serverRecvChan := make(chan []byte, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverErrChan <- err
			return
		}
		defer conn.Close()

		// Read data from client
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			serverErrChan <- err
			return
		}
		serverRecvChan <- buf[:n]

		// Echo back
		_, err = conn.Write([]byte("hello-from-udp-server"))
		if err != nil {
			serverErrChan <- err
		}
	}()

	// 1. Dial client UDP connection
	clientConn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		t.Fatalf("failed to dial UDP: %v", err)
	}
	defer clientConn.Close()

	// 2. Write virtual stream packet (Stream ID = 42)
	payload := []byte("hello-from-udp-client")
	packet := make([]byte, 4+len(payload))
	// Stream ID = 42
	packet[0] = 0
	packet[1] = 0
	packet[2] = 0
	packet[3] = 42
	copy(packet[4:], payload)

	_, err = clientConn.Write(packet)
	if err != nil {
		t.Fatalf("failed to write client packet: %v", err)
	}

	// 3. Wait for server to receive and verify payload
	select {
	case err := <-serverErrChan:
		t.Fatalf("server error: %v", err)
	case received := <-serverRecvChan:
		if !bytes.Equal(received, payload) {
			t.Errorf("expected %s, got %s", string(payload), string(received))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for server read")
	}

	// 4. Client reads the echoed back data
	readBuf := make([]byte, 1024)
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := clientConn.ReadFromUDP(readBuf)
	if err != nil {
		t.Fatalf("failed to read from server: %v", err)
	}

	if n < 4 {
		t.Fatalf("read packet too small: %d bytes", n)
	}

	streamID := uint32(readBuf[0])<<24 | uint32(readBuf[1])<<16 | uint32(readBuf[2])<<8 | uint32(readBuf[3])
	if streamID != 42 {
		t.Errorf("expected Stream ID 42, got %d", streamID)
	}

	resp := readBuf[4:n]
	expected := []byte("hello-from-udp-server")
	if !bytes.Equal(resp, expected) {
		t.Errorf("expected %s, got %s", string(expected), string(resp))
	}
}
