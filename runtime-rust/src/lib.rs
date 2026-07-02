pub mod sniffer;
pub mod server;
pub mod metadata;
pub mod errors;
pub mod client_pool;
pub mod deadline;
pub mod health;
pub mod compression;
pub mod batching;
pub mod retry;

pub use sniffer::{sniff_protocol, Protocol};
pub use server::{
    handle_http_connection, handle_http_connection_streaming,
    HttpServiceHandler, HttpStreamingHandler, RestRoute,
    ServerStream, GrpcServerStream,
};
pub use metadata::get_metadata;
pub use errors::{ErrorCode, HelixError};
pub use client_pool::{ClientConnPool, Balancer, RoundRobinBalancer};
pub mod resolver;
pub use resolver::{Resolver, StaticResolver};
pub mod shm_transport;
pub use shm_transport::ShmConn;
pub use deadline::{parse_grpc_timeout, extract_deadline};
pub use health::{HealthChecker, HealthStatus};
pub use compression::{Compressor, GzipCompressor, get_compressor};
pub use batching::{BatchScheduler, LeastConnBalancer};
pub use retry::{CircuitBreaker, TokenBucket, RetryPolicy, execute_with_retry};
pub mod telemetry; pub use telemetry::attach_span_context;
pub mod pyo3_runner;
pub use pyo3_runner::PyModelHandler;
pub mod auth;
pub use auth::{JwtValidator, get_jwt_claims, get_api_key_principal, validate_api_key};
pub mod ratelimit;
pub use ratelimit::RateLimiter;


#[cfg(test)]
mod tests_resilience;
