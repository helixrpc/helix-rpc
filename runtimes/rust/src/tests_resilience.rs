#[cfg(test)]
mod resilience_tests {
    use crate::batching::LeastConnBalancer;
    use crate::retry::{
        execute_with_retry, CircuitBreaker, CircuitState, RetryPolicy, TokenBucket,
    };
    use std::sync::atomic::{AtomicUsize, Ordering};
    use std::sync::Arc;
    use std::time::Duration;

    // -----------------------------------------------------------------------
    // CircuitBreaker Tests
    // -----------------------------------------------------------------------

    #[test]
    fn test_cb_initial_state_closed() {
        let cb = CircuitBreaker::new(3, Duration::from_secs(10), 2);
        assert_eq!(cb.state(), CircuitState::Closed);
        assert!(cb.allow());
    }

    #[test]
    fn test_cb_opens_after_max_failures() {
        let cb = CircuitBreaker::new(3, Duration::from_secs(10), 2);
        cb.record_failure();
        cb.record_failure();
        assert_eq!(cb.state(), CircuitState::Closed);
        cb.record_failure();
        assert_eq!(cb.state(), CircuitState::Open);
        assert!(!cb.allow());
    }

    #[test]
    fn test_cb_success_resets_failure_count() {
        let cb = CircuitBreaker::new(3, Duration::from_secs(10), 2);
        cb.record_failure();
        cb.record_failure();
        cb.record_success(); // resets counter to 0
        cb.record_failure(); // 1 failure — should not trip (max=3)
        assert_eq!(cb.state(), CircuitState::Closed);
    }

    #[test]
    fn test_cb_half_open_after_timeout() {
        let cb = CircuitBreaker::new(1, Duration::from_millis(50), 1);
        cb.record_failure();
        assert_eq!(cb.state(), CircuitState::Open);
        std::thread::sleep(Duration::from_millis(60));
        assert!(cb.allow(), "should allow probe after timeout");
        assert_eq!(cb.state(), CircuitState::HalfOpen);
    }

    #[test]
    fn test_cb_closes_after_half_open_probes() {
        let cb = CircuitBreaker::new(1, Duration::from_millis(10), 2);
        cb.record_failure();
        std::thread::sleep(Duration::from_millis(15));
        cb.allow(); // → HalfOpen
        cb.record_success();
        cb.record_success(); // 2 probes → Closed
        assert_eq!(cb.state(), CircuitState::Closed);
    }

    #[test]
    fn test_cb_concurrent_failures() {
        let cb = Arc::new(CircuitBreaker::new(10, Duration::from_secs(60), 2));
        let mut handles = vec![];
        for _ in 0..20 {
            let cb_clone = cb.clone();
            handles.push(std::thread::spawn(move || cb_clone.record_failure()));
        }
        for h in handles {
            let _ = h.join();
        }
        assert_eq!(cb.state(), CircuitState::Open);
    }

    // -----------------------------------------------------------------------
    // TokenBucket Tests
    // -----------------------------------------------------------------------

    #[test]
    fn test_bucket_consumes_up_to_capacity() {
        let tb = TokenBucket::new(3.0, 0.0);
        assert!(tb.consume());
        assert!(tb.consume());
        assert!(tb.consume());
        assert!(!tb.consume()); // exhausted
    }

    #[test]
    fn test_bucket_refills() {
        let tb = TokenBucket::new(1.0, 50.0); // 50 tokens/s
        tb.consume(); // drain
        assert!(!tb.consume());
        std::thread::sleep(Duration::from_millis(30)); // ~1.5 tokens refilled
        assert!(tb.consume());
    }

