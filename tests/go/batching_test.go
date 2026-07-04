package go_go

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/helix-rpc/helix/runtime-go"
)

type UserProfile struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

func TestDynamicBatching(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	server := runtime.NewServer(addr)

	var batchCallCount int32
	var totalProcessed int32

	handler := func(ctx context.Context, reqs []interface{}) ([]interface{}, error) {
		atomic.AddInt32(&batchCallCount, 1)
		atomic.AddInt32(&totalProcessed, int32(len(reqs)))

		time.Sleep(10 * time.Millisecond)

		resps := make([]interface{}, len(reqs))
		for i, r := range reqs {
			req := r.(*UserProfile)
			resps[i] = &UserProfile{
				Id:   req.Id,
				Name: "Batched User " + req.Id,
			}
		}
		return resps, nil
	}

	methodInfo := runtime.MethodInfo{
		Decoder: func(dec func(interface{}) error) (interface{}, error) {
			var req UserProfile
			if err := dec(&req); err != nil {
				return nil, err
			}
			return &req, nil
		},
	}

	server.RegisterBatchMethod("/test.UserProfileService/GetUserProfile", 10, 50*time.Millisecond, methodInfo, handler)
	server.RegisterRESTRoute("POST", "/v1/users", "/test.UserProfileService/GetUserProfile")

	go func() {
		if err := server.Start(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()
	time.Sleep(100 * time.Millisecond)

	var wg sync.WaitGroup
	concurrency := 50
	wg.Add(concurrency)

	client := &http.Client{Timeout: 5 * time.Second}

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			defer wg.Done()
			reqBody := fmt.Sprintf(`{"id":"%d"}`, id)
			resp, err := client.Post(fmt.Sprintf("http://%s/v1/users", addr), "application/json", bytes.NewBufferString(reqBody))
			if err != nil {
				t.Errorf("request %d failed: %v", id, err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("request %d got status: %d", id, resp.StatusCode)
				return
			}

			body, _ := io.ReadAll(resp.Body)
			var profile UserProfile
			json.Unmarshal(body, &profile)

			expectedName := "Batched User " + fmt.Sprintf("%d", id)
			if profile.Name != expectedName {
				t.Errorf("request %d got wrong response: %v", id, profile.Name)
			}
		}(i)
	}

	wg.Wait()
	server.Shutdown(context.Background())

	calls := atomic.LoadInt32(&batchCallCount)
	processed := atomic.LoadInt32(&totalProcessed)

	t.Logf("Total processed: %d, Batch calls: %d", processed, calls)

	if processed != 50 {
		t.Errorf("expected 50 total processed, got %d", processed)
	}
	if calls >= 50 {
		t.Errorf("expected batching to occur, but handler was called %d times (no batching)", calls)
	}
}
