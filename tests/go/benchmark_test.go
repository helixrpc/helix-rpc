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
	"sync"
	"testing"
	"time"

	"github.com/apache/thrift/lib/go/thrift"
	generated "github.com/helix-rpc/helix/tests/go/generated"
	"github.com/helix-rpc/helix/runtime-go"
	"golang.org/x/net/http2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	kitexclient "github.com/cloudwego/kitex/client"
	kitexserver "github.com/cloudwego/kitex/server"
	"github.com/helix-rpc/helix/tests/go/kitex_gen/helix/example"
	"github.com/helix-rpc/helix/tests/go/kitex_gen/helix/example/userprofileservice"
)

var (
	initBenchOnce sync.Once

	nativeGrpcAddr   string
	nativeThriftAddr string
	nativeHttpAddr   string
	helixAddr        string
	kitexAddr        string

	grpcConn    *grpc.ClientConn
	kitexClient userprofileservice.Client

	httpClient      *http.Client
	helixGrpcClient *http.Client
)

func initBenchServers() {
	// Initialize persistent HTTP client
	httpClient = &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	// Initialize Helix gRPC HTTP/2 client
	helixGrpcClient = &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, addr)
			},
		},
	}

	// 1. Start Native gRPC Server
	grpcSrv := grpc.NewServer()
	registerNativeGrpc(grpcSrv)
	ln1, _ := net.Listen("tcp", "127.0.0.1:0")
	nativeGrpcAddr = ln1.Addr().String()
	go func() {
		_ = grpcSrv.Serve(ln1)
	}()

	// 2. Start Native HTTP JSON Server
	ln2, _ := net.Listen("tcp", "127.0.0.1:0")
	nativeHttpAddr = ln2.Addr().String()
	httpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req generated.UserProfile
		_ = json.Unmarshal(body, &req)
		resp := generated.UserProfile{
			UserID:   req.UserID,
			Username: req.Username + "-response",
			Email:    req.Email + "-verified",
		}
		respBytes, _ := json.Marshal(&resp)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(respBytes)
	})
	go func() {
		_ = http.Serve(ln2, httpHandler)
	}()

	// 3. Start Native Thrift Compact Server (Framed)
	ln3, _ := net.Listen("tcp", "127.0.0.1:0")
	nativeThriftAddr = ln3.Addr().String()
	go func() {
		for {
			conn, err := ln3.Accept()
			if err != nil {
				break
			}
			go handleNativeThrift(conn)
		}
	}()

	// 4. Start Helix Server (Multiplexed)
	helixSrv := runtime.NewServer("127.0.0.1:0")
	serviceImpl := &myUserProfileService{}
	generated.RegisterUserProfileService(helixSrv, serviceImpl)
	helixSrv.RegisterThriftProcessor(generated.NewUserProfileServiceProcessor(serviceImpl))
	ln4, _ := net.Listen("tcp", "127.0.0.1:0")
	helixAddr = ln4.Addr().String()
	ln4.Close()
	helixSrv.Addr = helixAddr
	go func() {
		_ = helixSrv.Start()
	}()

	// 5. Start Kitex Server (Go Thrift)
	ln5, _ := net.Listen("tcp", "127.0.0.1:0")
	kitexAddr = ln5.Addr().String()
	go func() {
		svr := userprofileservice.NewServer(new(KitexUserProfileServiceImpl), kitexserver.WithListener(ln5))
		_ = svr.Run()
	}()

	// Wait for servers to spin up
	time.Sleep(300 * time.Millisecond)

	// Pre-dial client connections
	grpcConn, _ = grpc.Dial(nativeGrpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	kitexClient, _ = userprofileservice.NewClient("user-service", kitexclient.WithHostPorts(kitexAddr))
}

// Manual gRPC server registration helpers
type generatedUserProfileServiceServer interface {
	GetUserProfile(context.Context, *generated.UserProfile) (*generated.UserProfile, error)
}

func registerNativeGrpc(s *grpc.Server) {
	s.RegisterService(&grpc.ServiceDesc{
		ServiceName: "helix_example.UserProfileService",
		HandlerType: (*generatedUserProfileServiceServer)(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: "GetUserProfile",
				Handler: func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
					in := new(generated.UserProfile)
					if err := dec(in); err != nil {
						return nil, err
					}
					return srv.(generatedUserProfileServiceServer).GetUserProfile(ctx, in)
				},
			},
		},
		Streams:  []grpc.StreamDesc{},
		Metadata: "test.proto",
	}, &grpcServerImpl{})
}

type grpcServerImpl struct{}

func (s *grpcServerImpl) GetUserProfile(ctx context.Context, req *generated.UserProfile) (*generated.UserProfile, error) {
	return &generated.UserProfile{
		UserID:   req.UserID,
		Username: req.Username + "-response",
		Email:    req.Email + "-verified",
	}, nil
}

// Native Thrift framed dispatcher loop
func handleNativeThrift(conn net.Conn) {
	socket := thrift.NewTSocketFromConnConf(conn, nil)
	trans := thrift.NewTFramedTransportConf(socket, nil)
	iprot := thrift.NewTCompactProtocolConf(trans, nil)
	oprot := thrift.NewTCompactProtocolConf(trans, nil)
	for {
		name, _, seqId, err := iprot.ReadMessageBegin(context.Background())
		if err != nil {
			break
		}
		if name != "GetUserProfile" {
			break
		}
		req := &generated.UserProfile{}
		if err := req.Read(context.Background(), iprot); err != nil {
			break
		}
		_ = iprot.ReadMessageEnd(context.Background())

		resp := &generated.UserProfile{
			UserID:   req.UserID,
			Username: req.Username + "-response",
			Email:    req.Email + "-verified",
		}

		_ = oprot.WriteMessageBegin(context.Background(), "GetUserProfile", thrift.REPLY, seqId)
		_ = resp.Write(context.Background(), oprot)
		_ = oprot.WriteMessageEnd(context.Background())
		_ = oprot.Flush(context.Background())
	}
	conn.Close()
}