    #[test]
    fn test_bucket_concurrent_safe() {
        let tb = Arc::new(TokenBucket::new(10.0, 0.0));
        let counter = Arc::new(AtomicUsize::new(0));
        let mut handles = vec![];
        for _ in 0..50 {
            let tb = tb.clone();
            let counter = counter.clone();
            handles.push(std::thread::spawn(move || {
                if tb.consume() {
                    counter.fetch_add(1, Ordering::Relaxed);
                }
            }));
        }
        for h in handles {
            let _ = h.join();
        }
        assert_eq!(counter.load(Ordering::Relaxed), 10);
    }

    // -----------------------------------------------------------------------
    // execute_with_retry Tests
    // -----------------------------------------------------------------------

    #[tokio::test]
    async fn test_retry_succeeds_first_attempt() {
        let policy = RetryPolicy::default();
        let calls = Arc::new(AtomicUsize::new(0));
        let c = calls.clone();
        let result: Result<String, String> = execute_with_retry(&policy, move || {
            let cc = c.clone();
            async move {
                cc.fetch_add(1, Ordering::Relaxed);
                Ok("ok".to_string())
            }
        })
        .await;
        assert_eq!(result.unwrap(), "ok");
        assert_eq!(calls.load(Ordering::Relaxed), 1);
    }

    #[tokio::test]
    async fn test_retry_exhausts_attempts() {
        let policy = RetryPolicy {
            max_attempts: 3,
            initial_backoff: Duration::from_millis(1),
            max_backoff: Duration::from_millis(10),
            ..Default::default()
        };
        let calls = Arc::new(AtomicUsize::new(0));
        let c = calls.clone();
        let result: Result<String, String> = execute_with_retry(&policy, move || {
            let cc = c.clone();
            async move {
                cc.fetch_add(1, Ordering::Relaxed);
                Err("transient".to_string())
            }
        })
        .await;
        assert!(result.is_err());
        assert_eq!(calls.load(Ordering::Relaxed), 3);
    }

    #[tokio::test]
    async fn test_retry_succeeds_on_second_attempt() {
        let policy = RetryPolicy {
            max_attempts: 3,
            initial_backoff: Duration::from_millis(1),
            ..Default::default()
        };
        let calls = Arc::new(AtomicUsize::new(0));
        let c = calls.clone();
        let result: Result<String, String> = execute_with_retry(&policy, move || {
            let cc = c.clone();
            async move {
                let n = cc.fetch_add(1, Ordering::Relaxed) + 1;
                if n < 2 {
                    Err("not ready".to_string())
                } else {
                    Ok("ready".to_string())
                }
            }
        })
        .await;
        assert_eq!(result.unwrap(), "ready");
        assert_eq!(calls.load(Ordering::Relaxed), 2);
    }

    #[tokio::test]
    async fn test_retry_circuit_open_fast_fails() {
        let cb = CircuitBreaker::new(1, Duration::from_secs(60), 1);
        cb.record_failure(); // trip

        let policy = RetryPolicy {
            breaker: Some(cb),
            ..Default::default()
        };
        let calls = Arc::new(AtomicUsize::new(0));
        let c = calls.clone();
        let result: Result<String, String> = execute_with_retry(&policy, move || {
            let cc = c.clone();
            async move {
                cc.fetch_add(1, Ordering::Relaxed);
                Ok("ok".to_string())
            }
        })
        .await;
        assert!(result.is_err());
        assert_eq!(
            calls.load(Ordering::Relaxed),
            0,
            "operation must not be called when circuit is open"
        );
    }

    #[tokio::test]
    async fn test_hedging_returns_fastest() {
        use std::time::Instant;

        let policy = RetryPolicy {
            max_attempts: 1,
            hedge_delay: Some(Duration::from_millis(10)),
            hedge_bucket: Some(Arc::new(TokenBucket::new(10.0, 100.0))),
            ..Default::default()
        };

        let calls = Arc::new(AtomicUsize::new(0));
        let c = calls.clone();

        let start = Instant::now();
        let result: Result<String, String> = execute_with_retry(&policy, move || {
            let cc = c.clone();
            async move {
                let n = cc.fetch_add(1, Ordering::SeqCst) + 1;
                if n == 1 {
                    tokio::time::sleep(Duration::from_millis(500)).await;
                    Ok("slow".to_string())
                } else {
                    Ok("fast".to_string())
                }
            }
        })
        .await;
        let elapsed = start.elapsed();

        assert_eq!(result.unwrap(), "fast");
        assert!(
            elapsed < Duration::from_millis(300),
            "hedging should be fast, took {:?}",
            elapsed
        );
        assert!(calls.load(Ordering::SeqCst) >= 2);
    }

