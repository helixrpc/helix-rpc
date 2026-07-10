package io.helixrpc.runtime;

import redis.clients.jedis.Jedis;
import redis.clients.jedis.JedisPool;

public class RedisRateLimiter {
    private final JedisPool jedisPool;
    private final double requestsPerSecond;
    private final int burst;
    private final String scriptSha;

    private static final String LUA_SCRIPT = 
        "local key = KEYS[1] " +
        "local rate = tonumber(ARGV[1]) " +
        "local burst = tonumber(ARGV[2]) " +
        "local now = tonumber(ARGV[3]) " +
        "local tokens_key = key .. ':tokens' " +
        "local timestamp_key = key .. ':ts' " +
        "local last_tokens = tonumber(redis.call('get', tokens_key)) " +
        "if last_tokens == nil then last_tokens = burst end " +
        "local last_refreshed = tonumber(redis.call('get', timestamp_key)) " +
        "if last_refreshed == nil then last_refreshed = 0 end " +
        "local delta = math.max(0, now - last_refreshed) " +
        "local filled_tokens = math.min(burst, last_tokens + (delta * rate)) " +
        "local allowed = filled_tokens >= 1 " +
        "local new_tokens = filled_tokens " +
        "if allowed then new_tokens = filled_tokens - 1 end " +
        "local ttl = math.ceil(burst / rate) " +
        "redis.call('setex', tokens_key, ttl, new_tokens) " +
        "redis.call('setex', timestamp_key, ttl, now) " +
        "if allowed then return 1 else return 0 end";

    public RedisRateLimiter(String redisUri, double requestsPerSecond, int burst) {
        this.jedisPool = new JedisPool(redisUri);
        this.requestsPerSecond = requestsPerSecond;
        this.burst = burst;
        try (Jedis jedis = jedisPool.getResource()) {
            this.scriptSha = jedis.scriptLoad(LUA_SCRIPT);
        }
    }

    public boolean allow(String key) {
        long now = System.currentTimeMillis() / 1000;
        try (Jedis jedis = jedisPool.getResource()) {
            Object result = jedis.evalsha(scriptSha, 1, "ratelimit:" + key, 
                String.valueOf(requestsPerSecond), 
                String.valueOf(burst), 
                String.valueOf(now));
            return ((Long) result) == 1L;
        }
    }
}
