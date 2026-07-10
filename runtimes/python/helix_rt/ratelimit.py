"""
helix_rt.ratelimit
~~~~~~~~~~~~~~~~~~
Per-client token bucket rate limiter matching the Go/Rust runtime implementations.

Usage::

    from helix_rt.ratelimit import RateLimiter

    limiter = RateLimiter(requests_per_second=100, burst=20)
    server.add_middleware(limiter.middleware())

The middleware injects standard X-RateLimit-* response headers and returns
HTTP 429 with Retry-After when a client exceeds their quota.
"""

from __future__ import annotations

import asyncio
import time
from typing import Callable, Awaitable, Optional

from aiohttp import web

from .errors import HelixError, ErrorCode


class _ClientBucket:
    """Asyncio-safe token bucket for a single client."""

    __slots__ = ("_tokens", "_capacity", "_rate", "_last", "_lock")

    def __init__(self, rate: float, burst: int) -> None:
        self._tokens   = float(burst)
        self._capacity = float(burst)
        self._rate     = rate           # tokens / second
        self._last     = time.monotonic()
        self._lock     = asyncio.Lock()

    async def consume(self) -> tuple[int, bool]:
        """
        Attempt to consume one token.
        Returns (remaining_tokens, allowed).
        """
        async with self._lock:
            now     = time.monotonic()
            elapsed = now - self._last
            self._last = now
            self._tokens = min(self._capacity, self._tokens + elapsed * self._rate)

            if self._tokens >= 1.0:
                self._tokens -= 1.0
                return int(self._tokens), True
            return 0, False


class RateLimiter:
    """
    Per-client token bucket rate limiter.

    Clients are identified by a key extracted from the request (default: remote IP).
    Idle buckets are never explicitly purged — for production use with many ephemeral
    clients, consider a TTL-based eviction strategy or a Redis-backed implementation.
    """

    def __init__(
        self,
        requests_per_second: float,
        burst: Optional[int] = None,
        key_func: Optional[Callable[[web.Request], str]] = None,
    ) -> None:
        self.rps     = requests_per_second
        self.burst   = burst or max(1, int(requests_per_second))
        self.key_func = key_func or _ip_key
        self._buckets: dict[str, _ClientBucket] = {}
        self._lock    = asyncio.Lock()

    async def _bucket(self, key: str) -> _ClientBucket:
        if key not in self._buckets:
            async with self._lock:
                if key not in self._buckets:
                    self._buckets[key] = _ClientBucket(self.rps, self.burst)
        return self._buckets[key]

    async def allow(self, key: str) -> tuple[int, bool]:
        """Return (remaining, allowed) for the given client key."""
        b = await self._bucket(key)
        return await b.consume()

    def middleware(self) -> Callable:
        """Return an aiohttp middleware enforcing the rate limit."""
        limiter = self

        @web.middleware
        async def _middleware(request: web.Request, handler: Callable) -> web.StreamResponse:
            key = limiter.key_func(request)
            remaining, allowed = await limiter.allow(key)
            retry_after = f"{1.0 / limiter.rps:.3f}"

            if not allowed:
                return web.Response(
                    status=429,
                    content_type="application/json",
                    headers={
                        "X-RateLimit-Limit":     str(limiter.burst),
                        "X-RateLimit-Remaining": "0",
                        "Retry-After":           retry_after,
                    },
                    body=b'{"error":"rate limit exceeded","code":14}',
                )

            response = await handler(request)
            response.headers["X-RateLimit-Limit"]     = str(limiter.burst)
            response.headers["X-RateLimit-Remaining"] = str(remaining)
            return response

        return _middleware


LUA_TOKEN_BUCKET = """
local key = KEYS[1]
local rate = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local now = tonumber(ARGV[3])

local tokens_key = key .. ":tokens"
local timestamp_key = key .. ":ts"

local last_tokens = tonumber(redis.call("get", tokens_key))
if last_tokens == nil then
    last_tokens = burst
end

local last_refreshed = tonumber(redis.call("get", timestamp_key))
if last_refreshed == nil then
    last_refreshed = 0
end

local delta = math.max(0, now - last_refreshed)
local filled_tokens = math.min(burst, last_tokens + (delta * rate))
local allowed = filled_tokens >= 1
local new_tokens = filled_tokens

if allowed then
    new_tokens = filled_tokens - 1
end

redis.call("setex", tokens_key, math.ceil(burst / rate), new_tokens)
redis.call("setex", timestamp_key, math.ceil(burst / rate), now)

return { new_tokens, allowed }
"""

class RedisRateLimiter(RateLimiter):
    """
    Globally distributed token bucket rate limiter backed by Redis.
    Uses an atomic Lua script to prevent race conditions across multiple workers.
    """

    def __init__(
        self,
        redis_client,
        requests_per_second: float,
        burst: Optional[int] = None,
        key_func: Optional[Callable[[web.Request], str]] = None,
    ) -> None:
        super().__init__(requests_per_second, burst, key_func)
        self.redis = redis_client
        self._script = self.redis.register_script(LUA_TOKEN_BUCKET)

    async def allow(self, key: str) -> tuple[int, bool]:
        """Return (remaining, allowed) for the given client key via Redis."""
        now = time.time()
        res = await self._script(
            keys=[f"ratelimit:{key}"],
            args=[self.rps, self.burst, now]
        )
        # Lua script returns [new_tokens, allowed]
        return int(res[0]), bool(res[1])

def _ip_key(request: web.Request) -> str:
    """Extract client IP, honouring X-Forwarded-For."""
    forwarded = request.headers.get("X-Forwarded-For", "")
    if forwarded:
        return forwarded.split(",")[0].strip()
    return request.remote or "unknown"
