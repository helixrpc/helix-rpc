import json
import logging
from aiohttp import web
from dataclasses import asdict

class HelixServer:
    """
    A lightweight, high-performance HTTP server wrapper leveraging aiohttp.
    Designed to serve Helix RPC generated Abstract Base Classes natively.
    """
    def __init__(self, host: str = "127.0.0.1", port: int = 8080):
        self.host = host
        self.port = port
        self.app = web.Application()

    def register_route(self, method: str, path: str, handler):
        async def web_handler(request: web.Request):
            try:
                body = await request.json()
                
                # We expect the handler to take kwargs mapped from the JSON payload.
                # E.g. handler(request_dataclass(**body))
                resp = await handler(body)
                
                # If the response is a dataclass, serialize it cleanly to JSON
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
