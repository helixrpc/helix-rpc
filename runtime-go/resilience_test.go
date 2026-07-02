package runtime

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ===========================================================================
// Circuit Breaker Tests
// ===========================================================================

func TestCircuitBreaker_InitialState(t *testing.T) {
	cb := NewCircuitBreaker(3, 10*time.Second, 2)
	if cb.State() != CircuitClosed {
		t.Fatalf("expected Closed, got %v", cb.State())
	}
	if !cb.Allow() {
		t.Fatal("closed circuit should allow requests")
	}
}

func TestCircuitBreaker_OpensAfterMaxFailures(t *testing.T) {
	cb := NewCircuitBreaker(3, 10*time.Second, 2)
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != CircuitClosed {
		t.Fatal("should still be closed after 2 failures (max=3)")
	}
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Fatal("should be open after 3 failures")
	}
	if cb.Allow() {
		t.Fatal("open circuit must not allow requests")
	}
}

func TestCircuitBreaker_SuccessResetsFailures(t *testing.T) {
	cb := NewCircuitBreaker(3, 10*time.Second, 2)
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordSuccess() // reset — failure counter goes to 0
	cb.RecordFailure()
	if cb.State() != CircuitClosed {
		t.Fatal("one failure after a success should not trip the circuit")
	}
}

func TestCircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	cb := NewCircuitBreaker(1, 50*time.Millisecond, 1)
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Fatal("expected Open")
	}
	time.Sleep(60 * time.Millisecond) // wait for open timeout
	if !cb.Allow() {
		t.Fatal("circuit should allow one probe after timeout")
	}
	if cb.State() != CircuitHalfOpen {
		t.Fatal("expected HalfOpen after timeout + Allow()")
	}
}

func TestCircuitBreaker_ClosesAfterHalfOpenSuccess(t *testing.T) {
	cb := NewCircuitBreaker(1, 10*time.Millisecond, 2)
	cb.RecordFailure()
	time.Sleep(15 * time.Millisecond)
	cb.Allow() // transitions to HalfOpen
	cb.RecordSuccess()
	cb.RecordSuccess() // 2 probes → Closed
	if cb.State() != CircuitClosed {
		t.Fatalf("expected Closed after probes, got %v", cb.State())
	}
}

func TestCircuitBreaker_ConcurrentSafety(t *testing.T) {
	cb := NewCircuitBreaker(100, 10*time.Second, 2)
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cb.RecordFailure()
		}()
	}
	wg.Wait()
	if cb.State() != CircuitOpen {
		t.Fatal("circuit should be open after 200 concurrent failures")
	}
}

// ===========================================================================
// Token Bucket Tests
// ===========================================================================

func TestTokenBucket_BasicConsume(t *testing.T) {
	tb := NewHedgingTokenBucket(3, 0) // 3 burst, no refill
	if !tb.Consume() || !tb.Consume() || !tb.Consume() {
		t.Fatal("first 3 consumes should succeed")
	}
	if tb.Consume() {
		t.Fatal("4th consume should fail — bucket empty")
	}
}

func TestTokenBucket_Refills(t *testing.T) {
	tb := NewHedgingTokenBucket(1, 100) // 100 tokens/sec
	tb.Consume()                         // drain it
	time.Sleep(20 * time.Millisecond)   // wait for ~2 tokens to refill
	if !tb.Consume() {
		t.Fatal("bucket should have refilled at least one token")
	}
}

func TestTokenBucket_ConcurrentSafe(t *testing.T) {
	tb := NewHedgingTokenBucket(50, 0)
	var consumed int64
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if tb.Consume() {
				atomic.AddInt64(&consumed, 1)
			}
		}()
	}
	wg.Wait()
	if consumed != 50 {
		t.Fatalf("expected exactly 50 successful consumes, got %d", consumed)
	}
}

// ===========================================================================
// RetryPolicy / ExecuteWithRetry Tests
// ===========================================================================

