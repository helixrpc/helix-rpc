use std::collections::HashMap;
use std::sync::Mutex;
use std::time::Instant;

pub struct TenantLimiter {
    capacity: f64,
    rate: f64,
    tokens: f64,
    last_refill: Instant,
}

impl TenantLimiter {
    pub fn new(capacity: f64, rate: f64) -> Self {
        Self {
            capacity,
            rate,
            tokens: capacity,
            last_refill: Instant::now(),
        }
    }

    pub fn consume(&mut self, count: f64) -> bool {
        let now = Instant::now();
        let elapsed = now.duration_since(self.last_refill).as_secs_f64();
        self.tokens = (self.tokens + elapsed * self.rate).min(self.capacity);
        self.last_refill = now;

        if self.tokens >= count {
            self.tokens -= count;
            true
        } else {
            false
        }
    }
}

pub struct MultiTenantRateLimiter {
    limiters: Mutex<HashMap<String, TenantLimiter>>,
    default_capacity: f64,
    default_rate: f64,
}

impl MultiTenantRateLimiter {
    pub fn new(default_capacity: f64, default_rate: f64) -> Self {
        Self {
            limiters: Mutex::new(HashMap::new()),
            default_capacity,
            default_rate,
        }
    }

    pub fn set_tenant_limit(&self, tenant_id: &str, capacity: f64, rate: f64) {
        let mut guard = self.limiters.lock().unwrap();
        guard.insert(tenant_id.to_string(), TenantLimiter::new(capacity, rate));
    }

    pub fn allow(&self, tenant_id: &str, count: f64) -> bool {
        let mut guard = self.limiters.lock().unwrap();
        let limiter = guard.entry(tenant_id.to_string()).or_insert_with(|| {
            TenantLimiter::new(self.default_capacity, self.default_rate)
        });
        limiter.consume(count)
    }
}
