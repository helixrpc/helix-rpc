package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	chat "github.com/helix-rpc/helix/tests/python/ai/schema"
	"github.com/helix-rpc/helix/runtime-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// 1. Connect to Python AI Model Server via standard gRPC
	conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to connect to python grpc server: %v", err)
	}
	defer conn.Close()
	grpcClient := chat.NewChatCompletionServiceClient(conn)

	// 2. Start Helix RPC API Gateway on port 8080
	server := runtime.NewServer("0.0.0.0:8080")

	server.RegisterRESTRoute("POST", "/v1/chat/completions", "/openai.chat.ChatCompletionService/StreamChatCompletion")

	server.RegisterMethod("/openai.chat.ChatCompletionService/StreamChatCompletion", runtime.MethodInfo{
		Decoder: func(dec func(interface{}) error) (interface{}, error) {
			var req chat.ChatCompletionRequest
			if err := dec(&req); err != nil {
				return nil, err
			}
			return &req, nil
		},
		IsStreaming: true,
		StreamHandler: func(stream runtime.ServerStream) error {
			var req chat.ChatCompletionRequest
			if err := stream.Recv(&req); err != nil {
				return err
			}

			ctx, cancel := context.WithTimeout(stream.Context(), 10*time.Second)
			defer cancel()

			pythonStream, err := grpcClient.StreamChatCompletion(ctx, &req)
			if err != nil {
				return err
			}

			for {
				resp, err := pythonStream.Recv()
				if err == io.EOF {
					break
				}
				if err != nil {
					return err
				}
				if err := stream.Send(resp); err != nil {
					return err
				}
			}
			return nil
		},
	})

	go func() {
		fmt.Println("Helix RPC AI Gateway listening on port 8080...")
		fmt.Println("You can run: curl -X POST http://localhost:8080/v1/chat/completions -H 'Content-Type: application/json' -d '{\"model\":\"gpt-4\", \"messages\":[{\"role\":\"user\", \"content\":\"hello!\"}]}'")
		if err := server.Start(); err != nil {
			log.Fatalf("server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	fmt.Println("Shutting down gateway...")
	server.Shutdown(context.Background())
}
