package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	runtime "github.com/helixrpc/helix-rt"
)

// PredictRequest simulates an AI inference prompt
type PredictRequest struct {
	Prompt string `json:"prompt"`
}

// PredictResponse simulates an AI inference response
type PredictResponse struct {
	Completion string `json:"completion"`
}

// MockBatchAIModel implements the batch processing logic natively
type MockBatchAIModel struct{}

func (m *MockBatchAIModel) PredictBatch(ctx context.Context, reqs []interface{}) ([]interface{}, error) {
	fmt.Printf("[MockAI] 🚀 Executing batch of %d prompts simultaneously on virtual GPU...\n", len(reqs))
	
	// Simulate heavy AI computation
	time.Sleep(100 * time.Millisecond)

	var resps []interface{}
	for i, r := range reqs {
		var prompt string
		if pr, ok := r.(*PredictRequest); ok {
			prompt = pr.Prompt
		} else if mapReq, ok := r.(map[string]interface{}); ok {
			prompt = mapReq["prompt"].(string)
		} else {
			prompt = fmt.Sprintf("%v", r)
		}
		
		fmt.Printf("  -> [Batch Index %d] Processing prompt: %q\n", i, prompt)
		resps = append(resps, &PredictResponse{
			Completion: fmt.Sprintf("AI Response to: %s", prompt),
		})
	}
	
	fmt.Printf("[MockAI] ✅ Batch execution complete!\n")
	return resps, nil
}

func main() {
	fmt.Println("Initializing Helix RPC Go Dynamic Batching Server...")

	// 1. Create the Batch Dispatcher
	dispatcher := runtime.NewBatchDispatcher(50*time.Millisecond, 100, &MockBatchAIModel{})
	dispatcher.Start()
	defer dispatcher.Stop()

	// 2. Initialize the Server
	server := runtime.NewServer()

	// 3. Register the Batching Interceptor (Handler)
	server.RegisterUnary("/v1/models/predict", func(ctx context.Context, payload []byte) (interface{}, error) {
		var req PredictRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		
		// Dispatch to the batcher. This blocks until the batch window closes and the batch executes!
		resp, err := dispatcher.Dispatch(ctx, &req)
		if err != nil {
			return nil, err
		}
		
		return resp, nil
	}, runtime.RestOptions{
		Method: "POST",
		Path:   "/v1/models/predict",
	})

	// 4. Start the HTTP server
	srv := &http.Server{
		Addr:    ":8080",
		Handler: server,
	}

	go func() {
		fmt.Println("🚀 Starting API Gateway on http://127.0.0.1:8080...")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Listen error: %v\n", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\nShutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server Shutdown Failed: %v", err)
	}
	fmt.Println("Server gracefully stopped")
}
