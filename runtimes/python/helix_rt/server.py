import json
import logging
import asyncio
import signal
import gzip
from typing import AsyncIterator, Callable, Awaitable
from aiohttp import web
from dataclasses import asdict

from .errors import HelixError, ErrorCode
from .telemetry import telemetry_middleware

# ---------------------------------------------------------------------------
# Built-in Middlewares
# ---------------------------------------------------------------------------

@web.middleware
async def deadline_middleware(request: web.Request, handler: Callable[[web.Request], Awaitable[web.StreamResponse]]):
    """
    Parses grpc-timeout header and enforces a hard execution deadline.
    Supports units: n (ns), u (µs), m (ms), S (s), M (min), H (hr).
    """
    timeout_header = request.headers.get("grpc-timeout")
    if timeout_header:
        unit = timeout_header[-1]
        try:
            val = int(timeout_header[:-1])
            unit_map = {"n": 1e-9, "u": 1e-6, "m": 1e-3, "S": 1.0, "M": 60.0, "H": 3600.0}
            timeout_secs = val * unit_map.get(unit, 1e-3)  # default ms
            return await asyncio.wait_for(handler(request), timeout=timeout_secs)
        except asyncio.TimeoutError:
            return web.Response(status=408, text="Deadline Exceeded")
        except (ValueError, KeyError):
            pass
    return await handler(request)


@web.middleware
async def gzip_middleware(request: web.Request, handler: Callable[[web.Request], Awaitable[web.StreamResponse]]):
    """
    Compresses responses if the client requested grpc-encoding: gzip.
    Skips SSE streams which cannot be post-compressed.
    """
    response = await handler(request)
    accept_encoding = request.headers.get("grpc-encoding", "")
    if "gzip" in accept_encoding and isinstance(response, web.Response) and response.content_type != "text/event-stream":
        body = response.body
        if body:
            compressed = gzip.compress(body)
            response.body = compressed
            response.headers["grpc-encoding"] = "gzip"
            response.headers["Content-Length"] = str(len(compressed))
    return response


@web.middleware
async def logging_middleware(request: web.Request, handler: Callable[[web.Request], Awaitable[web.StreamResponse]]):
    """
    Logs request path, method, latency, status code, and trace IDs in JSON.
    """
    start = asyncio.get_event_loop().time()
    try:
        response = await handler(request)
        status = response.status
        return response
    except Exception as e:
        status = 500
        raise e
    finally:
        latency = (asyncio.get_event_loop().time() - start) * 1000.0
        trace_id = request.headers.get("traceparent", "")
        log_entry = {
            "method": request.method,
            "path": request.path,
            "status": status,
            "duration_ms": round(latency, 2),
            "trace_id": trace_id,
        }
        logging.info(f"RPC Request: {json.dumps(log_entry)}")


# ---------------------------------------------------------------------------
# HelixServer
# ---------------------------------------------------------------------------

