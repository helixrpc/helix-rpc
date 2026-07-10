import pytest
from unittest.mock import AsyncMock, MagicMock
from helix_rt.cache import MemcachedCache

@pytest.mark.asyncio
async def test_cache_get_hit():
    cache = MemcachedCache(["localhost:11211"])
    cache.client = MagicMock()
    cache.client.get = AsyncMock(return_value=b"cached_payload")
    
    val, hit = await cache.get(b"key")
    assert hit is True
    assert val == b"cached_payload"
    cache.client.get.assert_called_once_with(b"key")

@pytest.mark.asyncio
async def test_cache_get_miss():
    cache = MemcachedCache(["localhost:11211"])
    cache.client = MagicMock()
    cache.client.get = AsyncMock(return_value=None)
    
    val, hit = await cache.get(b"key")
    assert hit is False
    assert val is None

@pytest.mark.asyncio
async def test_cache_set():
    cache = MemcachedCache(["localhost:11211"])
    cache.client = MagicMock()
    cache.client.set = AsyncMock()
    
    await cache.set(b"key", b"payload")
    cache.client.set.assert_called_once_with(b"key", b"payload", exptime=60)
