#[global_allocator]
static GLOBAL: mimalloc::MiMalloc = mimalloc::MiMalloc;

pub mod batching;
pub mod client_pool;
pub mod compression;
pub mod deadline;
pub mod errors;
pub mod health;
pub mod metadata;
pub mod retry;
pub mod server;
pub mod sniffer;

pub use client_pool::{AnyStream, Balancer, ClientConnPool, RoundRobinBalancer, ConsistentHashBalancer};
pub use errors::{ErrorCode, HelixError};
pub use metadata::get_metadata;
pub use server::{
    handle_http_connection, handle_http_connection_streaming, GrpcServerStream, HttpServiceHandler,
    HttpStreamingHandler, RestRoute, ServerStream,
};
pub use sniffer::{sniff_protocol, Protocol};
pub mod resolver;
pub use resolver::{DnsResolver, Resolver, StaticResolver};
pub mod shm_transport;
pub use batching::{BatchScheduler, LeastConnBalancer};
pub use compression::{get_compressor, Compressor, GzipCompressor};
pub use deadline::{extract_deadline, parse_grpc_timeout};
pub use health::{HealthChecker, HealthStatus};
pub use retry::{execute_with_retry, CircuitBreaker, RetryPolicy, TokenBucket};
pub use shm_transport::ShmConn;
pub mod telemetry;
pub use telemetry::attach_span_context;
pub mod pyo3_runner;
pub use pyo3_runner::PyModelHandler;
pub mod auth;
pub use auth::{get_api_key_principal, get_jwt_claims, validate_api_key, JwtValidator};
pub mod ratelimit;
pub use ratelimit::RateLimiter;
pub mod metrics;
pub use metrics::{MetricsCollector, GLOBAL_METRICS};
pub mod config;
pub use config::{load_config, watch_config, Config};
pub mod ebpf;
pub use ebpf::{has_unix_prefix, load_bpf_sockmap, strip_unix_prefix};

#[cfg(test)]
mod tests_resilience;
