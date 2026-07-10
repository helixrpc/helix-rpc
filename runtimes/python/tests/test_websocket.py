import asyncio
import json
from aiohttp import web
from aiohttp.test_utils import TestClient, TestServer
from dataclasses import dataclass, asdict
from typing import AsyncIterator

@dataclass
class EchoMessage:
    text: str

async def mock_stream_handler(body: dict) -> AsyncIterator[EchoMessage]:
    yield EchoMessage(text=f"echo: {body.get('text', '')}")

async def handler(request: web.Request):
    if request.headers.get("Upgrade", "").lower() == "websocket":
        ws = web.WebSocketResponse()
        await ws.prepare(request)
        msg = await ws.receive_json()
        resp = mock_stream_handler(msg)
        async for chunk in resp:
            chunk_data = asdict(chunk)
            await ws.send_json(chunk_data)
        return ws
    return web.Response(text="OK")

async def run_test():
    app = web.Application()
    app.router.add_route("GET", "/v1.TestService/StreamEcho", handler)
    
    server = TestServer(app)
    client = TestClient(server)
    await client.start_server()

    try:
        ws = await client.ws_connect("/v1.TestService/StreamEcho")
        await ws.send_json({"text": "hello"})
        resp = await ws.receive_json()
        assert resp["text"] == "echo: hello", f"Expected echo: hello, got {resp['text']}"
        await ws.close()
        print("WebSocket test passed!")
    finally:
        await client.close()

if __name__ == "__main__":
    asyncio.run(run_test())
