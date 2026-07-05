"""
tests/test_resilience.py
========================
Unit tests for helix_rt.errors, helix_rt.retry, and helix_rt.server resilience
features added in the parity sprint.

Run with:
    cd runtime-python
    source venv/bin/activate
    pytest tests/test_resilience.py -v
"""

import asyncio
import pytest
import sys
import os

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "..")))

from helix_rt.errors import HelixError, ErrorCode, map_to_http_status
from helix_rt.retry import (
    CircuitBreaker,
    CircuitState,
    TokenBucket,
    RetryPolicy,
    execute_with_retry,
)


# ---------------------------------------------------------------------------
# HelixError / ErrorCode Tests
# ---------------------------------------------------------------------------

class TestHelixError:
    def test_basic_creation(self):
        err = HelixError(ErrorCode.NOT_FOUND, "model not loaded")
        assert err.code == ErrorCode.NOT_FOUND
        assert err.message == "model not loaded"

    def test_str_representation(self):
        err = HelixError(ErrorCode.INTERNAL, "gpu exploded")
        assert "INTERNAL" in str(err)
        assert "gpu exploded" in str(err)

    def test_http_status_mapping(self):
        cases = {
            ErrorCode.OK:                200,
            ErrorCode.INVALID_ARGUMENT:  400,
            ErrorCode.NOT_FOUND:         404,
            ErrorCode.ALREADY_EXISTS:    409,
            ErrorCode.PERMISSION_DENIED: 403,
            ErrorCode.UNIMPLEMENTED:     501,
            ErrorCode.INTERNAL:          500,
            ErrorCode.UNAVAILABLE:       503,
            ErrorCode.UNAUTHENTICATED:   401,
        }
        for code, expected_status in cases.items():
            err = HelixError(code, "msg")
            assert err.http_status == expected_status, f"wrong status for {code}"

    def test_grpc_status_string(self):
        err = HelixError(ErrorCode.NOT_FOUND, "x")
        assert err.grpc_status == "5"

    def test_is_exception(self):
        with pytest.raises(HelixError):
            raise HelixError(ErrorCode.UNAVAILABLE, "service down")

    def test_map_to_http_status_helper(self):
        assert map_to_http_status(ErrorCode.UNAUTHENTICATED) == 401


# ---------------------------------------------------------------------------
# CircuitBreaker Tests
# ---------------------------------------------------------------------------

class TestCircuitBreaker:
    def test_initial_state_closed(self):
        cb = CircuitBreaker()
        assert asyncio.run(cb.state()) == CircuitState.CLOSED

    def test_allows_when_closed(self):
        cb = CircuitBreaker()
        assert asyncio.run(cb.allow()) is True

    def test_opens_after_max_failures(self):
        cb = CircuitBreaker(max_failures=3)
        async def run():
            for _ in range(3):
                await cb.record_failure()
            assert await cb.state() == CircuitState.OPEN
        asyncio.run(run())

    def test_does_not_allow_when_open(self):
        cb = CircuitBreaker(max_failures=1)
        async def run():
            await cb.record_failure()
            return await cb.allow()
        assert asyncio.run(run()) is False

    def test_success_resets_failure_count(self):
        cb = CircuitBreaker(max_failures=3)
        async def run():
            await cb.record_failure()
            await cb.record_failure()
            await cb.record_success()  # reset
            await cb.record_failure()
            return await cb.state()
        assert asyncio.run(run()) == CircuitState.CLOSED

    def test_transitions_to_half_open_after_timeout(self):
        cb = CircuitBreaker(max_failures=1, open_timeout=0.05)
        async def run():
            await cb.record_failure()
            assert await cb.state() == CircuitState.OPEN
            await asyncio.sleep(0.06)
            allowed = await cb.allow()
            return allowed, await cb.state()
        allowed, state = asyncio.run(run())
        assert allowed is True
        assert state == CircuitState.HALF_OPEN

    def test_closes_after_half_open_probes(self):
        cb = CircuitBreaker(max_failures=1, open_timeout=0.01, half_open_probes=2)
        async def run():
            await cb.record_failure()
            await asyncio.sleep(0.02)
            await cb.allow()  # → HalfOpen
            await cb.record_success()
            await cb.record_success()  # 2 probes → Closed
            return await cb.state()
        assert asyncio.run(run()) == CircuitState.CLOSED


