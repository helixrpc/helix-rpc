use rand::random;
use std::sync::atomic::{AtomicI32, AtomicI64, AtomicU64, Ordering};
use std::sync::Arc;
use std::time::Duration;

// ---------------------------------------------------------------------------
// Circuit Breaker
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Copy, PartialEq)]
#[repr(i32)]
pub enum CircuitState {
    Closed = 0,
    Open = 1,
    HalfOpen = 2,
}

/// Thread-safe atomic circuit breaker matching the Go implementation.
pub struct CircuitBreaker {
    state: AtomicI32,
    failures: AtomicI64,
    successes: AtomicI64,
    last_open_ns: AtomicU64,
    max_failures: i64,
    open_timeout: Duration,
    half_open_probes: i64,
}

impl CircuitBreaker {
    pub fn new(max_failures: i64, open_timeout: Duration, half_open_probes: i64) -> Arc<Self> {
        Arc::new(Self {
            state: AtomicI32::new(CircuitState::Closed as i32),
            failures: AtomicI64::new(0),
            successes: AtomicI64::new(0),
            last_open_ns: AtomicU64::new(0),
            max_failures,
            open_timeout,
            half_open_probes,
        })
    }

    pub fn state(&self) -> CircuitState {
        match self.state.load(Ordering::SeqCst) {
            1 => CircuitState::Open,
            2 => CircuitState::HalfOpen,
            _ => CircuitState::Closed,
        }
    }

    pub fn allow(&self) -> bool {
        match self.state() {
            CircuitState::Closed => true,
            CircuitState::HalfOpen => true,
            CircuitState::Open => {
                let opened_ns = self.last_open_ns.load(Ordering::SeqCst);
                let opened = std::time::UNIX_EPOCH + Duration::from_nanos(opened_ns);
                if let Ok(elapsed) = opened.elapsed() {
                    if elapsed >= self.open_timeout {
                        let _ = self.state.compare_exchange(
                            CircuitState::Open as i32,
                            CircuitState::HalfOpen as i32,
                            Ordering::SeqCst,
                            Ordering::SeqCst,
                        );
                        self.successes.store(0, Ordering::SeqCst);
                        return true;
                    }
                }
                false
            }
        }
    }

    pub fn record_success(&self) {
        match self.state() {
            CircuitState::HalfOpen => {
                let probes = self.successes.fetch_add(1, Ordering::SeqCst) + 1;
                if probes >= self.half_open_probes {
                    self.state
                        .store(CircuitState::Closed as i32, Ordering::SeqCst);
                    self.failures.store(0, Ordering::SeqCst);
                }
            }
            CircuitState::Closed => {
                self.failures.store(0, Ordering::SeqCst);
            }
            _ => {}
        }
    }

    pub fn record_failure(&self) {
        let f = self.failures.fetch_add(1, Ordering::SeqCst) + 1;
        if f >= self.max_failures {
            let now_ns = std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap_or_default()
                .as_nanos() as u64;
            let prev = self.state.compare_exchange(
                CircuitState::Closed as i32,
                CircuitState::Open as i32,
                Ordering::SeqCst,
                Ordering::SeqCst,
            );
            if prev.is_ok() {
                self.last_open_ns.store(now_ns, Ordering::SeqCst);
            }
        }
    }
}

// ---------------------------------------------------------------------------
// Token Bucket
// ---------------------------------------------------------------------------

/// A simple atomic token bucket for hedging rate-limiting.
pub struct TokenBucket {
    tokens: AtomicI64, // nano-tokens (1 token = 1_000_000_000 nano-tokens)
    capacity: i64,
    refill_rate: i64,       // nano-tokens per nanosecond
    last_refill: AtomicU64, // nanosecond timestamp
}

impl TokenBucket {
    pub fn new(capacity: f64, rate_per_second: f64) -> Self {
        let cap_nano = (capacity * 1_000_000_000.0) as i64;
        let rate = rate_per_second as i64; // nano-tokens per nanosecond ≈ tokens/s
        let now_ns = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap_or_default()
            .as_nanos() as u64;
        Self {
            tokens: AtomicI64::new(cap_nano),
            capacity: cap_nano,
            refill_rate: rate.max(1),
            last_refill: AtomicU64::new(now_ns),
        }
    }

    pub fn consume(&self) -> bool {
        let now_ns = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap_or_default()
            .as_nanos() as u64;

        let last = self.last_refill.load(Ordering::Relaxed);
        let elapsed = now_ns.saturating_sub(last);
        if elapsed > 0 {
            let refill = (elapsed as i64).saturating_mul(self.refill_rate);
            if refill > 0 {
                loop {
                    let cur = self.tokens.load(Ordering::Relaxed);
                    let next = (cur + refill).min(self.capacity);
                    if self
                        .tokens
                        .compare_exchange(cur, next, Ordering::Relaxed, Ordering::Relaxed)
                        .is_ok()
                    {
                        let _ = self.last_refill.compare_exchange(
                            last,
                            now_ns,
                            Ordering::Relaxed,
                            Ordering::Relaxed,
                        );
                        break;
                    }
                }
            }
        }

        const ONE_TOKEN: i64 = 1_000_000_000;
        loop {
            let cur = self.tokens.load(Ordering::Relaxed);
            if cur < ONE_TOKEN {
                return false;
            }
            if self
                .tokens
                .compare_exchange(cur, cur - ONE_TOKEN, Ordering::Relaxed, Ordering::Relaxed)
                .is_ok()
            {
                return true;
            }
        }
    }
}

