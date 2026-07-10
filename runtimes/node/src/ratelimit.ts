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

import { Redis } from 'ioredis';

const LUA_TOKEN_BUCKET = `
local key = KEYS[1]
local rate = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local now = tonumber(ARGV[3])

local tokens_key = key .. ":tokens"
local timestamp_key = key .. ":ts"

local last_tokens = tonumber(redis.call("get", tokens_key))
if last_tokens == nil or last_tokens == false then
    last_tokens = burst
end

local last_refreshed = tonumber(redis.call("get", timestamp_key))
if last_refreshed == nil or last_refreshed == false then
    last_refreshed = 0
end

local delta = math.max(0, now - last_refreshed)
local filled_tokens = math.min(burst, last_tokens + (delta * rate))
local allowed = filled_tokens >= 1
local new_tokens = filled_tokens

if allowed then
    new_tokens = filled_tokens - 1
end

redis.call("setex", tokens_key, math.ceil(burst / rate), new_tokens)
redis.call("setex", timestamp_key, math.ceil(burst / rate), now)

if allowed then
    return { new_tokens, 1 }
else
    return { new_tokens, 0 }
end
`;

export class RedisRateLimiter {
    private redis: Redis;
    private rps: number;
    private burst: number;

    constructor(redis: Redis, rps: number, burst?: number) {
        this.redis = redis;
        this.rps = rps;
        this.burst = burst ?? Math.max(1, Math.floor(rps));
        
        // Register the script
        this.redis.defineCommand('allowRateLimit', {
            numberOfKeys: 1,
            lua: LUA_TOKEN_BUCKET,
        });
    }

    public async allow(key: string): Promise<[number, boolean]> {
        const now = Date.now() / 1000.0;
        // @ts-ignore - custom command
        const res = await this.redis.allowRateLimit(`ratelimit:${key}`, this.rps, this.burst, now);
        return [Number(res[0]), Boolean(res[1])];
    }
}
