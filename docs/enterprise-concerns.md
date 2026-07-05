# Enterprise Concerns: Logging, Secrets, Config, & Databases

This guide covers implementation patterns and best practices for integrating Helix RPC with enterprise systems.

---

## 1. Structured Logging

For production monitoring, Helix RPC services should emit logs in structured JSON format, injecting OpenTelemetry `trace_id` and `span_id` headers to correlate logs with request traces.

**Go Example (using standard `log/slog`):**
```go
import "log/slog"

// Initialize JSON logger
logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
slog.SetDefault(logger)

// Log request execution with trace context
slog.Info("processing model prediction",
    "method", "/v1.ModelService/Predict",
    "trace_id", md.Get("traceparent"), // Correlates log with telemetry span
)
```

**Rust Example (using `tracing`):**
```rust
use tracing::{info, info_span};

// Spans automatically inherit trace metadata
let span = info_span!("predict_request", method = "/v1.ModelService/Predict");
let _guard = span.enter();

info!("calculating inference output");
```

---

## 2. Secrets & Key Management

Never commit mTLS certificates, API keys, or JWT validation secrets to code or config files. Use a secure Secret Manager (HashiCorp Vault, AWS Secrets Manager, GCP Secret Manager).

### Fetching Secrets at Boot
Services should resolve credentials dynamically from the environment or directly via APIs at startup.

**Go pattern utilizing AWS Secrets Manager:**
```go
import (
    "github.com/aws/aws-sdk-go/aws"
    "github.com/aws/aws-sdk-go/service/secretsmanager"
)

func LoadJWTSecret() (string, error) {
    svc := secretsmanager.New(session.New())
    result, err := svc.GetSecretValue(&secretsmanager.GetSecretValueInput{
        SecretId: aws.String("helix/production/jwt-key"),
    })
    if err != nil {
        return "", err
    }
    return *result.SecretString, nil
}
```

---

## 3. Configuration Management

While `helix.json` supports local file-based loading and watching, in enterprise environments this config is typically sourced from Consul KV, AWS AppConfig, or etcd.

### Watching External Config Engines
Instead of checking file modifications, update the `WatchConfig` routine to poll a key-value store or receive updates via pub/sub channels.

**Consul KV Configuration Watch (Go):**
```go
func WatchConsulConfig(consulClient *api.Client, key string, onChange func(*Config)) {
    go func() {
        var lastIndex uint64
        for {
            pair, meta, err := consulClient.KV().Get(key, &api.QueryOptions{WaitIndex: lastIndex})
            if err != nil {
                time.Sleep(5 * time.Second)
                continue
            }
            lastIndex = meta.LastIndex
            
            var cfg Config
            if err := json.Unmarshal(pair.Value, &cfg); err == nil {
                onChange(&cfg)
            }
        }
    }()
}
```

---

## 4. Database Connection Pooling

High-throughput AI applications must share a single, thread-safe database connection pool across all connection threads.

- **Go:** Use `sql.DB` (thread-safe by default). Set maximum open connections (`SetMaxOpenConns(100)`) and maximum idle connections (`SetMaxIdleConns(10)`) to prevent exhausting socket descriptors.
- **Rust:** Use `sqlx` with an `Arc<Pool<Postgres>>` passed into your `HttpServiceHandler` to allow lock-free concurrent queries inside tokio tasks.
- **Python:** Use `asyncpg` pools for PostgreSQL, acquiring connections asynchronously to prevent blocking the single-threaded asyncio event loop:
  ```python
  # Acquire connection from shared pool
  async with db_pool.acquire() as conn:
      result = await conn.fetch("SELECT * FROM models")
  ```

---

## 5. KVCache Consistent-Hash Prefix Routing

In large-scale AI deployment settings, serving models concurrently requires optimizing key-value (KV) prompt caching. Helix RPC provides a `ConsistentHashBalancer` that hashes the prompt prefix metadata to direct traffic to corresponding nodes.

### Configuration

**Go Usage:**
```go
import "github.com/helix-rpc/helix/runtime-go"

balancer := runtime.NewConsistentHashBalancer(50) // 50 virtual nodes
targets := []string{"10.0.0.1:9090", "10.0.0.2:9090", "10.0.0.3:9090"}

// Consistent selection based on prompt prefix hash
target, err := balancer.NextWithKey(targets, "system-prompt-v1")
```

**Rust Usage:**
```rust
use helix_rt::{ConsistentHashBalancer, Balancer};

let balancer = ConsistentHashBalancer::new(50);
let targets = vec!["10.0.0.1:9090".to_string(), "10.0.0.2:9090".to_string()];

let target = balancer.next_with_key(&targets, "system-prompt-v1").unwrap();
```

---

## 6. QUIC / HTTP/3 Sniffing Transport

Mobile clients running under lossy connections experience head-of-line blocking using traditional TCP/HTTP2 multiplexing. Helix runtimes support high-performance UDP-based QUIC stream transport.

- **Go**: Binds to UDP port to listen for virtual stream frames using `QUICTransportListener`.
- **Rust**: Uses a tokio-based `QuicListener` running on a separate thread socket accept loop.

---

## 7. Multi-Tenant Rate Limiting & Auth

To manage traffic limits dynamically across multiple consumer tiers, use `MultiTenantRateLimiter`.

```go
limiter := runtime.NewMultiTenantRateLimiter(
    runtime.TenantConfig{RequestsPerSecond: 5, BurstSize: 5}, // Default fallback
    func(r *http.Request) (string, runtime.TenantConfig, error) {
        tenantID := r.Header.Get("x-tenant-id")
        // Resolve and return tenant configuration dynamically...
        return tenantID, runtime.TenantConfig{RequestsPerSecond: 100, BurstSize: 100}, nil
    },
)
```

---

## 8. Kubernetes Controller & Schema CRD

Helix RPC features a Kubernetes Custom Resource Definition (`HelixSchema`) and watch controller to compile service contracts dynamically inside K8s clusters.

### HelixSchema Resource
```yaml
apiVersion: helixrpc.io/v1alpha1
kind: HelixSchema
metadata:
  name: user-profile-service
  namespace: production
spec:
  idlContent: |
    syntax = "proto3";
    package user;
    message ProfileRequest { int64 user_id = 1; }
  language: rust
  outputDirectory: /app/generated
```

