import json
import logging
import asyncio
import gzip
from typing import AsyncIterator, Callable, Awaitable
from aiohttp import web
from dataclasses import asdict

@web.middleware
async def deadline_middleware(request: web.Request, handler: Callable[[web.Request], Awaitable[web.StreamResponse]]):
    """
    Parses grpc-timeout header and enforces a hard execution deadline via asyncio.wait_for
    """
    timeout_header = request.headers.get("grpc-timeout")
    if timeout_header and timeout_header.endswith("m"):
        try:
            ms = int(timeout_header[:-1])
            return await asyncio.wait_for(handler(request), timeout=ms / 1000.0)
        except asyncio.TimeoutError:
            return web.Response(status=408, text="Deadline Exceeded")
    return await handler(request)

@web.middleware
async def gzip_middleware(request: web.Request, handler: Callable[[web.Request], Awaitable[web.StreamResponse]]):
    """
    Compresses standard JSON responses if the client requested grpc-encoding: gzip
    """
    response = await handler(request)
    
    accept_encoding = request.headers.get("grpc-encoding", "")
    if "gzip" in accept_encoding and isinstance(response, web.Response) and response.content_type != 'text/event-stream':
        body = response.body
        if body:
            compressed = gzip.compress(body)
            response.body = compressed
            response.headers["grpc-encoding"] = "gzip"
            response.headers["Content-Length"] = str(len(compressed))
    return response

class HelixServer:
    """
    A lightweight, high-performance HTTP server wrapper leveraging aiohttp.
    Designed to serve Helix RPC generated Abstract Base Classes natively.
    """
    def __init__(self, host: str = "127.0.0.1", port: int = 8080):
        self.host = host
        self.port = port
        # Built-in middlewares for Production Parity
        self.app = web.Application(middlewares=[deadline_middleware, gzip_middleware])

    def add_middleware(self, mw):
        """Register a custom aiohttp middleware"""
        self.app.middlewares.append(mw)

    def register_route(self, method: str, path: str, handler):
        async def web_handler(request: web.Request):
            try:
                # 1. Parse JSON Request
                body = await request.json()
                
                # 2. Execute Handler
                resp = await handler(body)
                
                # 3. Detect Streaming (AsyncIterator) and Transcode to SSE
                if isinstance(resp, AsyncIterator) or hasattr(resp, "__aiter__"):
                    response = web.StreamResponse(
                        status=200,
                        reason='OK',
                        headers={
                            'Content-Type': 'text/event-stream',
                            'Cache-Control': 'no-cache',
                            'Connection': 'keep-alive',
                        }
                    )
                    await response.prepare(request)
                    
                    async for chunk in resp:
                        if hasattr(chunk, "__dataclass_fields__"):
                            chunk_data = asdict(chunk)
                        else:
                            chunk_data = chunk
                        msg = f"data: {json.dumps(chunk_data)}\n\n"
                        await response.write(msg.encode('utf-8'))
                    
                    return response
                
                # 4. Standard Response
                if hasattr(resp, "__dataclass_fields__"):
                    resp = asdict(resp)
                    
                return web.json_response(resp)
            except Exception as e:
                logging.exception("Helix RPC Handler Exception")
                return web.json_response({"error": str(e)}, status=500)
                
        self.app.router.add_route(method, path, web_handler)

    def start(self):
        print(f"🚀 Starting Python Helix Gateway on http://{self.host}:{self.port}")
        web.run_app(self.app, host=self.host, port=self.port, print=None)
