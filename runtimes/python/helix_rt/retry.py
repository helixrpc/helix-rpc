"""
helix_rt.retry
~~~~~~~~~~~~~~
Exponential backoff, Circuit Breaker, Token Bucket, and P99 Hedging —
full Python port of the Go runtime-go/retry.go implementation.

Usage::

    from helix_rt.retry import RetryPolicy, execute_with_retry, CircuitBreaker

    policy = RetryPolicy.with_defaults()

    async def call_model():
        ...

    result = await execute_with_retry(policy, call_model)
"""

from __future__ import annotations

import asyncio
import random
import time
from dataclasses import dataclass, field
from enum import IntEnum
from typing import Callable, Awaitable, Optional, TypeVar

T = TypeVar("T")


# ---------------------------------------------------------------------------
# Circuit Breaker
# ---------------------------------------------------------------------------

class CircuitState(IntEnum):
    CLOSED    = 0
    OPEN      = 1
    HALF_OPEN = 2


class CircuitBreaker:
    """
    Thread-safe (asyncio-safe) circuit breaker matching the Go/Rust implementations.

    State machine:
      CLOSED  → normal traffic
      OPEN    → fast-fail; re-checks after open_timeout
      HALF_OPEN → one probe request; closes on success, reopens on failure
    """

    def __init__(
        self,
        max_failures: int = 5,
        open_timeout: float = 30.0,  # seconds
        half_open_probes: int = 2,
    ) -> None:
        self.max_failures     = max_failures
        self.open_timeout     = open_timeout
        self.half_open_probes = half_open_probes

        self._state     = CircuitState.CLOSED
        self._failures  = 0
        self._successes = 0
        self._opened_at = 0.0
        self._lock      = asyncio.Lock()

    async def state(self) -> CircuitState:
        async with self._lock:
            return self._state

    async def allow(self) -> bool:
        async with self._lock:
            if self._state == CircuitState.CLOSED:
                return True
            if self._state == CircuitState.HALF_OPEN:
                return True
            # OPEN: check timeout
            if time.monotonic() - self._opened_at >= self.open_timeout:
                self._state     = CircuitState.HALF_OPEN
                self._successes = 0
                return True
            return False

    async def record_success(self) -> None:
        async with self._lock:
            if self._state == CircuitState.HALF_OPEN:
                self._successes += 1
                if self._successes >= self.half_open_probes:
                    self._state    = CircuitState.CLOSED
                    self._failures = 0
            elif self._state == CircuitState.CLOSED:
                self._failures = 0

    async def record_failure(self) -> None:
        async with self._lock:
            self._failures += 1
            if self._failures >= self.max_failures and self._state == CircuitState.CLOSED:
                self._state     = CircuitState.OPEN
                self._opened_at = time.monotonic()


# ---------------------------------------------------------------------------
# Token Bucket (hedging rate limiter)
# ---------------------------------------------------------------------------

class TokenBucket:
    """
    Thread-safe token bucket for limiting hedged request rate.
    Prevents hedge-induced thundering-herd during cluster-wide latency spikes.
    """

    def __init__(self, capacity: int, rate_per_second: float) -> None:
        self.capacity      = capacity
        self.rate          = rate_per_second  # tokens per second
        self._tokens       = float(capacity)
        self._last_refill  = time.monotonic()
        self._lock         = asyncio.Lock()

    async def consume(self) -> bool:
        async with self._lock:
            now     = time.monotonic()
            elapsed = now - self._last_refill
            self._tokens = min(self.capacity, self._tokens + elapsed * self.rate)
            self._last_refill = now

            if self._tokens >= 1.0:
                self._tokens -= 1.0
                return True
            return False


# ---------------------------------------------------------------------------
# RetryPolicy
# ---------------------------------------------------------------------------

@dataclass
class RetryPolicy:
    max_attempts:       int   = 3
    initial_backoff:    float = 0.1   # seconds
    max_backoff:        float = 2.0   # seconds
    backoff_multiplier: float = 2.0
    hedge_delay:        Optional[float] = None  # seconds; None disables hedging
    hedge_bucket:       Optional[TokenBucket]    = None
    breaker:            Optional[CircuitBreaker] = None

    @classmethod
    def with_defaults(cls) -> "RetryPolicy":
        """Production-safe policy: 3 retries, 10 hedges/s, 5-failure circuit breaker."""
        return cls(
            breaker      = CircuitBreaker(max_failures=5, open_timeout=30.0, half_open_probes=2),
            hedge_bucket = TokenBucket(capacity=5, rate_per_second=10.0),
        )


# ---------------------------------------------------------------------------
# execute_with_retry
# ---------------------------------------------------------------------------

async def execute_with_retry(
    policy: RetryPolicy,
    operation: Callable[[], Awaitable[T]],
) -> T:
    """Run `operation` with the given RetryPolicy (backoff + hedging + circuit breaker)."""

    if policy.breaker and not await policy.breaker.allow():
        raise RuntimeError("circuit open: too many recent failures")

    if policy.hedge_delay is not None:
        return await _execute_with_hedging(policy, operation)

    return await _execute_retries_only(policy, operation)


async def _execute_retries_only(policy: RetryPolicy, operation: Callable[[], Awaitable[T]]) -> T:
    backoff   = policy.initial_backoff
    last_err: Optional[Exception] = None

    for attempt in range(1, policy.max_attempts + 1):
        try:
            result = await operation()
            if policy.breaker:
                await policy.breaker.record_success()
            return result
        except Exception as e:
            if policy.breaker:
                await policy.breaker.record_failure()
            last_err = e
            if attempt == policy.max_attempts:
                break
            # Full-jitter backoff: sleep rand[0, backoff)
            sleep_duration = random.uniform(0, backoff)
            await asyncio.sleep(sleep_duration)
            backoff = min(backoff * policy.backoff_multiplier, policy.max_backoff)

    raise last_err  # type: ignore[misc]


async def _execute_with_hedging(policy: RetryPolicy, operation: Callable[[], Awaitable[T]]) -> T:
    result_queue: asyncio.Queue = asyncio.Queue(maxsize=2)

    async def run_attempt():
        try:
            r = await _execute_retries_only(policy, operation)
            await result_queue.put(("ok", r))
        except Exception as e:
            await result_queue.put(("err", e))

    # Primary attempt
    asyncio.ensure_future(run_attempt())

    try:
        # Wait for primary to finish or hedge_delay to expire
        first = await asyncio.wait_for(result_queue.get(), timeout=policy.hedge_delay)
        status, value = first
        if status == "ok":
            return value  # type: ignore[return-value]
    except asyncio.TimeoutError:
        pass  # Hedge delay expired; fire duplicate

    # Rate-limit the hedge
    can_hedge = True
    if policy.hedge_bucket:
        can_hedge = await policy.hedge_bucket.consume()

    if can_hedge:
        asyncio.ensure_future(run_attempt())

    # Return whichever completes first with success
    for _ in range(2):
        status, value = await result_queue.get()
        if status == "ok":
            return value  # type: ignore[return-value]

    raise value  # type: ignore[misc]
