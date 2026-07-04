export class TokenBucketRateLimiter {
    private capacity: number;
    private refillRate: number; // tokens per millisecond
    private tokens: number;
    private lastRefill: number;

    constructor(capacity: number, refillRatePerSec: number) {
        this.capacity = capacity;
        this.refillRate = refillRatePerSec / 1000;
        this.tokens = capacity;
        this.lastRefill = Date.now();
    }

    public allow(): boolean {
        const now = Date.now();
        const elapsed = now - this.lastRefill;
        this.lastRefill = now;

        this.tokens = Math.min(this.capacity, this.tokens + elapsed * this.refillRate);

        if (this.tokens >= 1) {
            this.tokens -= 1;
            return true;
        }
        return false;
    }
}
