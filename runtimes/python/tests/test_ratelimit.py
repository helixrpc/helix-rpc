import pytest
from unittest.mock import AsyncMock, MagicMock
from helix_rt.ratelimit import _ClientBucket, RedisRateLimiter

@pytest.mark.asyncio
async def test_client_bucket():
    bucket = _ClientBucket(rate=1.0, burst=2)
    # consume first
    tokens, allowed = await bucket.consume()
    assert allowed is True
    # consume second
    tokens, allowed = await bucket.consume()
    assert allowed is True
    # consume third should fail
    tokens, allowed = await bucket.consume()
    assert allowed is False

@pytest.mark.asyncio
async def test_redis_rate_limiter():
    redis_mock = MagicMock()
    redis_mock.register_script = MagicMock(return_value=AsyncMock(return_value=[1, 1]))
    
    limiter = RedisRateLimiter(redis_mock, requests_per_second=10.0, burst=5)
    
    tokens, allowed = await limiter.allow("client_ip")
    assert allowed is True
    assert tokens == 1
    limiter._script.assert_called_once()
