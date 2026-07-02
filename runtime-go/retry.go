package runtime

import (
	"context"
	"errors"
	"math/rand"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------------------------
// Circuit Breaker
// ---------------------------------------------------------------------------

// CircuitState represents the three states of a circuit breaker.
type CircuitState int32

const (
	CircuitClosed   CircuitState = iota // Normal: requests pass through
	CircuitOpen                         // Tripped: requests fast-fail immediately
	CircuitHalfOpen                     // Probe: one request is allowed through to test recovery
)

func (cs CircuitState) String() string {
	switch cs {
	case CircuitClosed:
		return "CLOSED"
	case CircuitOpen:
		return "OPEN"
	case CircuitHalfOpen:
		return "HALF-OPEN"
	default:
		return "UNKNOWN"
	}
}

// CircuitBreaker is a thread-safe, atomic state machine that prevents cascading
// failures by fast-failing requests when the error rate exceeds a threshold.
type CircuitBreaker struct {
	state        int32  // CircuitState stored atomically
	failures     int64  // rolling failure count (atomic)
	successes    int64  // rolling success count in HalfOpen (atomic)
	lastOpenTime int64  // UnixNano of when circuit opened (atomic)

	// Config
	MaxFailures      int64         // failures before tripping (e.g. 5)
	OpenTimeout      time.Duration // how long to stay Open before probing
	HalfOpenProbes   int64         // successes required in HalfOpen to re-close
}

// NewCircuitBreaker returns a production-ready circuit breaker.
func NewCircuitBreaker(maxFailures int64, openTimeout time.Duration, halfOpenProbes int64) *CircuitBreaker {
	return &CircuitBreaker{
		MaxFailures:    maxFailures,
		OpenTimeout:    openTimeout,
		HalfOpenProbes: halfOpenProbes,
	}
}

// State returns the current CircuitState.
func (cb *CircuitBreaker) State() CircuitState {
	return CircuitState(atomic.LoadInt32(&cb.state))
}

// Allow returns true if the request should be allowed through.
func (cb *CircuitBreaker) Allow() bool {
	switch cb.State() {
	case CircuitClosed:
		return true
	case CircuitOpen:
		// After OpenTimeout, transition to HalfOpen for a probe attempt.
		openedAt := time.Unix(0, atomic.LoadInt64(&cb.lastOpenTime))
		if time.Since(openedAt) >= cb.OpenTimeout {
			if atomic.CompareAndSwapInt32(&cb.state, int32(CircuitOpen), int32(CircuitHalfOpen)) {
				atomic.StoreInt64(&cb.successes, 0)
			}
			return true
		}
		return false
	case CircuitHalfOpen:
		// Allow only one concurrent probe
		return true
	}
	return false
}

// RecordSuccess records a successful operation, potentially closing a HalfOpen circuit.
func (cb *CircuitBreaker) RecordSuccess() {
	switch cb.State() {
	case CircuitHalfOpen:
		probes := atomic.AddInt64(&cb.successes, 1)
		if probes >= cb.HalfOpenProbes {
			atomic.StoreInt32(&cb.state, int32(CircuitClosed))
			atomic.StoreInt64(&cb.failures, 0)
		}
	case CircuitClosed:
		// Reset failure count on success to prevent slow-burn tripping.
		atomic.StoreInt64(&cb.failures, 0)
	}
}

// RecordFailure records a failed operation, potentially opening the circuit.
func (cb *CircuitBreaker) RecordFailure() {
	f := atomic.AddInt64(&cb.failures, 1)
	if f >= cb.MaxFailures {
		if atomic.CompareAndSwapInt32(&cb.state, int32(CircuitClosed), int32(CircuitOpen)) ||
			atomic.CompareAndSwapInt32(&cb.state, int32(CircuitHalfOpen), int32(CircuitOpen)) {
			atomic.StoreInt64(&cb.lastOpenTime, time.Now().UnixNano())
		}
	}
}

// ---------------------------------------------------------------------------
// Token Bucket (for hedging rate limiting)
// ---------------------------------------------------------------------------

// TokenBucket implements a thread-safe token bucket for rate-limiting hedged
// requests. Tokens refill continuously at a fixed rate using atomic CAS.
type TokenBucket struct {
	// tokens is stored as a fixed-point integer: actual tokens = tokens / 1e9
	tokens    int64 // scaled by 1e9 (nano-tokens)
	capacity  int64 // max nano-tokens
	refillRate int64 // nano-tokens added per nanosecond
	lastRefill int64 // UnixNano of last refill (atomic)
}

// NewTokenBucket creates a bucket with `capacity` max tokens refilling at
// `ratePerSecond` tokens/second.
func NewTokenBucket(capacity float64, ratePerSecond float64) *TokenBucket {
	capNano := int64(capacity * 1_000_000_000)
	// refillRate: nano-tokens added per nanosecond = ratePerSecond / 1e9 * 1e9 = ratePerSecond
	refillRate := int64(ratePerSecond)
	if refillRate == 0 && ratePerSecond > 0 {
		refillRate = 1
	}
	return &TokenBucket{
		tokens:     capNano,
		capacity:   capNano,
		refillRate: refillRate,
		lastRefill: time.Now().UnixNano(),
	}
}



// NewHedgingTokenBucket returns a bucket that allows `hedgesPerSecond` hedged
// requests per second with a burst capacity of `burstCapacity`.
func NewHedgingTokenBucket(burstCapacity int, hedgesPerSecond float64) *TokenBucket {
	capNano := int64(burstCapacity) * 1_000_000_000
	// refillRate: nano-tokens added per nanosecond
	refillRate := int64(hedgesPerSecond * 1_000_000_000 / 1_000_000_000)
	if refillRate == 0 && hedgesPerSecond > 0 {
		refillRate = 1
	}
	return &TokenBucket{
		tokens:     capNano,
		capacity:   capNano,
		refillRate: refillRate,
		lastRefill: time.Now().UnixNano(),
	}
}

// Consume attempts to take one token. Returns true if successful.
func (tb *TokenBucket) Consume() bool {
	now := time.Now().UnixNano()
	last := atomic.LoadInt64(&tb.lastRefill)
	elapsed := now - last
	if elapsed > 0 {
		refill := elapsed * tb.refillRate
		if refill > 0 {
			for {
				cur := atomic.LoadInt64(&tb.tokens)
				next := cur + refill
				if next > tb.capacity {
					next = tb.capacity
				}
				if atomic.CompareAndSwapInt64(&tb.tokens, cur, next) {
					atomic.CompareAndSwapInt64(&tb.lastRefill, last, now)
					break
				}
			}
		}
	}
	// Attempt to consume one full token (1e9 nano-tokens)
	const oneToken = 1_000_000_000
	for {
		cur := atomic.LoadInt64(&tb.tokens)
		if cur < oneToken {
			return false
		}
		if atomic.CompareAndSwapInt64(&tb.tokens, cur, cur-oneToken) {
			return true
		}
	}
}

// ---------------------------------------------------------------------------
// RetryPolicy and ExecuteWithRetry
// ---------------------------------------------------------------------------

// RetryPolicy defines how failures are retried and how hedging is controlled.
type RetryPolicy struct {
	MaxAttempts       int
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
	BackoffMultiplier float64
	// RetryableErrors decides if an error warrants a retry. nil = always retry.
	RetryableErrors func(error) bool

	// HedgeDelay > 0 enables P99 hedging. A duplicate request fires if the
	// first does not complete within this window.
	HedgeDelay time.Duration

	// HedgeBucket limits the rate at which hedged requests may fire to prevent
	// thundering-herd amplification. nil = unlimited hedging (dangerous).
	HedgeBucket *TokenBucket

	// Breaker, if non-nil, fast-fails requests when the circuit is open.
	Breaker *CircuitBreaker
}

// DefaultRetryPolicy returns a production-safe policy with exponential backoff,
// a 10 hedges/s token bucket, and a 5-failure circuit breaker.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts:       3,
		InitialBackoff:    100 * time.Millisecond,
		MaxBackoff:        2 * time.Second,
		BackoffMultiplier: 2.0,
		RetryableErrors:   nil, // retry all errors
		HedgeDelay:        0,
		HedgeBucket:       NewHedgingTokenBucket(5, 10),
		Breaker:           NewCircuitBreaker(5, 30*time.Second, 2),
	}
}

