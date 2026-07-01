import asyncio
from typing import List, Any, Callable, Awaitable, Optional

class BatchRequest:
    def __init__(self, req: Any):
        self.req = req
        self.future = asyncio.get_event_loop().create_future()

class BatchScheduler:
    """
    Helix RPC Dynamic Batch Scheduler for Python asyncio.
    Absorbs concurrent requests into a unified List, dispatching them either
    when max_size is hit or the batch_window_ms timer expires.
    """
    def __init__(self, max_size: int, batch_window_ms: float, handler: Callable[[List[Any]], Awaitable[List[Any]]]):
        self.max_size = max_size
        self.batch_window_ms = batch_window_ms / 1000.0
        self.handler = handler
        
        self.queue: List[BatchRequest] = []
        self.lock = asyncio.Lock()
        self.timer_task: Optional[asyncio.Task] = None

    async def invoke(self, req: Any) -> Any:
        br = BatchRequest(req)
        
        async with self.lock:
            self.queue.append(br)
            
            if len(self.queue) >= self.max_size:
                # Dispatch immediately
                dispatch_queue = self.queue
                self.queue = []
                if self.timer_task and not self.timer_task.done():
                    self.timer_task.cancel()
                asyncio.create_task(self._process_batch(dispatch_queue))
            elif len(self.queue) == 1:
                # First item, start timer
                loop = asyncio.get_running_loop()
                self.timer_task = loop.create_task(self._wait_and_dispatch())
                
        # Wait for the batch to be processed and return the specific result
        return await br.future
        
    async def _wait_and_dispatch(self):
        try:
            await asyncio.sleep(self.batch_window_ms)
        except asyncio.CancelledError:
            return
            
        async with self.lock:
            if not self.queue:
                return
            dispatch_queue = self.queue
            self.queue = []
            
        await self._process_batch(dispatch_queue)
        
    async def _process_batch(self, batch: List[BatchRequest]):
        reqs = [b.req for b in batch]
        try:
            resps = await self.handler(reqs)
            if len(resps) != len(batch):
                raise ValueError("Batch Handler returned mismatched response list length")
            for br, resp in zip(batch, resps):
                if not br.future.done():
                    br.future.set_result(resp)
        except Exception as e:
            for br in batch:
                if not br.future.done():
                    br.future.set_exception(e)