    // -----------------------------------------------------------------------
    // LeastConnBalancer Tests
    // -----------------------------------------------------------------------

    #[tokio::test]
    async fn test_least_conn_empty_targets_error() {
        let lb = LeastConnBalancer::new();
        let result: Result<String, String> = lb.next(&[]).await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_least_conn_routes_to_least_busy() {
        let lb = LeastConnBalancer::new();
        let targets = vec!["a".to_string(), "b".to_string(), "c".to_string()];
        lb.register(&["a", "b", "c"]).await;

        // Load "a" with 5 requests without calling done
        for _ in 0..5 {
            let _: String = lb.next(&targets).await.unwrap();
        }

        // Next should NOT go to "a"
        let chosen: String = lb.next(&targets).await.unwrap();
        assert_ne!(chosen, "a", "should route away from the most loaded node");
    }

    #[tokio::test]
    async fn test_least_conn_done_decrements() {
        let lb = LeastConnBalancer::new();
        let targets = vec!["x".to_string()];
        lb.register(&["x"]).await;
        let _: String = lb.next(&targets).await.unwrap();
        lb.done("x").await;
        let chosen: String = lb.next(&targets).await.unwrap();
        assert_eq!(chosen, "x");
        lb.done("x").await;
    }

    // -----------------------------------------------------------------------
    // Auth & Rate Limit Tests
    // -----------------------------------------------------------------------

    #[test]
    fn test_jwt_validator_hmac() {
        use crate::auth::JwtValidator;
        use hyper::Request;
        use jsonwebtoken::{encode, EncodingKey, Header};
        use serde_json::Value;
        use std::collections::HashMap;
        use std::time::{SystemTime, UNIX_EPOCH};

        let secret = b"my_test_secret_key_1234567890_long";
        let validator = JwtValidator::new_hmac(secret, vec!["sub".to_string()]);

        let mut claims = HashMap::new();
        claims.insert("sub".to_string(), Value::String("user123".to_string()));
        let exp = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs()
            + 3600;
        claims.insert("exp".to_string(), Value::Number(exp.into()));

        let token = encode(
            &Header::default(),
            &claims,
            &EncodingKey::from_secret(secret),
        )
        .unwrap();

        let req = Request::builder()
            .header("authorization", format!("Bearer {}", token))
            .body(())
            .unwrap();

        let parsed = validator.validate_request(&req).unwrap();
        assert_eq!(parsed.get("sub").unwrap().as_str().unwrap(), "user123");
    }

    #[test]
    fn test_validate_api_key() {
        use crate::auth::validate_api_key;
        use hyper::header::HeaderMap;
        use std::collections::HashMap;

        let mut valid_keys = HashMap::new();
        valid_keys.insert("secret-key-1".to_string(), "admin".to_string());

        let mut headers = HeaderMap::new();
        headers.insert("x-api-key", "secret-key-1".parse().unwrap());

        let principal = validate_api_key(&headers, &valid_keys).unwrap();
        assert_eq!(principal, "admin");
    }

    #[test]
    fn test_rate_limiter() {
        use crate::ratelimit::RateLimiter;
        let limiter = RateLimiter::new(10.0, Some(2));

        let (rem, allowed) = limiter.allow("client1");
        assert!(allowed);
        assert_eq!(rem, 1);

        let (rem2, allowed2) = limiter.allow("client1");
        assert!(allowed2);
        assert_eq!(rem2, 0);

        let (_, allowed3) = limiter.allow("client1");
        assert!(!allowed3);
    }

    #[test]
    fn test_metrics_collection() {
        use crate::metrics::MetricsCollector;
        use std::time::Duration;

        let collector = MetricsCollector::new();
        collector.record("POST", "/predict", 200, Duration::from_millis(50));
        collector.record("POST", "/predict", 500, Duration::from_millis(150));

        let formatted = collector.format_prometheus();
        assert!(formatted
            .contains("helix_requests_total{method=\"POST\",path=\"/predict\",status=\"200\"} 1"));
        assert!(formatted
            .contains("helix_requests_total{method=\"POST\",path=\"/predict\",status=\"500\"} 1"));
        assert!(formatted.contains("helix_errors_total{method=\"POST\",path=\"/predict\"} 1"));
        assert!(formatted
            .contains("helix_request_duration_seconds_count{method=\"POST\",path=\"/predict\"} 2"));
    }

    #[test]
    fn test_consistent_hash_balancer() {
        use crate::client_pool::{Balancer, ConsistentHashBalancer};

        let lb = ConsistentHashBalancer::new(50);
        let targets = vec![
            "127.0.0.1:8081".to_string(),
            "127.0.0.1:8082".to_string(),
            "127.0.0.1:8083".to_string(),
        ];

        // 1. Same key maps to the same target consistently
        let key1 = "system-prompt-llm-1";
        let choice1 = lb.next_with_key(&targets, key1).unwrap();

        for _ in 0..20 {
            let choice = lb.next_with_key(&targets, key1).unwrap();
            assert_eq!(choice, choice1);
        }

        // 2. Different key maps to a stable target consistently
        let key2 = "different-system-prompt";
        let choice2 = lb.next_with_key(&targets, key2).unwrap();

        for _ in 0..20 {
            let choice = lb.next_with_key(&targets, key2).unwrap();
            assert_eq!(choice, choice2);
        }
    }

    #[tokio::test]
    async fn test_quic_transport() {
        use crate::quic_transport::QuicListener;
        use tokio::io::{AsyncReadExt, AsyncWriteExt};
        use tokio::net::UdpSocket;

        let addr: std::net::SocketAddr = "127.0.0.1:0".parse().unwrap();
        let (listener, mut rx) = QuicListener::bind(addr).await.unwrap();
        let bound_addr = listener.local_addr().unwrap();

        // Start server accept in background task
        let server_handle = tokio::spawn(async move {
            let mut stream = rx.recv().await.unwrap();
            let mut buf = vec![0u8; 1024];
            let n = stream.read(&mut buf).await.unwrap();
            assert_eq!(&buf[..n], b"hello-from-udp-client");
            stream.write_all(b"hello-from-udp-server").await.unwrap();
        });

        // Client dials UDP
        let client_socket = UdpSocket::bind("127.0.0.1:0").await.unwrap();
        client_socket.connect(bound_addr).await.unwrap();

        // Write virtual stream packet (Stream ID = 42)
        let payload = b"hello-from-udp-client";
        let mut packet = Vec::with_capacity(4 + payload.len());
        packet.extend_from_slice(&42u32.to_be_bytes());
        packet.extend_from_slice(payload);

        client_socket.send(&packet).await.unwrap();

        // Wait for server accept and processing
        server_handle.await.unwrap();

        // Client reads response packet
        let mut resp_buf = vec![0u8; 1024];
        let n = client_socket.recv(&mut resp_buf).await.unwrap();
        assert!(n >= 4);
        let stream_id = u32::from_be_bytes([resp_buf[0], resp_buf[1], resp_buf[2], resp_buf[3]]);
        assert_eq!(stream_id, 42);
        assert_eq!(&resp_buf[4..n], b"hello-from-udp-server");
    }
}

