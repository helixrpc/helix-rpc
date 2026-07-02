// examples/go-resilience/main.go
//
// Complete working example demonstrating all new helix_rt Go resilience features:
//
//   - LeastConnBalancer (lock-free)
//   - CircuitBreaker (atomic FSM)
//   - RetryPolicy with exponential full-jitter backoff
//   - P99 Hedging with TokenBucket rate limiter
//
// Run:
//   go run .
//
// Test:
//   curl -s http://localhost:8085/route    (shows least-conn routing)
//   curl -s http://localhost:8085/resilient (shows retry + circuit breaker)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	runtime "github.com/helix-rpc/helix/runtime-go"
)

// ---------------------------------------------------------------------------
// Simulated AI backend pool
// ---------------------------------------------------------------------------

var backends = []string{"gpu-node-1:9090", "gpu-node-2:9090", "gpu-node-3:9090"}

var balancer = func() *runtime.LeastConnBalancer {
	lb := runtime.NewLeastConnBalancer()
	lb.Register(backends)
	return lb
}()

// ---------------------------------------------------------------------------
// Circuit Breaker + Retry Policy
// ---------------------------------------------------------------------------

var policy = runtime.RetryPolicy{
	MaxAttempts:       3,
	InitialBackoff:    50 * time.Millisecond,
	MaxBackoff:        1 * time.Second,
	BackoffMultiplier: 2.0,
	HedgeDelay:        100 * time.Millisecond,
	HedgeBucket:       runtime.NewHedgingTokenBucket(5, 10),
	Breaker:           runtime.NewCircuitBreaker(5, 30*time.Second, 2),
}

// ---------------------------------------------------------------------------
// Mock AI inference
// ---------------------------------------------------------------------------

func callBackend(ctx context.Context, target string) (string, error) {
	// Simulate 10% error rate and variable latency
	if rand.Float64() < 0.1 {
		return "", fmt.Errorf("backend %s: transient error", target)
	}
	latency := time.Duration(rand.Intn(80)+10) * time.Millisecond
	select {
	case <-time.After(latency):
		return fmt.Sprintf(`{"completion":"Response from %s","latency_ms":%d}`, target, latency.Milliseconds()), nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// ---------------------------------------------------------------------------
// HTTP Handlers
// ---------------------------------------------------------------------------

func routeHandler(w http.ResponseWriter, r *http.Request) {
	target, err := balancer.Next(backends)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer balancer.Done(target)

	result, err := callBackend(r.Context(), target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("X-Backend", target)
	w.Header().Set("X-Active-Conns", fmt.Sprintf("%d", balancer.ActiveConns(target)))
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, result)
}

func resilientHandler(w http.ResponseWriter, r *http.Request) {
	result, err := runtime.ExecuteWithRetry[string](r.Context(), policy, func(ctx context.Context) (string, error) {
		target, terr := balancer.Next(backends)
		if terr != nil {
			return "", terr
		}
		res, cerr := callBackend(ctx, target)
		if cerr != nil {
			balancer.Done(target)
			return "", cerr
		}
		balancer.Done(target)
		return res, nil
	})
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, result)
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	stats := make(map[string]int64, len(backends))
	for _, b := range backends {
		stats[b] = balancer.ActiveConns(b)
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"balancer": stats,
		"circuit":  policy.Breaker.State(),
	})
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	// Create rate limiter (10 req/s, burst of 5)
	limiter := runtime.NewRateLimiter(runtime.RateLimitConfig{
		RequestsPerSecond: 10.0,
		BurstSize:         5,
	})

	// Create JWT configuration
	authCfg := runtime.JWTAuthConfig{
		SecretKey: []byte("example-secret-key-that-is-long-enough"),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/route",     routeHandler)

	// Wrap /resilient with JWT auth and rate limiting middlewares
	resilientHandlerWithMiddleware := runtime.NewJWTMiddleware(authCfg, limiter.HTTPMiddleware(http.HandlerFunc(resilientHandler)))
	mux.Handle("/resilient", resilientHandlerWithMiddleware)

	mux.HandleFunc("/status",    statusHandler)

	log.Println("📋 Routes:")
	log.Println("  GET /route     — least-conn routed backend call")
	log.Println("  GET /resilient — (JWT auth & RateLimited) retry + circuit breaker + hedging")
	log.Println("  GET /status    — balancer & circuit state")
	log.Println("🚀 Go resilience example listening on :8085")
	log.Fatal(http.ListenAndServe(":8085", mux))
}