func TestExecuteWithRetry_SucceedsFirstAttempt(t *testing.T) {
	policy := DefaultRetryPolicy()
	calls := 0
	result, err := ExecuteWithRetry[string](context.Background(), policy, func(context.Context) (string, error) {
		calls++
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Fatalf("expected 'ok', got %q", result)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestExecuteWithRetry_RetriesOnFailure(t *testing.T) {
	policy := RetryPolicy{
		MaxAttempts:       3,
		InitialBackoff:    time.Millisecond,
		MaxBackoff:        10 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}
	calls := 0
	_, err := ExecuteWithRetry[int](context.Background(), policy, func(context.Context) (int, error) {
		calls++
		return 0, errors.New("transient error")
	})
	if err == nil {
		t.Fatal("expected error after exhausted retries")
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestExecuteWithRetry_SucceedsOnSecondAttempt(t *testing.T) {
	policy := RetryPolicy{
		MaxAttempts:       3,
		InitialBackoff:    time.Millisecond,
		MaxBackoff:        10 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}
	calls := 0
	result, err := ExecuteWithRetry[string](context.Background(), policy, func(context.Context) (string, error) {
		calls++
		if calls < 2 {
			return "", errors.New("not ready")
		}
		return "ready", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ready" {
		t.Fatalf("expected 'ready', got %q", result)
	}
}

func TestExecuteWithRetry_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	policy := RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Second}
	_, err := ExecuteWithRetry[int](ctx, policy, func(ctx context.Context) (int, error) {
		// first call won't even sleep — context already done
		return 0, errors.New("error")
	})
	if err == nil {
		t.Fatal("expected error due to cancelled context")
	}
}

func TestExecuteWithRetry_OpenCircuitFastFails(t *testing.T) {
	cb := NewCircuitBreaker(1, 60*time.Second, 1)
	cb.RecordFailure() // trip circuit

	policy := DefaultRetryPolicy()
	policy.Breaker = cb

	calls := 0
	_, err := ExecuteWithRetry[int](context.Background(), policy, func(context.Context) (int, error) {
		calls++
		return 0, nil
	})
	if err == nil {
		t.Fatal("expected circuit-open error")
	}
	if calls != 0 {
		t.Fatalf("operation should not have been invoked, got %d calls", calls)
	}
}

// ===========================================================================
// Lock-Free LeastConnBalancer Tests (additional coverage)
// ===========================================================================

func TestLeastConnBalancer_EmptyTargetsError(t *testing.T) {
	lb := NewLeastConnBalancer()
	_, err := lb.Next([]string{})
	if err == nil {
		t.Fatal("expected error for empty targets")
	}
}

func TestLeastConnBalancer_LazyRegistration(t *testing.T) {
	lb := NewLeastConnBalancer()
	// Do NOT call Register — targets should be auto-registered on first Next()
	target, err := lb.Next([]string{"a:1", "b:2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lb.Done(target)
}

func TestLeastConnBalancer_ActiveConnsObservability(t *testing.T) {
	lb := NewLeastConnBalancer()
	lb.Register([]string{"x:9"})
	lb.Next([]string{"x:9"}) //nolint:errcheck
	if lb.ActiveConns("x:9") != 1 {
		t.Fatalf("expected 1 active conn, got %d", lb.ActiveConns("x:9"))
	}
	lb.Done("x:9")
	if lb.ActiveConns("x:9") != 0 {
		t.Fatalf("expected 0 active conns after Done, got %d", lb.ActiveConns("x:9"))
	}
}

// ===========================================================================
// Hedging Tests
// ===========================================================================

func TestHedging_ReturnsFastestResult(t *testing.T) {
	calls := int64(0)
	policy := RetryPolicy{
		MaxAttempts:   1,
		HedgeDelay:    5 * time.Millisecond,
		HedgeBucket:   NewHedgingTokenBucket(10, 100),
	}

	start := time.Now()
	result, err := ExecuteWithRetry[string](context.Background(), policy, func(ctx context.Context) (string, error) {
		n := atomic.AddInt64(&calls, 1)
		if n == 1 {
			// Primary is slow
			time.Sleep(200 * time.Millisecond)
			return "slow", nil
		}
		// Hedge is fast
		return "fast", nil
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "fast" {
		t.Fatalf("expected 'fast' from hedged request, got %q", result)
	}
	if elapsed > 100*time.Millisecond {
		t.Fatalf("hedging should have returned quickly, took %v", elapsed)
	}
	if atomic.LoadInt64(&calls) < 2 {
		t.Fatal("expected at least 2 calls (primary + hedge)")
	}
}

func TestHedging_TokenBucketThrottles(t *testing.T) {
	// Bucket with 0 burst — hedge should never fire
	emptyBucket := NewHedgingTokenBucket(0, 0)
	policy := RetryPolicy{
		MaxAttempts: 1,
		HedgeDelay:  5 * time.Millisecond,
		HedgeBucket: emptyBucket,
	}

	calls := int64(0)
	result, err := ExecuteWithRetry[string](context.Background(), policy, func(context.Context) (string, error) {
		atomic.AddInt64(&calls, 1)
		return "primary", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
	// Only 1 call should have been made — hedge was throttled
	if c := atomic.LoadInt64(&calls); c > 1 {
		t.Fatalf("hedge should have been throttled, but got %d calls", c)
	}
}