class HelixServer:
    """
    A lightweight, high-performance HTTP server wrapping aiohttp.
    Designed to serve Helix RPC generated service implementations.

    Built-in middlewares (in order):
      1. telemetry_middleware  — probabilistic OpenTelemetry tracing (1%)
      2. deadline_middleware   — grpc-timeout deadline enforcement
      3. gzip_middleware       — response compression

    Supports graceful shutdown via SIGTERM/SIGINT with configurable drain
    time, matching the Go and Rust gateway behaviour.
    """

    def __init__(
        self,
        host: str = "127.0.0.1",
        port: int = 8080,
        disable_metrics: bool = False,
        disable_health: bool = False,
        disable_gzip: bool = False,
        disable_deadline: bool = False,
    ):
        self.host = host
        self.port = port
        
        middlewares = [telemetry_middleware, logging_middleware]
        if not disable_deadline:
            middlewares.append(deadline_middleware)
        if not disable_gzip:
            middlewares.append(gzip_middleware)

        self.app = web.Application(middlewares=middlewares)
        self.disable_metrics = disable_metrics
        self.disable_health = disable_health
        self._runner: web.AppRunner | None = None
        self._site:   web.TCPSite | None   = None

        if not disable_health:
            async def health_check_handler(body):
                return {"status": 1}
            self.register_route("POST", "/grpc.health.v1.Health/Check", health_check_handler)
            self.register_route("GET", "/grpc.health.v1.Health/Check", health_check_handler)

    # ------------------------------------------------------------------
    # Middleware registration
    # ------------------------------------------------------------------

    def add_middleware(self, mw) -> None:
        """Register a custom aiohttp middleware. Must be called before start()."""
        self.app.middlewares.append(mw)

    # ------------------------------------------------------------------
    # Route registration
    # ------------------------------------------------------------------

    def register_route(self, method: str, path: str, handler) -> None:
        """
        Register an RPC handler function at the given HTTP method + path.

        The handler receives a parsed dict (from JSON body) and may return:
          - a dict / dataclass  → serialised as JSON
          - an AsyncIterator    → streamed as Server-Sent Events (SSE)

        HelixError is automatically converted to the appropriate HTTP status.
        """
        async def web_handler(request: web.Request) -> web.StreamResponse:
            ct = request.content_type or ""
            is_grpc_web = ct.startswith("application/grpc-web")
            is_grpc_web_text = ct.startswith("application/grpc-web-text")

            try:
                if is_grpc_web or is_grpc_web_text:
                    raw_body = await request.read()
                    if is_grpc_web_text:
                        import base64
                        try:
                            raw_body = base64.b64decode(raw_body)
                        except Exception as decode_err:
                            raise HelixError(ErrorCode.INVALID_ARGUMENT, f"Failed to decode base64 body: {decode_err}")

                    if len(raw_body) < 5:
                        raise HelixError(ErrorCode.INVALID_ARGUMENT, "grpc frame too small")

                    compressed_flag = raw_body[0]
                    length = int.from_bytes(raw_body[1:5], byteorder="big")
                    if len(raw_body) < 5 + length:
                        raise HelixError(ErrorCode.INVALID_ARGUMENT, "grpc frame payload truncated")

                    payload = raw_body[5:5+length]
                    if compressed_flag == 1:
                        payload = gzip.decompress(payload)

                    payload_str = payload.decode("utf-8") if payload else ""
                    body = json.loads(payload_str) if payload_str else {}
                else:
                    body = await request.json()

                resp = await handler(body)

                if is_grpc_web or is_grpc_web_text:
                    if hasattr(resp, "marshal_flatbuffers") and callable(resp.marshal_flatbuffers):
                        resp_bytes = resp.marshal_flatbuffers()
                    elif hasattr(resp, "__dataclass_fields__"):
                        resp_bytes = json.dumps(asdict(resp)).encode("utf-8")
                    else:
                        resp_bytes = json.dumps(resp).encode("utf-8")

                    # Write the response body frame (5-byte header + binary Protobuf/data)
                    response_header = b"\x00" + len(resp_bytes).to_bytes(4, byteorder="big")
                    response_frame = response_header + resp_bytes

                    # Append the trailers frame
                    trailers_str = "grpc-status: 0\r\ngrpc-message: \r\n"
                    trailers_bytes = trailers_str.encode("ascii")
                    trailers_len = len(trailers_bytes)
                    trailers_header = b"\x80" + trailers_len.to_bytes(4, byteorder="big")
                    trailers_frame = trailers_header + trailers_bytes

                    combined_frame = response_frame + trailers_frame
                    if is_grpc_web_text:
                        import base64
                        response_body = base64.b64encode(combined_frame)
                    else:
                        response_body = combined_frame

                    return web.Response(
                        body=response_body,
                        content_type=ct,
                        status=200
                    )

                # SSE streaming
                if isinstance(resp, AsyncIterator) or hasattr(resp, "__aiter__"):
                    response = web.StreamResponse(
                        status=200,
                        reason="OK",
                        headers={
                            "Content-Type": "text/event-stream",
                            "Cache-Control": "no-cache",
                            "Connection": "keep-alive",
                        },
                    )
                    await response.prepare(request)
                    async for chunk in resp:
                        chunk_data = asdict(chunk) if hasattr(chunk, "__dataclass_fields__") else chunk
                        await response.write(f"data: {json.dumps(chunk_data)}\n\n".encode())
                    return response

                # Standard unary response
                if hasattr(resp, "__dataclass_fields__"):
                    resp = asdict(resp)
                return web.json_response(resp)

            except HelixError as he:
                logging.warning("Helix RPC HelixError: %s", he)
                if is_grpc_web or is_grpc_web_text:
                    trailers_str = f"grpc-status: {he.grpc_status}\r\ngrpc-message: {he.message}\r\n"
                    trailers_bytes = trailers_str.encode("ascii")
                    trailers_len = len(trailers_bytes)
                    trailers_header = b"\x80" + trailers_len.to_bytes(4, byteorder="big")
                    trailers_frame = trailers_header + trailers_bytes

                    if is_grpc_web_text:
                        import base64
                        response_body = base64.b64encode(trailers_frame)
                    else:
                        response_body = trailers_frame

                    return web.Response(
                        body=response_body,
                        content_type=ct,
                        status=200
                    )
                else:
                    return web.json_response(
                        {"error": he.message, "code": he.grpc_status},
                        status=he.http_status,
                    )
            except Exception as e:
                logging.exception("Helix RPC Handler Exception")
                if is_grpc_web or is_grpc_web_text:
                    trailers_str = f"grpc-status: 13\r\ngrpc-message: {str(e)}\r\n"
                    trailers_bytes = trailers_str.encode("ascii")
                    trailers_len = len(trailers_bytes)
                    trailers_header = b"\x80" + trailers_len.to_bytes(4, byteorder="big")
                    trailers_frame = trailers_header + trailers_bytes

                    if is_grpc_web_text:
                        import base64
                        response_body = base64.b64encode(trailers_frame)
                    else:
                        response_body = trailers_frame

                    return web.Response(
                        body=response_body,
                        content_type=ct,
                        status=200
                    )
                else:
                    return web.json_response({"error": str(e)}, status=500)

        self.app.router.add_route(method, path, web_handler)

    # ------------------------------------------------------------------
    # Lifecycle
    # ------------------------------------------------------------------

    def start(self) -> None:
        """Start the server synchronously (blocking). Handles SIGTERM/SIGINT gracefully."""
        asyncio.run(self._run_async())

    async def start_async(self) -> None:
        """Start the server inside an existing event loop (non-blocking)."""
        self._runner = web.AppRunner(self.app)
        await self._runner.setup()
        self._site = web.TCPSite(self._runner, self.host, self.port)
        await self._site.start()
        print(f"🚀 Helix Python Gateway listening on http://{self.host}:{self.port}")

    async def stop_async(self, drain_seconds: float = 5.0) -> None:
        """
        Graceful shutdown: stop accepting connections, wait `drain_seconds`
        for in-flight requests to complete, then tear down the runner.
        """
        if self._site:
            await self._site.stop()
        if drain_seconds > 0:
            await asyncio.sleep(drain_seconds)
        if self._runner:
            await self._runner.cleanup()
        print("✅ Helix Python Gateway shut down cleanly.")

    async def _run_async(self) -> None:
        loop = asyncio.get_running_loop()
        stop_event = asyncio.Event()

        def _handle_signal():
            logging.info("Shutdown signal received; draining…")
            stop_event.set()

        for sig in (signal.SIGTERM, signal.SIGINT):
            try:
                loop.add_signal_handler(sig, _handle_signal)
            except (NotImplementedError, RuntimeError):
                pass  # Windows / non-main threads

        await self.start_async()
        await stop_event.wait()
        await self.stop_async()
