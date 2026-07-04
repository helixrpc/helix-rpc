import asyncio
import logging

class DatabaseConnectionPool:
    """
    Generic async database connection pool wrapper (Postgres/MySQL/Redis)
    to enforce safe timeout bounds during startup connection checks.
    """
    def __init__(self, connect_coro, min_size=1, max_size=10, timeout=5.0):
        self.connect_coro = connect_coro
        self.min_size = min_size
        self.max_size = max_size
        self.timeout = timeout
        self.pool = None

    async def init(self):
        try:
            self.pool = await asyncio.wait_for(self.connect_coro(), timeout=self.timeout)
            logging.info("🧬 [Helix] Shared Database Connection Pool initialized successfully.")
            return self.pool
        except Exception as e:
            logging.error(f"✗ [Helix] Database Connection Pool initialization failed: {e}")
            raise e