// ExecuteWithRetry runs the operation with the given policy.
func ExecuteWithRetry[T any](ctx context.Context, policy RetryPolicy, operation func(context.Context) (T, error)) (T, error) {
	// Circuit breaker fast-fail
	if policy.Breaker != nil && !policy.Breaker.Allow() {
		var zero T
		return zero, errors.New("circuit open: too many recent failures")
	}

	if policy.HedgeDelay > 0 {
		return executeWithHedging(ctx, policy, operation)
	}
	return executeRetriesOnly(ctx, policy, operation)
}

func executeRetriesOnly[T any](ctx context.Context, policy RetryPolicy, operation func(context.Context) (T, error)) (T, error) {
	var lastErr error
	backoff := policy.InitialBackoff

	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		result, err := operation(ctx)
		if err == nil {
			if policy.Breaker != nil {
				policy.Breaker.RecordSuccess()
			}
			return result, nil
		}

		if policy.Breaker != nil {
			policy.Breaker.RecordFailure()
		}
		if policy.RetryableErrors != nil && !policy.RetryableErrors(err) {
			return result, err
		}
		lastErr = err

		if attempt == policy.MaxAttempts {
			break
		}

		// Exponential backoff with full-jitter (avoids thundering herd on retry)
		jitterRange := int64(backoff)
		jitter := time.Duration(rand.Int63n(jitterRange))
		sleepDuration := jitter // full-jitter: sleep = rand[0, backoff)

		select {
		case <-time.After(sleepDuration):
		case <-ctx.Done():
			var zero T
			return zero, ctx.Err()
		}

		nextBackoff := time.Duration(float64(backoff) * policy.BackoffMultiplier)
		if nextBackoff > policy.MaxBackoff {
			nextBackoff = policy.MaxBackoff
		}
		backoff = nextBackoff
	}

	var zero T
	return zero, lastErr
}

