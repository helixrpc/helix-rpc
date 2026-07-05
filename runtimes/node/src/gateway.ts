class TenantLimiter {
    private tokens: number;
    private lastRefill: number;

    constructor(private readonly capacity: number, private readonly rate: number) {
        this.tokens = capacity;
        this.lastRefill = Date.now();
    }

    public consume(count: number): boolean {
        const now = Date.now();
        const elapsed = (now - this.lastRefill) / 1000.0;
        this.tokens = Math.min(this.capacity, this.tokens + elapsed * this.rate);
        this.lastRefill = now;

        if (this.tokens >= count) {
            this.tokens -= count;
            return true;
        }
        return false;
    }
}

export class MultiTenantRateLimiter {
    private readonly limiters: Map<string, TenantLimiter> = new Map();

    constructor(private readonly defaultCapacity: number, private readonly defaultRate: number) {}

    public setTenantLimit(tenantId: string, capacity: number, rate: number): void {
        this.limiters.set(tenantId, new TenantLimiter(capacity, rate));
    }

    public allow(tenantId: string, count: number): boolean {
        let limiter = this.limiters.get(tenantId);
        if (!limiter) {
            limiter = new TenantLimiter(this.defaultCapacity, this.defaultRate);
            this.limiters.set(tenantId, limiter);
        }
        return limiter.consume(count);
    }
}