# ---------------------------------------------------------------------------
# TokenBucket Tests
# ---------------------------------------------------------------------------

class TestTokenBucket:
    def test_consumes_up_to_capacity(self):
        async def run():
            tb = TokenBucket(capacity=3, rate_per_second=0)
            results = [await tb.consume() for _ in range(5)]
            return results
        results = asyncio.run(run())
        assert results[:3] == [True, True, True]
        assert results[3] is False
        assert results[4] is False

    def test_refills_over_time(self):
        async def run():
            tb = TokenBucket(capacity=1, rate_per_second=50)
            await tb.consume()  # drain
            assert not await tb.consume()
            await asyncio.sleep(0.05)   # wait ~2.5 tokens to refill
            return await tb.consume()
        assert asyncio.run(run()) is True

    def test_concurrent_consume_respects_capacity(self):
        async def run():
            tb = TokenBucket(capacity=5, rate_per_second=0)
            tasks = [tb.consume() for _ in range(20)]
            results = await asyncio.gather(*tasks)
            return sum(1 for r in results if r)
        consumed = asyncio.run(run())
        assert consumed == 5


# ---------------------------------------------------------------------------
# RetryPolicy / execute_with_retry Tests
# ---------------------------------------------------------------------------

class TestExecuteWithRetry:
    def test_succeeds_first_attempt(self):
        policy = RetryPolicy(max_attempts=3)
        calls = []
        async def op():
            calls.append(1)
            return "ok"
        result = asyncio.run(execute_with_retry(policy, op))
        assert result == "ok"
        assert len(calls) == 1

    def test_retries_on_failure(self):
        policy = RetryPolicy(max_attempts=3, initial_backoff=0.001, max_backoff=0.01)
        calls = []
        async def op():
            calls.append(1)
            raise ValueError("transient")
        with pytest.raises(ValueError):
            asyncio.run(execute_with_retry(policy, op))
        assert len(calls) == 3

    def test_succeeds_on_retry(self):
        policy = RetryPolicy(max_attempts=3, initial_backoff=0.001)
        calls = []
        async def op():
            calls.append(1)
            if len(calls) < 2:
                raise RuntimeError("not ready")
            return "ready"
        result = asyncio.run(execute_with_retry(policy, op))
        assert result == "ready"
        assert len(calls) == 2

    def test_circuit_open_fast_fails(self):
        cb = CircuitBreaker(max_failures=1)
        async def run():
            await cb.record_failure()  # trip circuit
        asyncio.run(run())

        policy = RetryPolicy(max_attempts=3, breaker=cb)
        calls = []
        async def op():
            calls.append(1)
            return "ok"
        with pytest.raises(RuntimeError, match="circuit open"):
            asyncio.run(execute_with_retry(policy, op))
        assert len(calls) == 0

    def test_hedging_returns_fastest(self):
        import time
        policy = RetryPolicy(
            max_attempts=1,
            hedge_delay=0.01,
            hedge_bucket=TokenBucket(capacity=10, rate_per_second=100),
        )
        call_count = 0
        async def op():
            nonlocal call_count
            call_count += 1
            n = call_count
            if n == 1:
                await asyncio.sleep(2)   # primary is very slow
                return "slow"
            return "fast"                # hedge is instant

        start = time.monotonic()
        result = asyncio.run(execute_with_retry(policy, op))
        elapsed = time.monotonic() - start

        assert result == "fast"
        assert elapsed < 0.5, f"hedging should be fast, took {elapsed:.3f}s"
        assert call_count >= 2

    def test_hedging_token_bucket_throttles(self):
        """When the token bucket is exhausted, no hedge fires — primary result is returned."""
        policy = RetryPolicy(
            max_attempts=1,
            hedge_delay=0.01,
            hedge_bucket=TokenBucket(capacity=0, rate_per_second=0),
        )
        calls = []
        async def op():
            calls.append(1)
            return "primary"
        result = asyncio.run(execute_with_retry(policy, op))
        assert result == "primary"
        assert len(calls) <= 1   # hedge should have been throttled


