import { TokenBucketRateLimiter, RedisRateLimiter } from './ratelimit';

jest.mock('ioredis', () => {
    const mockDefineCommand = jest.fn();
    const MockRedis = jest.fn().mockImplementation(() => ({
        defineCommand: mockDefineCommand,
    }));
    return { Redis: MockRedis, default: MockRedis };
});

import { Redis } from 'ioredis';

describe('TokenBucketRateLimiter', () => {
    it('should allow requests within capacity', () => {
        const limiter = new TokenBucketRateLimiter(5, 10);
        expect(limiter.allow()).toBe(true);
    });

    it('should block requests exceeding capacity', () => {
        const limiter = new TokenBucketRateLimiter(1, 10);
        expect(limiter.allow()).toBe(true);
        expect(limiter.allow()).toBe(false);
    });
});

describe('RedisRateLimiter', () => {
    let redisMock: { defineCommand: jest.Mock; allowRateLimit?: jest.Mock };
    let limiter: RedisRateLimiter;

    beforeEach(() => {
        redisMock = {
            defineCommand: jest.fn(),
        };
        limiter = new RedisRateLimiter(redisMock as unknown as Redis, 10);
    });

    it('should initialize and register lua script', () => {
        expect(redisMock.defineCommand).toHaveBeenCalledWith('allowRateLimit', expect.any(Object));
    });

    it('should allow request based on redis response', async () => {
        redisMock.allowRateLimit = jest.fn().mockResolvedValue([9, 1]);
        const [tokens, allowed] = await limiter.allow('myKey');
        expect(tokens).toBe(9);
        expect(allowed).toBe(true);
        expect(redisMock.allowRateLimit).toHaveBeenCalledWith('ratelimit:myKey', 10, 10, expect.any(Number));
    });

    it('should deny request based on redis response', async () => {
        redisMock.allowRateLimit = jest.fn().mockResolvedValue([0, 0]);
        const [tokens, allowed] = await limiter.allow('myKey');
        expect(tokens).toBe(0);
        expect(allowed).toBe(false);
    });
});
