use redis::Client;
use std::collections::HashMap;
use std::sync::atomic::{AtomicI64, AtomicU64, Ordering};
use std::sync::{Arc, RwLock};
use std::time::{SystemTime, UNIX_EPOCH};

const NANO_TOKEN: i64 = 1_000_000_000;

#[derive(Debug)]
pub struct ClientBucket {
    tokens: AtomicI64,    // stored as nano-tokens
    last_seen: AtomicU64, // Unix nanoseconds
    capacity: i64,        // nano-tokens
    refill_ns: i64,       // nano-tokens/ns (= tokens/s at nano scale)
}

impl ClientBucket {
    pub fn new(rps: f64, burst: i64) -> Self {
        let cap = (burst as f64 * NANO_TOKEN as f64) as i64;
        let now_ns = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_nanos() as u64;

        Self {
            tokens: AtomicI64::new(cap),
            last_seen: AtomicU64::new(now_ns),
            capacity: cap,
            refill_ns: rps as i64,
        }
    }

    pub fn allow(&self) -> (i64, bool) {
        let now_ns = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_nanos() as u64;

        let last = self.last_seen.swap(now_ns, Ordering::SeqCst);
        let elapsed = now_ns.saturating_sub(last);
        if elapsed > 0 {
            let refill = (elapsed as i64).saturating_mul(self.refill_ns);
            loop {
                let cur = self.tokens.load(Ordering::SeqCst);
                let next = (cur + refill).min(self.capacity);
                if self
                    .tokens
                    .compare_exchange(cur, next, Ordering::SeqCst, Ordering::SeqCst)
                    .is_ok()
                {
                    break;
                }
            }
        }

        loop {
            let cur = self.tokens.load(Ordering::SeqCst);
            if cur < NANO_TOKEN {
                return (cur / NANO_TOKEN, false);
            }
            if self
                .tokens
                .compare_exchange(cur, cur - NANO_TOKEN, Ordering::SeqCst, Ordering::SeqCst)
                .is_ok()
            {
                return ((cur - NANO_TOKEN) / NANO_TOKEN, true);
            }
        }
    }
}

pub struct RateLimiter {
    rps: f64,
    burst: i64,
    buckets: RwLock<HashMap<String, Arc<ClientBucket>>>,
}

impl RateLimiter {
    pub fn new(requests_per_second: f64, burst_size: Option<i64>) -> Self {
        let burst = burst_size.unwrap_or_else(|| requests_per_second.max(1.0) as i64);
        Self {
            rps: requests_per_second,
            burst,
            buckets: RwLock::new(HashMap::new()),
        }
    }

    pub fn allow(&self, key: &str) -> (i64, bool) {
        {
            let read = self.buckets.read().unwrap();
            if let Some(bucket) = read.get(key) {
                return bucket.allow();
            }
        }
        let mut write = self.buckets.write().unwrap();
        let bucket = write
            .entry(key.to_string())
            .or_insert_with(|| Arc::new(ClientBucket::new(self.rps, self.burst)))
            .clone();
        bucket.allow()
    }

    pub fn limit(&self) -> i64 {
        self.burst
    }

    pub fn rps(&self) -> f64 {
        self.rps
    }
}

const LUA_TOKEN_BUCKET: &str = r#"
local key = KEYS[1]
local rate = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local now = tonumber(ARGV[3])

local tokens_key = key .. ":tokens"
local timestamp_key = key .. ":ts"

local last_tokens = tonumber(redis.call("get", tokens_key))
if last_tokens == nil then
    last_tokens = burst
end

local last_refreshed = tonumber(redis.call("get", timestamp_key))
if last_refreshed == nil then
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

return { new_tokens, allowed }
"#;

pub struct RedisRateLimiter {
    client: Client,
    rps: f64,
    burst: i64,
}

impl RedisRateLimiter {
    pub fn new(client: Client, rps: f64, burst_size: Option<i64>) -> Self {
        let burst = burst_size.unwrap_or_else(|| rps.max(1.0) as i64);
        Self { client, rps, burst }
    }

    pub async fn allow(&self, key: &str) -> redis::RedisResult<(i64, bool)> {
        let mut con = self.client.get_async_connection().await?;
        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs_f64();

        let result: Vec<i64> = redis::Script::new(LUA_TOKEN_BUCKET)
            .key(format!("ratelimit:{}", key))
            .arg(self.rps)
            .arg(self.burst)
            .arg(now)
            .invoke_async(&mut con)
            .await?;

        if result.len() == 2 {
            Ok((result[0], result[1] == 1))
        } else {
            Err(redis::RedisError::from((
                redis::ErrorKind::TypeError,
                "Invalid response from Lua script",
            )))
        }
    }
}