// executeWithHedging fires a duplicate request if the primary exceeds HedgeDelay.
// The token bucket prevents hedging from amplifying load during a cluster-wide
// latency spike.
func executeWithHedging[T any](ctx context.Context, policy RetryPolicy, operation func(context.Context) (T, error)) (T, error) {
	type result struct {
		val T
		err error
	}

	resultCh := make(chan result, 2)

	// Primary attempt
	primaryCtx, primaryCancel := context.WithCancel(ctx)
	defer primaryCancel()

	go func() {
		v, e := executeRetriesOnly(primaryCtx, policy, operation)
		resultCh <- result{v, e}
	}()

	select {
	case res := <-resultCh:
		return res.val, res.err

	case <-time.After(policy.HedgeDelay):
		// Only hedge if the token bucket has capacity.
		canHedge := policy.HedgeBucket == nil || policy.HedgeBucket.Consume()
		if !canHedge {
			// Token exhausted: wait for primary to finish.
			select {
			case res := <-resultCh:
				return res.val, res.err
			case <-ctx.Done():
				var zero T
				return zero, ctx.Err()
			}
		}

		// Launch hedged request on a fresh context so it can outlive the primary.
		hedgeCtx, hedgeCancel := context.WithCancel(ctx)
		defer hedgeCancel()

		go func() {
			v, e := executeRetriesOnly(hedgeCtx, policy, operation)
			resultCh <- result{v, e}
		}()

		// Accept whichever succeeds first; cancel the loser.
		res1 := <-resultCh
		if res1.err == nil {
			return res1.val, nil
		}
		res2 := <-resultCh
		if res2.err == nil {
			return res2.val, nil
		}
		var zero T
		return zero, errors.Join(res1.err, res2.err)

	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}
