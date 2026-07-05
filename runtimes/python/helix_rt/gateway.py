import time


class TenantLimiter:
    def __init__(self, capacity: float, rate: float):
        self.capacity = capacity
        self.rate = rate
        self.tokens = capacity
        self.last_refill = time.time()

    def consume(self, count: float) -> bool:
        now = time.time()
        elapsed = now - self.last_refill
        self.tokens = min(self.capacity, self.tokens + elapsed * self.rate)
        self.last_refill = now

        if self.tokens >= count:
            self.tokens -= count
            return True
        return False


class MultiTenantRateLimiter:
    def __init__(self, default_capacity: float, default_rate: float):
        self.default_capacity = default_capacity
        self.default_rate = default_rate
        self.limiters = {}

    def set_tenant_limit(self, tenant_id: str, capacity: float, rate: float) -> None:
        self.limiters[tenant_id] = TenantLimiter(capacity, rate)

    def allow(self, tenant_id: str, count: float) -> bool:
        limiter = self.limiters.get(tenant_id)
        if not limiter:
            limiter = TenantLimiter(self.default_capacity, self.default_rate)
            self.limiters[tenant_id] = limiter
        return limiter.consume(count)

    def allow_jwt_request(self, token: str, secret: str, count: float) -> bool:
        try:
            import jwt
            payload = jwt.decode(token, secret, algorithms=["HS256"])
            tenant_id = payload.get("tenant_id")
            if not tenant_id:
                return False
            return self.allow(tenant_id, count)
        except Exception:
            return False