func callHelixGRPC(client *http.Client, addr string, req *generated.UserProfile) (*generated.UserProfile, error) {
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

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status: %d", resp.StatusCode)
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

// --- Benchmark Test Cases ---

func BenchmarkNativeGRPC(b *testing.B) {
	initBenchOnce.Do(initBenchServers)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := &generated.UserProfile{
			UserID:   123,
			Username: "bench",
			Email:    "bench@grpc.com",
		}
		var resp generated.UserProfile
		err := grpcConn.Invoke(context.Background(), "/helix_example.UserProfileService/GetUserProfile", req, &resp)
		if err != nil {
			b.Fatalf("native grpc call failed: %v", err)
		}
	}
}

func BenchmarkHelixGRPC(b *testing.B) {
	initBenchOnce.Do(initBenchServers)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := &generated.UserProfile{
			UserID:   123,
			Username: "bench",
			Email:    "bench@grpc.com",
		}
		_, err := callHelixGRPC(helixGrpcClient, helixAddr, req)
		if err != nil {
			b.Fatalf("helix grpc call failed: %v", err)
		}
	}
}

func BenchmarkNativeHTTPJSON(b *testing.B) {
	initBenchOnce.Do(initBenchServers)
	url := fmt.Sprintf("http://%s/", nativeHttpAddr)
	reqJSON := []byte(`{"user_id": 123, "username": "bench", "email": "bench@json.com"}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := httpClient.Post(url, "application/json", bytes.NewReader(reqJSON))
		if err != nil {
			b.Fatalf("native http request failed: %v", err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}

func BenchmarkHelixHTTPJSON(b *testing.B) {
	initBenchOnce.Do(initBenchServers)
	url := fmt.Sprintf("http://%s/helix_example.UserProfileService/GetUserProfile", helixAddr)
	reqJSON := []byte(`{"user_id": 123, "username": "bench", "email": "bench@json.com"}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := httpClient.Post(url, "application/json", bytes.NewReader(reqJSON))
		if err != nil {
			b.Fatalf("helix http request failed: %v", err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}

func BenchmarkNativeThriftCompact(b *testing.B) {
	initBenchOnce.Do(initBenchServers)
	conn, err := net.Dial("tcp", nativeThriftAddr)
	if err != nil {
		b.Fatalf("thrift connection failed: %v", err)
	}
	defer conn.Close()

	socket := thrift.NewTSocketFromConnConf(conn, nil)
	trans := thrift.NewTFramedTransportConf(socket, nil)
	iprot := thrift.NewTCompactProtocolConf(trans, nil)
	oprot := thrift.NewTCompactProtocolConf(trans, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := &generated.UserProfile{
			UserID:   123,
			Username: "bench",
			Email:    "bench@thrift.com",
		}
		_ = oprot.WriteMessageBegin(context.Background(), "GetUserProfile", thrift.CALL, int32(i))
		_ = req.Write(context.Background(), oprot)
		_ = oprot.WriteMessageEnd(context.Background())
		_ = oprot.Flush(context.Background())

		_, _, _, _ = iprot.ReadMessageBegin(context.Background())
		resp := &generated.UserProfile{}
		_ = resp.Read(context.Background(), iprot)
		_ = iprot.ReadMessageEnd(context.Background())
	}
}

func BenchmarkHelixThriftCompact(b *testing.B) {
	initBenchOnce.Do(initBenchServers)
	conn, err := net.Dial("tcp", helixAddr)
	if err != nil {
		b.Fatalf("thrift connection failed: %v", err)
	}
	defer conn.Close()

	socket := thrift.NewTSocketFromConnConf(conn, nil)
	trans := thrift.NewTFramedTransportConf(socket, nil)
	iprot := thrift.NewTCompactProtocolConf(trans, nil)
	oprot := thrift.NewTCompactProtocolConf(trans, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := &generated.UserProfile{
			UserID:   123,
			Username: "bench",
			Email:    "bench@thrift.com",
		}
		_ = oprot.WriteMessageBegin(context.Background(), "GetUserProfile", thrift.CALL, int32(i))
		_ = req.Write(context.Background(), oprot)
		_ = oprot.WriteMessageEnd(context.Background())
		_ = oprot.Flush(context.Background())

		_, _, _, _ = iprot.ReadMessageBegin(context.Background())
		resp := &generated.UserProfile{}
		_ = resp.Read(context.Background(), iprot)
		_ = iprot.ReadMessageEnd(context.Background())
	}
}

type KitexUserProfileServiceImpl struct{}

func (s *KitexUserProfileServiceImpl) GetUserProfile(ctx context.Context, req *example.UserProfile) (resp *example.UserProfile, err error) {
	return &example.UserProfile{
		UserId:   req.UserId,
		Username: req.Username + "-response",
		Email:    req.Email + "-verified",
	}, nil
}

func BenchmarkKitexThrift(b *testing.B) {
	initBenchOnce.Do(initBenchServers)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := &example.UserProfile{
			UserId:   123,
			Username: "bench",
			Email:    "bench@kitex.com",
		}
		resp, err := kitexClient.GetUserProfile(context.Background(), req)
		if err != nil {
			b.Fatalf("kitex call failed: %v", err)
		}
		if resp.UserId != 123 {
			b.Fatalf("invalid response userId: %d", resp.UserId)
		}
	}
}