// ---------------------------------------------------------------------------
// RetryPolicy
// ---------------------------------------------------------------------------

pub struct RetryPolicy {
    pub max_attempts: usize,
    pub initial_backoff: Duration,
    pub max_backoff: Duration,
    pub backoff_multiplier: f64,
    pub hedge_delay: Option<Duration>,
    pub hedge_bucket: Option<Arc<TokenBucket>>,
    pub breaker: Option<Arc<CircuitBreaker>>,
}

impl Default for RetryPolicy {
    fn default() -> Self {
        Self {
            max_attempts: 3,
            initial_backoff: Duration::from_millis(100),
            max_backoff: Duration::from_secs(2),
            backoff_multiplier: 2.0,
            hedge_delay: None,
            hedge_bucket: None,
            breaker: None,
        }
    }
}

impl RetryPolicy {
    pub fn with_defaults() -> Self {
        Self {
            breaker: Some(CircuitBreaker::new(5, Duration::from_secs(30), 2)),
            hedge_bucket: Some(Arc::new(TokenBucket::new(5.0, 10.0))),
            ..Default::default()
        }
    }
}

/// Execute `operation` with exponential backoff + optional hedging + circuit breaker.
pub async fn execute_with_retry<T, F, Fut>(policy: &RetryPolicy, operation: F) -> Result<T, String>
where
    F: Fn() -> Fut + Send + Sync + Clone + 'static,
    Fut: std::future::Future<Output = Result<T, String>> + Send,
    T: Send + 'static,
{
    // Circuit breaker check
    if let Some(ref cb) = policy.breaker {
        if !cb.allow() {
            return Err("circuit open: too many recent failures".to_string());
        }
    }

    if let Some(hedge_delay) = policy.hedge_delay {
        return execute_with_hedging(policy, operation, hedge_delay).await;
    }

    execute_retries_only(policy, operation).await
}

async fn execute_retries_only<T, F, Fut>(policy: &RetryPolicy, operation: F) -> Result<T, String>
where
    F: Fn() -> Fut + Send + Sync + Clone,
    Fut: std::future::Future<Output = Result<T, String>> + Send,
    T: Send,
{
    let mut backoff = policy.initial_backoff;
    let mut last_err = String::new();

    for attempt in 1..=policy.max_attempts {
        match operation().await {
            Ok(v) => {
                if let Some(ref cb) = policy.breaker {
                    cb.record_success();
                }
                return Ok(v);
            }
            Err(e) => {
                if let Some(ref cb) = policy.breaker {
                    cb.record_failure();
                }
                last_err = e;
                if attempt == policy.max_attempts {
                    break;
                }
                // Full-jitter: sleep rand[0, backoff)
                let jitter_secs = random::<f64>() * backoff.as_secs_f64();
                tokio::time::sleep(Duration::from_secs_f64(jitter_secs)).await;
                backoff =
                    (Duration::from_secs_f64(backoff.as_secs_f64() * policy.backoff_multiplier))
                        .min(policy.max_backoff);
            }
        }
    }

    Err(last_err)
}

async fn execute_with_hedging<T, F, Fut>(
    policy: &RetryPolicy,
    operation: F,
    hedge_delay: Duration,
) -> Result<T, String>
where
    F: Fn() -> Fut + Send + Sync + Clone + 'static,
    Fut: std::future::Future<Output = Result<T, String>> + Send,
    T: Send + 'static,
{
    let (tx, mut rx) = tokio::sync::mpsc::channel(2);

    let op1 = operation.clone();
    let tx1 = tx.clone();
    tokio::spawn(async move {
        let r = op1().await;
        let _ = tx1.send(r).await;
    });

    tokio::select! {
        res = rx.recv() => {
            return res.unwrap_or_else(|| Err("channel closed".to_string()));
        }
        _ = tokio::time::sleep(hedge_delay) => {}
    }

    // Check token bucket before firing hedge
    let can_hedge = policy
        .hedge_bucket
        .as_ref()
        .map(|b| b.consume())
        .unwrap_or(true);
    if can_hedge {
        let op2 = operation.clone();
        let tx2 = tx.clone();
        tokio::spawn(async move {
            let r = op2().await;
            let _ = tx2.send(r).await;
        });
    }

    // Return whichever succeeds first
    let r1 = rx
        .recv()
        .await
        .unwrap_or_else(|| Err("channel closed".to_string()));
    if r1.is_ok() {
        return r1;
    }
    rx.recv().await.unwrap_or(r1)
}