# ---------------------------------------------------------------------------
# HelixServer Integration (smoke test)
# ---------------------------------------------------------------------------

class TestHelixServerIntegration:
    def test_helix_error_converted_to_http_status(self):
        """HelixError raised inside a handler returns the correct HTTP status."""
        from aiohttp.test_utils import AioHTTPTestCase, unittest_run_loop
        from aiohttp import web
        import json

        async def run():
            from helix_rt.server import HelixServer
            server = HelixServer()

            async def bad_handler(body):
                raise HelixError(ErrorCode.NOT_FOUND, "model not found")

            server.register_route("POST", "/predict", bad_handler)

            from aiohttp.test_utils import TestClient, TestServer
            client = TestClient(TestServer(server.app))
            await client.start_server()
            resp = await client.post("/predict", json={"prompt": "hello"})
            body = await resp.json()
            await client.close()
            return resp.status, body

        status, body = asyncio.run(run())
        assert status == 404
        assert body["code"] == "5"

    def test_graceful_shutdown_api_exists(self):
        from helix_rt.server import HelixServer
        server = HelixServer()
        assert hasattr(server, "start_async")
        assert hasattr(server, "stop_async")


# ---------------------------------------------------------------------------
# Auth Middleware Tests
# ---------------------------------------------------------------------------

class TestAuthMiddleware:
    def test_jwt_middleware_success(self):
        import time
        import jwt as pyjwt
        from helix_rt.auth import jwt_middleware
        from aiohttp import web
        
        async def run():
            app = web.Application()
            app.middlewares.append(jwt_middleware(secret="secret", required_claims=["sub"]))
            
            async def ok_handler(request):
                assert request["jwt_claims"]["sub"] == "user123"
                return web.json_response({"ok": True})
            
            app.router.add_post("/test", ok_handler)
            
            token = pyjwt.encode({"sub": "user123", "exp": int(time.time()) + 3600}, "secret", algorithm="HS256")
            
            from aiohttp.test_utils import TestClient, TestServer
            client = TestClient(TestServer(app))
            await client.start_server()
            resp = await client.post("/test", headers={"Authorization": f"Bearer {token}"})
            body = await resp.json()
            await client.close()
            return resp.status, body

        status, body = asyncio.run(run())
        assert status == 200
        assert body["ok"] is True

    def test_api_key_middleware_success(self):
        from helix_rt.auth import api_key_middleware
        from aiohttp import web
        
        async def run():
            app = web.Application()
            app.middlewares.append(api_key_middleware(valid_keys={"key123": "user_principal"}))
            
            async def ok_handler(request):
                assert request["api_key_principal"] == "user_principal"
                return web.json_response({"ok": True})
            
            app.router.add_post("/test", ok_handler)
            
            from aiohttp.test_utils import TestClient, TestServer
            client = TestClient(TestServer(app))
            await client.start_server()
            resp = await client.post("/test", headers={"X-API-Key": "key123"})
            body = await resp.json()
            await client.close()
            return resp.status, body

        status, body = asyncio.run(run())
        assert status == 200
        assert body["ok"] is True


# ---------------------------------------------------------------------------
# Rate Limiter Middleware Tests
# ---------------------------------------------------------------------------

class TestRateLimiterMiddleware:
    def test_ratelimit_middleware(self):
        from helix_rt.ratelimit import RateLimiter
        from aiohttp import web
        
        async def run():
            app = web.Application()
            limiter = RateLimiter(requests_per_second=100.0, burst=2)
            app.middlewares.append(limiter.middleware())
            
            async def ok_handler(request):
                return web.json_response({"ok": True})
            
            app.router.add_post("/test", ok_handler)
            
            from aiohttp.test_utils import TestClient, TestServer
            client = TestClient(TestServer(app))
            await client.start_server()
            
            # First request
            r1 = await client.post("/test")
            # Second request
            r2 = await client.post("/test")
            # Third request (should be rate limited)
            r3 = await client.post("/test")
            
            await client.close()
            return r1.status, r2.status, r3.status

        s1, s2, s3 = asyncio.run(run())
        assert s1 == 200
        assert s2 == 200
        assert s3 == 429


