import hashlib
import asyncio
from typing import Optional, Tuple
import aiomcache

class MemcachedCache:
    """
    Zero-Serialization Cache for Unary RPC Responses backed by Memcached.
    """

    def __init__(self, servers: list[str], ttl_seconds: int = 60) -> None:
        self.ttl = ttl_seconds
        # aiomcache expects host, port tuples or just string if single
        if len(servers) > 0:
            host_port = servers[0].split(":")
            host = host_port[0]
            port = int(host_port[1]) if len(host_port) > 1 else 11211
            self.client = aiomcache.Client(host, port)
        else:
            self.client = aiomcache.Client("localhost", 11211)

    def generate_cache_key(self, method: str, payload: bytes) -> bytes:
        """
        Computes a SHA256 hash of the method name and the request payload.
        Returns bytes as memcached keys are typically bytes.
        """
        h = hashlib.sha256()
        h.update(method.encode('utf-8'))
        h.update(payload)
        return h.hexdigest().encode('utf-8')

    async def get(self, key: bytes) -> tuple[Optional[bytes], bool]:
        """
        Attempts to retrieve a cached response.
        Returns (payload, True) if cache hit, (None, False) if cache miss.
        """
        try:
            val = await self.client.get(key)
            if val is not None:
                return val, True
        except Exception:
            pass
        return None, False

    async def set(self, key: bytes, payload: bytes) -> None:
        """
        Asynchronously caches the response payload.
        """
        try:
            await self.client.set(key, payload, exptime=self.ttl)
        except Exception:
            pass
