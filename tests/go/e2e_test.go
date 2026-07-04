package go_go

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/apache/thrift/lib/go/thrift"
	"golang.org/x/net/http2"

	generated "github.com/helix-rpc/helix/tests/go/generated"
	"github.com/helix-rpc/helix/runtime-go"
)

type myUserProfileService struct{}

func (s *myUserProfileService) GetUserProfile(ctx context.Context, req *generated.UserProfile) (*generated.UserProfile, error) {
	username := req.Username + "-response"
	if md, ok := runtime.FromContext(ctx); ok {
		if traceID := md.Get("x-trace-id"); len(traceID) > 0 {
			username = fmt.Sprintf("%s-%s", username, traceID[0])
		}
	}
	return &generated.UserProfile{
		UserID:   req.UserID,
		Username: username,
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
	generated.RegisterUserProfileService(server, serviceImpl)
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
		resp, err := callGRPCWithHeader(addr, req, "x-trace-id", "grpc-trace-123")
		if err != nil {
			t.Fatalf("gRPC call failed: %v", err)
		}
		if resp.UserID != 999 || resp.Username != "charlie-response-grpc-trace-123" || resp.Email != "charlie@example.com-verified" {
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
		httpReq.Header.Set("x-trace-id", "json-trace-456")

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

		expectedJSON := `{"user_id":555,"username":"david-response-json-trace-456","email":"david@example.com-verified"}`
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

func TestE2EInterceptor(t *testing.T) {
	server := runtime.NewServer("127.0.0.1:0")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	server.Addr = addr
	serviceImpl := &myUserProfileService{}
	generated.RegisterUserProfileService(server, serviceImpl)

	var interceptorCalled bool
	server.AddInterceptor(func(ctx context.Context, req interface{}, info *runtime.UnaryServerInfo, handler runtime.UnaryHandler) (interface{}, error) {
		interceptorCalled = true

		profile, ok := req.(*generated.UserProfile)
		if !ok {
			return nil, fmt.Errorf("unexpected request type in interceptor: %T", req)
		}

		profile.Username = profile.Username + "-intercepted"

		resp, err := handler(ctx, profile)
		return resp, err
	})

	go func() {
		if err := server.Start(); err != nil {
			panic(err)
		}
	}()

	time.Sleep(200 * time.Millisecond)

	req := &generated.UserProfile{
		UserID:   777,
		Username: "interceptor-user",
		Email:    "interceptor@example.com",
	}

	resp, err := callGRPCWithHeader(addr, req, "x-trace-id", "trace-tag-abc")
	if err != nil {
		t.Fatalf("gRPC call with interceptor failed: %v", err)
	}

	if !interceptorCalled {
		t.Error("expected server interceptor to be called, but it was not")
	}

	expectedUsername := "interceptor-user-intercepted-response-trace-tag-abc"
	if resp.Username != expectedUsername {
		t.Errorf("expected username %s, got %s", expectedUsername, resp.Username)
	}
}

func callGRPCWithHeader(addr string, req *generated.UserProfile, hk, hv string) (*generated.UserProfile, error) {
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
	if hk != "" {
		httpReq.Header.Set(hk, hv)
	}

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

type balancerUserProfileService struct {
	addr string
}

func (s *balancerUserProfileService) GetUserProfile(ctx context.Context, req *generated.UserProfile) (*generated.UserProfile, error) {
	return &generated.UserProfile{
		UserID:   req.UserID,
		Username: req.Username + "-response",
		Email:    s.addr,
	}, nil
}

func TestClientPoolingAndBalancing(t *testing.T) {
	servers := make([]*runtime.Server, 3)
	addrs := make([]string, 3)
	for i := 0; i < 3; i++ {
		servers[i] = runtime.NewServer("127.0.0.1:0")
		ln, _ := net.Listen("tcp", "127.0.0.1:0")
		addrs[i] = ln.Addr().String()
		ln.Close()
		servers[i].Addr = addrs[i]

		impl := &balancerUserProfileService{addr: addrs[i]}
		generated.RegisterUserProfileService(servers[i], impl)
		servers[i].RegisterThriftProcessor(generated.NewUserProfileServiceProcessor(impl))

		go func(s *runtime.Server) {
			_ = s.Start()
		}(servers[i])
	}

	time.Sleep(300 * time.Millisecond)

	pools := make(map[string]*runtime.ClientConnPool)
	for _, addr := range addrs {
		pools[addr] = runtime.NewClientConnPool(addr, 5)
	}

	balancer := runtime.NewRoundRobinBalancer()

	expectedOrder := []string{addrs[0], addrs[1], addrs[2], addrs[0], addrs[1], addrs[2]}
	for _, expectedAddr := range expectedOrder {
		target, err := balancer.Next(addrs)
		if err != nil {
			t.Fatalf("balancer next failed: %v", err)
		}
		if target != expectedAddr {
			t.Errorf("expected target %s, got %s", expectedAddr, target)
		}

		pool := pools[target]
		conn, err := pool.Get()
		if err != nil {
			t.Fatalf("failed to get connection from pool: %v", err)
		}

		socket := thrift.NewTSocketFromConnConf(conn, nil)
		trans := thrift.NewTFramedTransportConf(socket, nil)
		_ = trans.Open()
		iprot := thrift.NewTCompactProtocolConf(trans, nil)
		oprot := thrift.NewTCompactProtocolConf(trans, nil)

		req := &generated.UserProfile{
			UserID:   123,
			Username: "balancer-tester",
			Email:    "test@test.com",
		}
		_ = oprot.WriteMessageBegin(context.Background(), "GetUserProfile", thrift.CALL, 1)
		_ = req.Write(context.Background(), oprot)
		_ = oprot.WriteMessageEnd(context.Background())
		_ = oprot.Flush(context.Background())

		_, _, _, _ = iprot.ReadMessageBegin(context.Background())
		resp := &generated.UserProfile{}
		_ = resp.Read(context.Background(), iprot)
		_ = iprot.ReadMessageEnd(context.Background())

		if resp.Email != expectedAddr {
			t.Errorf("expected request to be handled by %s, but handled by %s", expectedAddr, resp.Email)
		}

		pool.Put(conn)
	}
}

func TestRESTTranscoding(t *testing.T) {
	server := runtime.NewServer("127.0.0.1:0")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	server.Addr = addr
	serviceImpl := &myUserProfileService{}
	generated.RegisterUserProfileService(server, serviceImpl)

	server.RegisterRESTRoute("GET", "/v1/users/{user_id}", "/helix_example.UserProfileService/GetUserProfile")

	go func() {
		if err := server.Start(); err != nil {
			panic(err)
		}
	}()

	time.Sleep(200 * time.Millisecond)

	resp, err := http.Get(fmt.Sprintf("http://%s/v1/users/998877", addr))
	if err != nil {
		t.Fatalf("failed to execute GET request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	var profile generated.UserProfile
	if err := json.Unmarshal(body, &profile); err != nil {
		t.Fatalf("failed to unmarshal JSON response: %v", err)
	}

	if profile.UserID != 998877 {
		t.Errorf("expected UserID 998877, got %d", profile.UserID)
	}
	if profile.Username != "-response" {
		t.Errorf("expected Username '-response', got %s", profile.Username)
	}
}

func TestServiceDiscovery(t *testing.T) {
	resolver := runtime.NewStaticResolver()
	addresses := []string{"127.0.0.1:9091", "127.0.0.1:9092"}
	resolver.Register("user-service", addresses)

	resolved, err := resolver.Resolve("user-service")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	if len(resolved) != 2 || resolved[0] != "127.0.0.1:9091" || resolved[1] != "127.0.0.1:9092" {
		t.Errorf("resolved targets mismatch: got %v, expected %v", resolved, addresses)
	}
}

func TestSharedMemoryIPC(t *testing.T) {
	shmPath := "./helix_shm_test.dat"
	shmSize := 4096

	// Create dynamic TCP signaling listener
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()
	addr := ln.Addr().String()

	var clientConn *runtime.SHMConn
	var serverConn net.Conn

	var wg sync.WaitGroup
	wg.Add(1)

	// Start background reader server
	go func() {
		defer wg.Done()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		// Open memory mapping on accepted signaling connection
		shmServer, err := runtime.NewSHMConn(conn, shmPath, shmSize, true)
		if err != nil {
			conn.Close()
			return
		}
		serverConn = shmServer

		// Read payload directly from memory mapping
		buf := make([]byte, 1024)
		n, err := shmServer.Read(buf)
		if err != nil {
			return
		}

		// Echo payload back via memory mapping
		_, _ = shmServer.Write(buf[:n])
	}()

	// Dial background reader server
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}

	// Open memory mapping on dialed signaling connection (isOwner = false)
	time.Sleep(100 * time.Millisecond) // Let file create first
	shmClient, err := runtime.NewSHMConn(conn, shmPath, shmSize, false)
	if err != nil {
		conn.Close()
		t.Fatalf("new shm client failed: %v", err)
	}
	clientConn = shmClient

	// Write payload into memory mapping
	payload := []byte("hello zero-copy shared memory!")
	_, err = shmClient.Write(payload)
	if err != nil {
		t.Fatalf("client write failed: %v", err)
	}

	// Read echoed response payload
	buf := make([]byte, 1024)
	n, err := shmClient.Read(buf)
	if err != nil {
		t.Fatalf("client read failed: %v", err)
	}

	if string(buf[:n]) != string(payload) {
		t.Errorf("echoed payload mismatch: got %s, expected %s", string(buf[:n]), string(payload))
	}

	// Clean up connections
	_ = clientConn.Close()
	wg.Wait()
	if serverConn != nil {
		_ = serverConn.Close()
	}
	_ = os.Remove(shmPath)
}

func TestBidirectionalStreaming(t *testing.T) {
	server := runtime.NewServer("127.0.0.1:0")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	server.Addr = addr

	server.RegisterMethod("/helix_example.UserProfileService/StreamUserProfiles", runtime.MethodInfo{
		IsStreaming: true,
		StreamHandler: func(stream runtime.ServerStream) error {
			for {
				var req generated.UserProfile
				err := stream.Recv(&req)
				if err == io.EOF {
					return nil
				}
				if err != nil {
					return err
				}
				resp := &generated.UserProfile{
					UserID:   req.UserID,
					Username: req.Username + "-echoed",
				}
				if err := stream.Send(resp); err != nil {
					return err
				}
			}
		},
	})

	go func() {
		_ = server.Start()
	}()

	time.Sleep(200 * time.Millisecond)

	client := &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, addr)
			},
		},
	}

	pr, pw := io.Pipe()

	url := fmt.Sprintf("http://%s/helix_example.UserProfileService/StreamUserProfiles", addr)
	httpReq, err := http.NewRequest("POST", url, pr)
	if err != nil {
		t.Fatalf("failed to create http request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/grpc")

	respChan := make(chan *http.Response, 1)
	errChan := make(chan error, 1)
	go func() {
		res, err := client.Do(httpReq)
		if err != nil {
			errChan <- err
			return
		}
		respChan <- res
	}()

	for i := 1; i <= 3; i++ {
		req := &generated.UserProfile{
			UserID:   int64(i),
			Username: fmt.Sprintf("stream-user-%d", i),
		}
		payload, _ := req.Marshal()
		frame := make([]byte, 5+len(payload))
		frame[0] = 0
		binary.BigEndian.PutUint32(frame[1:5], uint32(len(payload)))
		copy(frame[5:], payload)

		_, _ = pw.Write(frame)
		time.Sleep(50 * time.Millisecond)
	}
	_ = pw.Close()

	select {
	case err := <-errChan:
		t.Fatalf("request failed: %v", err)
	case res := <-respChan:
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("unexpected status code: %d", res.StatusCode)
		}

		for i := 1; i <= 3; i++ {
			header := make([]byte, 5)
			_, err := io.ReadFull(res.Body, header)
			if err != nil {
				t.Fatalf("failed to read response header at %d: %v", i, err)
			}
			length := binary.BigEndian.Uint32(header[1:5])
			payload := make([]byte, length)
			_, err = io.ReadFull(res.Body, payload)
			if err != nil {
				t.Fatalf("failed to read response payload at %d: %v", i, err)
			}

			var resp generated.UserProfile
			_ = resp.Unmarshal(payload)

			if resp.UserID != int64(i) {
				t.Errorf("expected UserID %d, got %d", i, resp.UserID)
			}
			expectedUsername := fmt.Sprintf("stream-user-%d-echoed", i)
			if resp.Username != expectedUsername {
				t.Errorf("expected username %s, got %s", expectedUsername, resp.Username)
			}
		}
	}
}
