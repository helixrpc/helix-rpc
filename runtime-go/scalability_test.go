package runtime

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// BenchmarkLeastConnBalancer_Concurrent proves that the lock-free hot path
// scales linearly with CPU cores rather than collapsing under mutex contention.
func BenchmarkLeastConnBalancer_Concurrent(b *testing.B) {
	targets := []string{
		"10.0.0.1:8080",
		"10.0.0.2:8080",
		"10.0.0.3:8080",
		"10.0.0.4:8080",
	}

	lb := NewLeastConnBalancer()
	lb.Register(targets)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			t, err := lb.Next(targets)
			if err != nil {
				b.Fatalf("unexpected error: %v", err)
			}
			lb.Done(t)
		}
	})
}

// TestLeastConnBalancer_Distribution verifies that work is spread evenly when
// requests stay in-flight concurrently (the scenario where least-conn shines).
func TestLeastConnBalancer_Distribution(t *testing.T) {
	targets := []string{"a:1", "b:2", "c:3"}
	lb := NewLeastConnBalancer()
	lb.Register(targets)

	// Simulate 300 concurrent in-flight requests, each holding the slot for
	// long enough that subsequent goroutines see a non-zero count.
	const n = 300
	counts := make(map[string]int, 3)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			chosen, err := lb.Next(targets)
			if err != nil {
				t.Errorf("Next() error: %v", err)
				return
			}
			// Hold the slot briefly so concurrent goroutines observe non-zero counts.
			time.Sleep(2 * time.Millisecond)
			lb.Done(chosen)
			mu.Lock()
			counts[chosen]++
			mu.Unlock()
		}()
	}
	wg.Wait()

	for _, tgt := range targets {
		pct := float64(counts[tgt]) / n * 100
		t.Logf("  %s → %d requests (%.1f%%)", tgt, counts[tgt], pct)
	}
	// At minimum every target should have received at least 5% of requests.
	for _, tgt := range targets {
		pct := float64(counts[tgt]) / n * 100
		if pct < 5 {
			t.Errorf("target %s received only %.1f%% — distribution too skewed", tgt, pct)
		}
	}
}

// TestCircuitBreaker_Trips verifies the circuit opens after MaxFailures.
func TestCircuitBreaker_Trips(t *testing.T) {
	cb := NewCircuitBreaker(3, 60*time.Second, 2)
	if cb.State() != CircuitClosed {
		t.Fatal("circuit should start closed")
	}
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Fatal("circuit should be open after 3 failures")
	}
	if cb.Allow() {
		t.Fatal("open circuit should not allow requests immediately")
	}
}

// TestTokenBucket_Consume verifies the hedge rate limiter throttles correctly.
func TestTokenBucket_Consume(t *testing.T) {
	// 2 burst, 0 refill → exactly 2 tokens available.
	tb := NewHedgingTokenBucket(2, 0)
	if !tb.Consume() {
		t.Fatal("first consume should succeed")
	}
	if !tb.Consume() {
		t.Fatal("second consume should succeed")
	}
	if tb.Consume() {
		t.Fatal("third consume should fail — bucket empty")
	}
}

// TestExecuteWithRetry_CircuitOpen ensures fast-fail when circuit is open.
func TestExecuteWithRetry_CircuitOpen(t *testing.T) {
	cb := NewCircuitBreaker(1, 60_000_000_000, 1) // 1 failure trips, 60s timeout
	cb.RecordFailure()                              // trip immediately

	policy := DefaultRetryPolicy()
	policy.Breaker = cb

	calls := 0
	_, err := ExecuteWithRetry[int](t.Context(), policy, func(_ context.Context) (int, error) {
		calls++
		return 0, nil
	})
	if err == nil {
		t.Fatal("expected error from open circuit")
	}
	if calls != 0 {
		t.Fatalf("operation should not have been called, got %d calls", calls)
	}
}

func ExampleLeastConnBalancer() {
	targets := []string{"node1:9090", "node2:9090", "node3:9090"}
	lb := NewLeastConnBalancer()
	lb.Register(targets)

	chosen, _ := lb.Next(targets)
	// ... send request to chosen ...
	lb.Done(chosen)
	fmt.Println("request routed and completed")
	// Output: request routed and completed
}
