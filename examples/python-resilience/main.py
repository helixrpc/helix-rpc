"""
examples/python-resilience/main.py
===================================
Complete working example demonstrating all new helix_rt resilience features:

  • HelixError — structured gRPC-compatible errors
  • CircuitBreaker — prevents cascading failures
  • RetryPolicy with exponential backoff
  • P99 Hedging with TokenBucket rate limiter
  • Graceful shutdown via SIGTERM

Run this server then test it with:

  # Trigger a NOT_FOUND error:
  curl -s -X POST http://localhost:8084/v1/model/predict \
       -H 'Content-Type: application/json' \
       -d '{"model": "unknown"}' | python -m json.tool

  # Successful prediction:
  curl -s -X POST http://localhost:8084/v1/model/predict \
       -H 'Content-Type: application/json' \
       -d '{"model": "llama3", "prompt": "hello world"}' | python -m json.tool

  # Test deadline (50ms):
  curl -s -X POST http://localhost:8084/v1/model/slow \
       -H 'grpc-timeout: 50m' \
       -H 'Content-Type: application/json' \
       -d '{}'
"""

import sys
import os
import asyncio
import random
import time

# Resolve runtime-python from relative path
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../runtime-python")))

from helix_rt.server import HelixServer
from helix_rt.errors import HelixError, ErrorCode
from helix_rt.retry import RetryPolicy, execute_with_retry, CircuitBreaker, TokenBucket
from helix_rt.batching import BatchScheduler

# ---------------------------------------------------------------------------
# Mock GPU model
# ---------------------------------------------------------------------------

LOADED_MODELS = {"llama3", "mistral", "phi3"}

async def _gpu_batch_inference(requests: list) -> list:
    """Simulate vectorised GPU batch inference."""
    print(f"[GPU] 🚀 Executing batch of {len(requests)} requests")
    await asyncio.sleep(0.05)  # simulate inference latency
    return [{"completion": f"Response to: {r.get('prompt', '?')}"} for r in requests]

# ---------------------------------------------------------------------------
# Circuit breaker — shared across all handler calls
# ---------------------------------------------------------------------------

cb = CircuitBreaker(max_failures=5, open_timeout=30.0, half_open_probes=2)

# ---------------------------------------------------------------------------
# Batch scheduler
# ---------------------------------------------------------------------------

scheduler = BatchScheduler(max_size=64, batch_window_ms=20, handler=_gpu_batch_inference)

# ---------------------------------------------------------------------------
# Retry policy with hedging
# ---------------------------------------------------------------------------

retry_policy = RetryPolicy(
    max_attempts=3,
    initial_backoff=0.05,
    max_backoff=1.0,
    backoff_multiplier=2.0,
    hedge_delay=0.1,                           # fire hedge if primary > 100ms
    hedge_bucket=TokenBucket(5, 10),           # max 10 hedges/sec, burst 5
    breaker=cb,
)

# ---------------------------------------------------------------------------
# Handlers
# ---------------------------------------------------------------------------

async def predict_handler(body: dict) -> dict:
    model = body.get("model")
    if model not in LOADED_MODELS:
        raise HelixError(
            ErrorCode.NOT_FOUND,
            f"model '{model}' is not loaded. Available: {sorted(LOADED_MODELS)}"
        )
    return await scheduler.invoke(body)


async def slow_handler(body: dict) -> dict:
    """Intentionally slow — use with grpc-timeout to test deadline propagation."""
    await asyncio.sleep(2.0)
    return {"status": "done"}


async def resilient_handler(body: dict) -> dict:
    """
    Wraps predict_handler with retry + circuit breaker.
    On flaky backends, this transparently retries up to 3 times with
    exponential backoff and fires a hedge request if the primary is slow.
    """
    async def operation():
        return await predict_handler(body)

    return await execute_with_retry(retry_policy, operation)


async def health_handler(body: dict) -> dict:
    return {"status": "serving", "loaded_models": sorted(LOADED_MODELS)}


# ---------------------------------------------------------------------------
# Server setup
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    server = HelixServer(host="127.0.0.1", port=8084)

    # 1. Add JWT authentication middleware (secret: "example-secret-key-that-is-long-enough")
    from helix_rt.auth import jwt_middleware
    server.add_middleware(jwt_middleware(secret="example-secret-key-that-is-long-enough"))

    # 2. Add Rate Limiter middleware (10 requests per second, burst 5)
    from helix_rt.ratelimit import RateLimiter
    limiter = RateLimiter(requests_per_second=10.0, burst=5)
    server.add_middleware(limiter.middleware())

    server.register_route("POST", "/v1/model/predict",   predict_handler)
    server.register_route("POST", "/v1/model/resilient", resilient_handler)
    server.register_route("POST", "/v1/model/slow",      slow_handler)
    server.register_route("GET",  "/health",             health_handler)

    print("📋 Routes:")
    print("  POST /v1/model/predict   — (JWT auth & RateLimited) direct prediction")
    print("  POST /v1/model/resilient — (JWT auth & RateLimited) prediction with retry + circuit breaker")
    print("  POST /v1/model/slow      — (JWT auth & RateLimited) slow endpoint")
    print("  GET  /health             — (JWT auth & RateLimited) health check")

    server.start()
