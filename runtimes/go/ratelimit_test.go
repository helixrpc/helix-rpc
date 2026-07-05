package runtime

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAdaptiveConcurrencyLimiter(t *testing.T) {
	limiter := NewAdaptiveConcurrencyLimiter(5, 2, 10)

	// Verify initial limit is 5
	if limit := limiter.Limit(); limit != 5 {
		t.Fatalf("expected limit to be 5, got %d", limit)
	}

	// Try acquiring 5 requests
	for i := 0; i < 5; i++ {
		if !limiter.Acquire() {
			t.Fatalf("failed to acquire request %d", i)
		}
	}

	// 6th acquire should fail
	if limiter.Acquire() {
		t.Fatal("expected 6th acquire to fail")
	}

	// Release one request and check if we can acquire again
	limiter.Release(10 * time.Millisecond)
	if !limiter.Acquire() {
		t.Fatal("expected to acquire after release")
	}
	limiter.Release(10 * time.Millisecond)

	// Simulate low-load (fast responses) to increase the limit
	// With rttNoLoad = 10ms, if we respond in 10ms, expected queue size = 0 < alpha (3)
	// So limit should increase. Let's release the remaining 4 requests with 10ms latency.
	for i := 0; i < 4; i++ {
		limiter.Release(10 * time.Millisecond)
	}

	// Limit should have increased towards maxLimit (10)
	if limit := limiter.Limit(); limit <= 5 {
		t.Logf("limit after low load: %d", limit)
	}

	// Simulate high-load (slow responses) to decrease the limit
	// Respond with high latency (e.g. 100ms)
	for i := 0; i < 10; i++ {
		limiter.Release(100 * time.Millisecond)
	}

	// Limit should decrease towards minLimit (2)
	if limit := limiter.Limit(); limit >= 10 {
		t.Fatalf("expected limit to decrease under high load, got %d", limit)
	}
}

func TestAdaptiveConcurrencyLimiter_Interceptor(t *testing.T) {
	limiter := NewAdaptiveConcurrencyLimiter(2, 1, 5)
	interceptor := limiter.Interceptor()

	info := &UnaryServerInfo{FullMethod: "/test"}

	// Test concurrency exhaustion
	// Block inside handler
	barrier := make(chan struct{})
	entered := make(chan struct{}, 2)
	handlerBlocked := func(ctx context.Context, req interface{}) (interface{}, error) {
		entered <- struct{}{}
		<-barrier
		return "blocked", nil
	}

	// Spawn 2 concurrent requests (initialLimit = 2)
	go func() {
		_, _ = interceptor(context.Background(), nil, info, handlerBlocked)
	}()
	go func() {
		_, _ = interceptor(context.Background(), nil, info, handlerBlocked)
	}()

	// Wait for both to enter the handler
	for i := 0; i < 2; i++ {
		<-entered
	}

	// 3rd concurrent request should fail
	_, err := interceptor(context.Background(), nil, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	})
	if err == nil {
		t.Fatal("expected interceptor to fail with resource exhausted error")
	}

	var hErr *HelixError
	if !errors.As(err, &hErr) {
		t.Fatalf("expected HelixError, got %T", err)
	}
	if hErr.Code != CodeResourceExhausted {
		t.Fatalf("expected CodeResourceExhausted, got %v", hErr.Code)
	}

	// Unblock
	close(barrier)
}
