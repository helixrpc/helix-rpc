import asyncio
import logging

try:
    import hvac
except ImportError:
    hvac = None

logger = logging.getLogger(__name__)

class HelixVault:
    def __init__(self, url: str, token: str, secret_path: str, refresh_interval: int = 300):
        if hvac is None:
            raise ImportError("hvac is required for HelixVault")
        self.url = url
        self.token = token
        self.secret_path = secret_path
        self.refresh_interval = refresh_interval
        self.client = hvac.Client(url=self.url, token=self.token)
        self.current_key = None
        self._task = None

    async def start(self):
        await self.refresh_key()
        self._task = asyncio.create_task(self._refresh_loop())

    async def stop(self):
        if self._task:
            self._task.cancel()
            try:
                await self._task
            except asyncio.CancelledError:
                pass

    async def _refresh_loop(self):
        while True:
            await asyncio.sleep(self.refresh_interval)
            try:
                await self.refresh_key()
            except Exception as e:
                logger.error(f"Failed to refresh vault key: {e}")

    async def refresh_key(self):
        loop = asyncio.get_event_loop()
        def _read():
            return self.client.secrets.kv.v2.read_secret_version(path=self.secret_path)
        
        response = await loop.run_in_executor(None, _read)
        self.current_key = response['data']['data'].get('key')
        
    def get_key(self) -> str:
        return self.current_key
