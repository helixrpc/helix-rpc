import asyncio
import json
import os
from aiohttp import web
from model import LLMModel

model = LLMModel()
UI_DIR = os.path.join(os.path.dirname(__file__), "ui")

async def handle_index(request):
    return web.FileResponse(os.path.join(UI_DIR, "index.html"))

async def handle_static(request):
    filename = request.match_info["filename"]
    filepath = os.path.join(UI_DIR, filename)
    if os.path.exists(filepath):
        return web.FileResponse(filepath)
    return web.Response(status=404)

async def handle_chat(request):
    body = await request.json()
    messages = body.get("messages", [])
    prompt = messages[-1]["content"] if messages else "Hello"

    response = web.StreamResponse()
    response.headers["Content-Type"] = "text/event-stream"
    response.headers["Cache-Control"] = "no-cache"
    response.headers["Access-Control-Allow-Origin"] = "*"
    await response.prepare(request)

    # Run the blocking generator in a thread pool
    loop = asyncio.get_event_loop()
    import concurrent.futures
    executor = concurrent.futures.ThreadPoolExecutor(max_workers=1)

    def get_token_generator():
        return model.generate_stream(prompt)

    generator = await loop.run_in_executor(executor, get_token_generator)
    
    while True:
        try:
            token = await loop.run_in_executor(executor, lambda: next(generator, None))
            if token is None:
                break
            
            # OpenAI-compatible SSE format
            chunk = {
                "choices": [{"delta": {"content": token}, "index": 0}]
            }
            await response.write(f"data: {json.dumps(chunk)}\n\n".encode())
            await asyncio.sleep(0)  # yield control
        except Exception as e:
            print(f"Error streaming token: {e}")
            break

    await response.write(b"data: [DONE]\n\n")
    await response.write_eof()
    return response

async def handle_options(request):
    return web.Response(headers={
        "Access-Control-Allow-Origin": "*",
        "Access-Control-Allow-Methods": "POST, GET, OPTIONS",
        "Access-Control-Allow-Headers": "Content-Type",
    })

def main():
    app = web.Application()
    app.router.add_get("/", handle_index)
    app.router.add_get("/{filename}", handle_static)
    app.router.add_post("/v1/chat/completions", handle_chat)
    app.router.add_options("/v1/chat/completions", handle_options)
    print("Helix LLM Streaming Server on http://127.0.0.1:8081")
    web.run_app(app, host="127.0.0.1", port=8081)

if __name__ == "__main__":
    main()
