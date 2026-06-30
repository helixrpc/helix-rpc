package go_go

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/apache/thrift/lib/go/thrift"
	"golang.org/x/net/http2"

	generated "github.com/helix-rpc/helix/integration-tests/go-go/generated"
	"github.com/helix-rpc/helix/runtime-go"
)

type myUserProfileService struct{}

func (s *myUserProfileService) GetUserProfile(ctx context.Context, req *generated.UserProfile) (*generated.UserProfile, error) {
	return &generated.UserProfile{
		UserID:   req.UserID,
		Username: req.Username + "-response",
		Email:    req.Email + "-verified",
	}, nil
}

func TestE2EMultiProtocol(t *testing.T) {
	// Start Helix server on dynamic port
	server := runtime.NewServer("127.0.0.1:0")

	// Create listener first to get dynamic address
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close() // Close it so port becomes available, but we bind immediately in Start

	server.Addr = addr
	serviceImpl := &myUserProfileService{}
	
	// Register both gRPC and Thrift handler
	server.RegisterService("helix_example.UserProfileService", serviceImpl)
	server.RegisterThriftProcessor(generated.NewUserProfileServiceProcessor(serviceImpl))

	// Run server in background
	go func() {
		if err := server.Start(); err != nil {
			panic(err)
		}
	}()

	// Wait for server to start
	time.Sleep(200 * time.Millisecond)

	t.Run("Thrift-Compact-Protocol", func(t *testing.T) {
		req := &generated.UserProfile{
			UserID:   12345,
			Username: "alice",
			Email:    "alice@example.com",
		}
		resp, err := callThrift(addr, true, req)
		if err != nil {
			t.Fatalf("thrift compact call failed: %v", err)
		}
		if resp.UserID != 12345 || resp.Username != "alice-response" || resp.Email != "alice@example.com-verified" {
			t.Errorf("unexpected thrift response: %+v", resp)
		}
	})

	t.Run("Thrift-Binary-Protocol", func(t *testing.T) {
		req := &generated.UserProfile{
			UserID:   67890,
			Username: "bob",
			Email:    "bob@example.com",
		}
		resp, err := callThrift(addr, false, req)
		if err != nil {
			t.Fatalf("thrift binary call failed: %v", err)
		}
		if resp.UserID != 67890 || resp.Username != "bob-response" || resp.Email != "bob@example.com-verified" {
			t.Errorf("unexpected thrift response: %+v", resp)
		}
	})

	t.Run("gRPC-Protocol", func(t *testing.T) {
		req := &generated.UserProfile{
			UserID:   999,
			Username: "charlie",
			Email:    "charlie@example.com",
		}
		resp, err := callGRPC(addr, req)
		if err != nil {
			t.Fatalf("gRPC call failed: %v", err)
		}
		if resp.UserID != 999 || resp.Username != "charlie-response" || resp.Email != "charlie@example.com-verified" {
			t.Errorf("unexpected gRPC response: %+v", resp)
		}
	})

	t.Run("HTTP-JSON-Protocol", func(t *testing.T) {
		reqJSON := []byte(`{"user_id": 555, "username": "david", "email": "david@example.com"}`)
		url := fmt.Sprintf("http://%s/helix_example.UserProfileService/GetUserProfile", addr)
		
		httpReq, err := http.NewRequest("POST", url, bytes.NewReader(reqJSON))
		if err != nil {
			t.Fatalf("failed to create http request: %v", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			t.Fatalf("HTTP JSON request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("unexpected status: %d", resp.StatusCode)
		}

		respBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed to read response body: %v", err)
		}

		expectedJSON := `{"user_id":555,"username":"david-response","email":"david@example.com-verified"}`
		if string(respBytes) != expectedJSON {
			t.Errorf("unexpected response JSON: got %s, expected %s", string(respBytes), expectedJSON)
		}
	})
}

func callThrift(addr string, isCompact bool, req *generated.UserProfile) (*generated.UserProfile, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	tSocket := thrift.NewTSocketFromConnConf(conn, nil)
	transport := thrift.NewTFramedTransport(tSocket)
	if !transport.IsOpen() {
		if err := transport.Open(); err != nil {
			return nil, err
		}
	}
	defer transport.Close()

	var protoFactory thrift.TProtocolFactory
	if isCompact {
		protoFactory = thrift.NewTCompactProtocolFactoryConf(nil)
	} else {
		protoFactory = thrift.NewTBinaryProtocolFactoryConf(nil)
	}

	iprot := protoFactory.GetProtocol(transport)
	oprot := protoFactory.GetProtocol(transport)

	ctx := context.Background()
	// Write call
	if err := oprot.WriteMessageBegin(ctx, "GetUserProfile", thrift.CALL, 1); err != nil {
		return nil, err
	}
	if err := req.Write(ctx, oprot); err != nil {
		return nil, err
	}
	if err := oprot.WriteMessageEnd(ctx); err != nil {
		return nil, err
	}
	if err := oprot.Flush(ctx); err != nil {
		return nil, err
	}

	// Read reply
	name, mTypeId, _, err := iprot.ReadMessageBegin(ctx)
	if err != nil {
		return nil, err
	}
	if mTypeId == thrift.EXCEPTION {
		x := thrift.NewTApplicationException(thrift.UNKNOWN_APPLICATION_EXCEPTION, "")
		x.Read(ctx, iprot)
		iprot.ReadMessageEnd(ctx)
		return nil, x
	}
	if name != "GetUserProfile" {
		return nil, fmt.Errorf("unexpected method reply: %s", name)
	}

	res := &generated.UserProfile{}
	if err := res.Read(ctx, iprot); err != nil {
		return nil, err
	}
	if err := iprot.ReadMessageEnd(ctx); err != nil {
		return nil, err
	}

	return res, nil
}

func callGRPC(addr string, req *generated.UserProfile) (*generated.UserProfile, error) {
	// Create H2C client (HTTP/2 cleartext)
	client := &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, addr)
			},
		},
	}

	marshaled, err := req.Marshal()
	if err != nil {
		return nil, err
	}

	// Prepare gRPC frame: 1 byte compressed flag, 4 bytes big endian length
	reqFrame := make([]byte, 5+len(marshaled))
	reqFrame[0] = 0 // uncompressed
	binary.BigEndian.PutUint32(reqFrame[1:5], uint32(len(marshaled)))
	copy(reqFrame[5:], marshaled)

	url := fmt.Sprintf("http://%s/helix_example.UserProfileService/GetUserProfile", addr)
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(reqFrame))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/grpc")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status: %d", resp.StatusCode)
	}

	grpcStatus := resp.Header.Get("grpc-status")
	if grpcStatus != "" && grpcStatus != "0" {
		return nil, fmt.Errorf("gRPC error: status=%s, message=%s", grpcStatus, resp.Header.Get("grpc-message"))
	}

	// Read response gRPC frame
	frameHeader := make([]byte, 5)
	if _, err := io.ReadFull(resp.Body, frameHeader); err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint32(frameHeader[1:5])
	payload := make([]byte, length)
	if _, err := io.ReadFull(resp.Body, payload); err != nil {
		return nil, err
	}

	res := &generated.UserProfile{}
	if err := res.Unmarshal(payload); err != nil {
		return nil, err
	}

	return res, nil
}
