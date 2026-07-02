package go_go

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	transcoding "github.com/helix-rpc/helix/integration-tests/go-go/generated/transcoding"
	"github.com/helix-rpc/helix/runtime-go"
)

type TranscodingServiceImpl struct{}

func (s *TranscodingServiceImpl) GetProfile(ctx context.Context, req *transcoding.UserProfile) (*transcoding.UserProfile, error) {
	return &transcoding.UserProfile{
		UserID:   req.UserID,
		Username: req.Username + "-processed",
		Email:    req.Email + "-verified",
	}, nil
}

func TestAdvancedRESTTranscoding(t *testing.T) {
	// Start server on dynamic port
	srv := runtime.NewServer("127.0.0.1:0")
	transcoding.RegisterTranscodingService(srv, &TranscodingServiceImpl{})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	srv.Addr = addr
	go func() {
		if err := srv.Start(); err != nil {
			// ignore server shutdown error
		}
	}()
	defer srv.Shutdown(context.Background())

	time.Sleep(50 * time.Millisecond)

	// Make HTTP GET query with path param (user_id=888) and query params (username, email)
	url := fmt.Sprintf("http://%s/v1/profile/888?username=rest_user&email=rest@test.com", addr)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("failed to make HTTP GET request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	var res transcoding.UserProfile
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("failed to unmarshal JSON response: %v, body: %s", err, string(body))
	}

	if res.UserID != 888 {
		t.Errorf("UserID mismatch: got %d, want 888", res.UserID)
	}
	if res.Username != "rest_user-processed" {
		t.Errorf("Username mismatch: got %q, want %q", res.Username, "rest_user-processed")
	}
	if res.Email != "rest@test.com-verified" {
		t.Errorf("Email mismatch: got %q, want %q", res.Email, "rest@test.com-verified")
	}
}
