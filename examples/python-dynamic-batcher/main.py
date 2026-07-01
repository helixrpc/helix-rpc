import sys
import os
import asyncio

# Add the runtime-python directory to path so we can import helix_rt
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), '../../runtime-python')))

from helix_rt.server import HelixServer
from helix_rt.batching import BatchScheduler

async def process_batch_on_gpu(requests):
    """
    Simulates a GPU processing a batch of prompts efficiently.
    """
    print(f"\n[MockAI] 🚀 Executing batch of {len(requests)} prompts simultaneously on virtual GPU...")
    responses = []
    for i, req in enumerate(requests):
        prompt = req.get("prompt", "")
        print(f"  -> [Batch Index {i}] Processing prompt: \"{prompt}\"")
        responses.append({"completion": f"Python AI Response to: {prompt}"})
        
    await asyncio.sleep(0.1) # Simulate inference latency
    print("[MockAI] ✅ Batch execution complete!")
    return responses

if __name__ == "__main__":
    # Create the Batch Scheduler (Max 100 requests, 50ms window)
    scheduler = BatchScheduler(100, 50, process_batch_on_gpu)

    server = HelixServer("127.0.0.1", 8083)

    # Wrap the handler to invoke the scheduler
    async def predict_handler(body: dict):
        return await scheduler.invoke(body)

    # A simple streaming handler simulating token-by-token generation
    async def stream_handler(body: dict):
        prompt = body.get("prompt", "")
        async def token_generator():
            for i in range(1, 6):
                await asyncio.sleep(0.1) # Simulate generation
                yield {"chunk": f"Token {i} for {prompt}"}
        return token_generator()

    server.register_route("POST", "/v1/models/predict", predict_handler)
    server.register_route("POST", "/v1/models/stream", stream_handler)
    server.start()
