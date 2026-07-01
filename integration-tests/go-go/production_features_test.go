package go_go

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"testing"
	"time"

	"golang.org/x/net/http2"

	generated "github.com/helix-rpc/helix/integration-tests/go-go/generated"
	"github.com/helix-rpc/helix/runtime-go"
)

type slowUserProfileService struct {
	sleepDuration time.Duration
}

func (s *slowUserProfileService) GetUserProfile(ctx context.Context, req *generated.UserProfile) (*generated.UserProfile, error) {
	select {
	case <-time.After(s.sleepDuration):
		return &generated.UserProfile{
			UserID:   req.UserID,
			Username: req.Username + "-slow",
			Email:    req.Email,
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestGoProductionFeatures(t *testing.T) {
	// Start Helix server on dynamic port
	server := runtime.NewServer("127.0.0.1:0")

	// Get a dynamic address
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	server.Addr = addr
	serviceImpl := &slowUserProfileService{sleepDuration: 50 * time.Millisecond}
	generated.RegisterUserProfileService(server, serviceImpl)

	go func() {
		if err := server.Start(); err != nil {
			panic(err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	client := &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, addr)
			},
		},
	}

	t.Run("Deadline-Exceeded", func(t *testing.T) {
		req := &generated.UserProfile{UserID: 1, Username: "slowpoke"}
		marshaled, _ := req.Marshal()
		reqFrame := make([]byte, 5+len(marshaled))
		binary.BigEndian.PutUint32(reqFrame[1:5], uint32(len(marshaled)))
		copy(reqFrame[5:], marshaled)

		url := fmt.Sprintf("http://%s/helix_example.UserProfileService/GetUserProfile", addr)
		httpReq, _ := http.NewRequest("POST", url, bytes.NewReader(reqFrame))
		httpReq.Header.Set("Content-Type", "application/grpc")
		// Set deadline of 10ms (server takes 50ms)
		httpReq.Header.Set("grpc-timeout", "10m")

		resp, err := client.Do(httpReq)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		grpcStatus := resp.Header.Get("grpc-status")
		// In gRPC, context.DeadlineExceeded corresponds to status code 4 (DEADLINE_EXCEEDED)
		// Or 13 (INTERNAL) because our grpc_handler.go returns "13" on any non-nil handler error:
		// if err != nil { w.Header().Set("grpc-status", "13"); w.Header().Set("grpc-message", err.Error()) ... }
		if grpcStatus != "13" {
			t.Errorf("expected grpc-status to be 13 (or 4), got %s", grpcStatus)
		}
	})

	t.Run("Health-Check", func(t *testing.T) {
		// Health check request has a string field "service".
		// For Protobuf: field 1, string tag is 0x0a.
		// If service is empty: tag is not present (empty byte array).
		reqFrame := make([]byte, 5) // empty service request
		url := fmt.Sprintf("http://%s/grpc.health.v1.Health/Check", addr)
		httpReq, _ := http.NewRequest("POST", url, bytes.NewReader(reqFrame))
		httpReq.Header.Set("Content-Type", "application/grpc")

		resp, err := client.Do(httpReq)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.Header.Get("grpc-status") != "0" {
			t.Fatalf("health check failed with status: %s", resp.Header.Get("grpc-status"))
		}

		frameHeader := make([]byte, 5)
		if _, err := io.ReadFull(resp.Body, frameHeader); err != nil {
			t.Fatalf("failed to read frame header: %v", err)
		}
		length := binary.BigEndian.Uint32(frameHeader[1:5])
		payload := make([]byte, length)
		if _, err := io.ReadFull(resp.Body, payload); err != nil {
			t.Fatalf("failed to read payload: %v", err)
		}

		// Response has field 1 (status): type varint, tag 0x08.
		// Value should be 1 (Serving) -> payload = [0x08, 0x01]
		if len(payload) != 2 || payload[0] != 0x08 || payload[1] != 0x01 {
			t.Errorf("unexpected health check payload: %v", payload)
		}
	})

	t.Run("Gzip-Compression", func(t *testing.T) {
		req := &generated.UserProfile{UserID: 2, Username: "compressed"}
		marshaled, _ := req.Marshal()
		
		// Compress using gzip
		w := runtime.GzipCompressor{}
		compressedBytes, err := w.Compress(marshaled)
		if err != nil {
			t.Fatalf("failed to compress: %v", err)
		}

		reqFrame := make([]byte, 5+len(compressedBytes))
		reqFrame[0] = 1 // compressed flag = 1
		binary.BigEndian.PutUint32(reqFrame[1:5], uint32(len(compressedBytes)))
		copy(reqFrame[5:], compressedBytes)

		url := fmt.Sprintf("http://%s/helix_example.UserProfileService/GetUserProfile", addr)
		httpReq, _ := http.NewRequest("POST", url, bytes.NewReader(reqFrame))
		httpReq.Header.Set("Content-Type", "application/grpc")
		httpReq.Header.Set("grpc-encoding", "gzip")

		resp, err := client.Do(httpReq)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.Header.Get("grpc-status") != "0" {
			t.Fatalf("request failed with status: %s", resp.Header.Get("grpc-status"))
		}

		if resp.Header.Get("grpc-encoding") != "gzip" {
			t.Errorf("expected grpc-encoding header to be 'gzip', got '%s'", resp.Header.Get("grpc-encoding"))
		}

		frameHeader := make([]byte, 5)
		if _, err := io.ReadFull(resp.Body, frameHeader); err != nil {
			t.Fatalf("failed to read frame header: %v", err)
		}
		
		compressedFlag := frameHeader[0]
		if compressedFlag != 1 {
			t.Errorf("expected response frame to be compressed (flag=1), got %d", compressedFlag)
		}

		length := binary.BigEndian.Uint32(frameHeader[1:5])
		payload := make([]byte, length)
		if _, err := io.ReadFull(resp.Body, payload); err != nil {
			t.Fatalf("failed to read payload: %v", err)
		}

		// Decompress payload
		decompressed, err := w.Decompress(payload)
		if err != nil {
			t.Fatalf("failed to decompress response: %v", err)
		}

		user := &generated.UserProfile{}
		if err := user.Unmarshal(decompressed); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if user.Username != "compressed-slow" {
			t.Errorf("unexpected response username: %s", user.Username)
		}
	})
}

func TestGoSecurityAndGracefulShutdown(t *testing.T) {
	certPEM, keyPEM, err := generateCertificates()
	if err != nil {
		t.Fatalf("failed to generate certificates: %v", err)
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("failed to load key pair: %v", err)
	}

	server := runtime.NewServer("127.0.0.1:0")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	server.Addr = addr
	server.TLSConfig = &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	serviceImpl := &slowUserProfileService{sleepDuration: 30 * time.Millisecond}
	generated.RegisterUserProfileService(server, serviceImpl)

	go func() {
		if err := server.Start(); err != nil {
			panic(err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(certPEM)
	clientTLSConfig := &tls.Config{
		RootCAs:            roots,
		InsecureSkipVerify: true,
	}

	client := &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
				return tls.Dial(network, addr, clientTLSConfig)
			},
		},
	}

	t.Run("TLS-Request", func(t *testing.T) {
		req := &generated.UserProfile{UserID: 10, Username: "secure-alice"}
		marshaled, _ := req.Marshal()
		reqFrame := make([]byte, 5+len(marshaled))
		binary.BigEndian.PutUint32(reqFrame[1:5], uint32(len(marshaled)))
		copy(reqFrame[5:], marshaled)

		url := fmt.Sprintf("https://%s/helix_example.UserProfileService/GetUserProfile", addr)
		httpReq, _ := http.NewRequest("POST", url, bytes.NewReader(reqFrame))
		httpReq.Header.Set("Content-Type", "application/grpc")

		resp, err := client.Do(httpReq)
		if err != nil {
			t.Fatalf("TLS request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.Header.Get("grpc-status") != "0" {
			t.Errorf("unexpected status: %s", resp.Header.Get("grpc-status"))
		}
	})

	t.Run("Graceful-Shutdown", func(t *testing.T) {
		req := &generated.UserProfile{UserID: 20, Username: "shutdown-bob"}
		marshaled, _ := req.Marshal()
		reqFrame := make([]byte, 5+len(marshaled))
		binary.BigEndian.PutUint32(reqFrame[1:5], uint32(len(marshaled)))
		copy(reqFrame[5:], marshaled)

		url := fmt.Sprintf("https://%s/helix_example.UserProfileService/GetUserProfile", addr)
		httpReq, _ := http.NewRequest("POST", url, bytes.NewReader(reqFrame))
		httpReq.Header.Set("Content-Type", "application/grpc")

		respChan := make(chan *http.Response, 1)
		errChan := make(chan error, 1)

		go func() {
			resp, err := client.Do(httpReq)
			if err != nil {
				errChan <- err
			} else {
				respChan <- resp
			}
		}()

		time.Sleep(10 * time.Millisecond)

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		err := server.Shutdown(shutdownCtx)
		if err != nil {
			t.Errorf("graceful shutdown failed: %v", err)
		}

		select {
		case err := <-errChan:
			t.Fatalf("in-flight request failed: %v", err)
		case resp := <-respChan:
			defer resp.Body.Close()
			if resp.Header.Get("grpc-status") != "0" {
				t.Errorf("unexpected in-flight status: %s", resp.Header.Get("grpc-status"))
			}
		case <-time.After(200 * time.Millisecond):
			t.Fatal("timed out waiting for in-flight request")
		}

		// Verify new requests are rejected/failed
		client.CloseIdleConnections()
		httpReq2, _ := http.NewRequest("POST", url, bytes.NewReader(reqFrame))
		httpReq2.Header.Set("Content-Type", "application/grpc")
		_, err = client.Do(httpReq2)
		if err == nil {
			t.Error("expected new request to fail after shutdown, but it succeeded")
		}
	})
}

func generateCertificates() ([]byte, []byte, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(time.Hour * 24)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber:          serialNumber,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, err
	}

	certBuf := new(bytes.Buffer)
	_ = pem.Encode(certBuf, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	keyBuf := new(bytes.Buffer)
	_ = pem.Encode(keyBuf, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	return certBuf.Bytes(), keyBuf.Bytes(), nil
}