# ---------------------------------------------------------------------------
# gRPC-Web Middleware Tests
# ---------------------------------------------------------------------------

class TestGrpcWebSupport:
    def test_grpc_web_success(self):
        from helix_rt.server import HelixServer
        import json

        async def run():
            server = HelixServer()
            
            async def hello_handler(body):
                return {"message": f"Hello {body.get('name', 'World')}"}

            server.register_route("POST", "/hello", hello_handler)

            from aiohttp.test_utils import TestClient, TestServer
            client = TestClient(TestServer(server.app))
            await client.start_server()

            # Construct gRPC-Web payload: 1 byte flag (0), 4 bytes length, then JSON payload
            payload_str = json.dumps({"name": "test-grpc-web"})
            payload_bytes = payload_str.encode("utf-8")
            header = b"\x00" + len(payload_bytes).to_bytes(4, byteorder="big")
            request_body = header + payload_bytes

            resp = await client.post(
                "/hello",
                headers={"Content-Type": "application/grpc-web"},
                data=request_body
            )
            response_content = await resp.read()
            await client.close()
            return resp.status, resp.headers.get("Content-Type"), response_content

        status, content_type, body = asyncio.run(run())
        assert status == 200
        assert content_type == "application/grpc-web"

        # Check response body:
        # First frame: 5 bytes header + payload
        assert len(body) > 5
        flag = body[0]
        assert flag == 0
        length = int.from_bytes(body[1:5], byteorder="big")
        payload = body[5:5+length]
        resp_data = json.loads(payload.decode("utf-8"))
        assert resp_data["message"] == "Hello test-grpc-web"

        # Trailers frame: starts at 5 + length
        trailers_part = body[5+length:]
        assert len(trailers_part) > 5
        assert trailers_part[0] == 0x80
        t_len = int.from_bytes(trailers_part[1:5], byteorder="big")
        t_str = trailers_part[5:5+t_len].decode("ascii")
        assert "grpc-status: 0" in t_str

    def test_grpc_web_text_success(self):
        from helix_rt.server import HelixServer
        import json
        import base64

        async def run():
            server = HelixServer()
            
            async def hello_handler(body):
                return {"message": f"Hello {body.get('name', 'World')}"}

            server.register_route("POST", "/hello", hello_handler)

            from aiohttp.test_utils import TestClient, TestServer
            client = TestClient(TestServer(server.app))
            await client.start_server()

            # Construct gRPC-Web-text payload: base64-encoded framed JSON
            payload_str = json.dumps({"name": "test-grpc-web-text"})
            payload_bytes = payload_str.encode("utf-8")
            header = b"\x00" + len(payload_bytes).to_bytes(4, byteorder="big")
            request_body = base64.b64encode(header + payload_bytes)

            resp = await client.post(
                "/hello",
                headers={"Content-Type": "application/grpc-web-text"},
                data=request_body
            )
            response_content = await resp.read()
            await client.close()
            return resp.status, resp.headers.get("Content-Type"), response_content

        status, content_type, body = asyncio.run(run())
        assert status == 200
        assert content_type == "application/grpc-web-text"

        # Base64 decode response first
        decoded_body = base64.b64decode(body)

        # Check response body:
        assert len(decoded_body) > 5
        flag = decoded_body[0]
        assert flag == 0
        length = int.from_bytes(decoded_body[1:5], byteorder="big")
        payload = decoded_body[5:5+length]
        resp_data = json.loads(payload.decode("utf-8"))
        assert resp_data["message"] == "Hello test-grpc-web-text"

        # Trailers frame: starts at 5 + length
        trailers_part = decoded_body[5+length:]
        assert len(trailers_part) > 5
        assert trailers_part[0] == 0x80
        t_len = int.from_bytes(trailers_part[1:5], byteorder="big")
        t_str = trailers_part[5:5+t_len].decode("ascii")
        assert "grpc-status: 0" in t_str


