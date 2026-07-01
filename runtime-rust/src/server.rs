use std::future::Future;
use std::pin::Pin;
use std::sync::Arc;
use tokio::net::TcpStream;
use tokio::sync::mpsc;
use hyper::{Body, Request, Response, StatusCode};
use hyper::service::Service;
use hyper::server::conn::Http;
use std::task::{Context, Poll};

// ---------------------------------------------------------------------------
// Bidirectional Streaming primitives
// ---------------------------------------------------------------------------

/// Trait for bidirectional gRPC streaming.
/// Mirrors Go's `ServerStream` interface – each call to `recv` reads the next
/// gRPC length-prefixed frame from the client and each call to `send` writes
/// one frame back.
#[async_trait::async_trait]
pub trait ServerStream: Send + Sync {
    /// Receive the next raw protobuf payload (without the 5-byte gRPC header).
    /// Returns `Ok(None)` when the client has finished sending.
    async fn recv(&mut self) -> Result<Option<Vec<u8>>, String>;

    /// Send a raw protobuf payload back to the client, wrapped in a 5-byte
    /// gRPC frame header.
    async fn send(&self, payload: Vec<u8>) -> Result<(), String>;
}

/// Concrete implementation that wraps `hyper::Body` (for receiving) and an
/// `mpsc::Sender<Vec<u8>>` (for sending) to implement full-duplex streaming
/// over a single HTTP/2 connection.
pub struct GrpcServerStream {
    body: Body,
    buf: Vec<u8>,
    tx: mpsc::Sender<Result<Vec<u8>, Box<dyn std::error::Error + Send + Sync>>>,
}

impl GrpcServerStream {
    pub fn new(
        body: Body,
        tx: mpsc::Sender<Result<Vec<u8>, Box<dyn std::error::Error + Send + Sync>>>,
    ) -> Self {
        GrpcServerStream {
            body,
            buf: Vec::new(),
            tx,
        }
    }

    /// Pull exactly `n` bytes from the internal buffer, reading more chunks
    /// from the body if necessary.
    async fn read_exact(&mut self, n: usize) -> Result<Vec<u8>, String> {
        use hyper::body::HttpBody;
        while self.buf.len() < n {
            match Pin::new(&mut self.body).data().await {
                Some(Ok(chunk)) => self.buf.extend_from_slice(&chunk),
                Some(Err(e)) => return Err(format!("body read error: {}", e)),
                None => return Err("unexpected end of stream".to_string()),
            }
        }
        let data = self.buf.drain(..n).collect();
        Ok(data)
    }

    /// Check if there are any more bytes available from the body.
    async fn has_more(&mut self) -> bool {
        use hyper::body::HttpBody;
        if !self.buf.is_empty() {
            return true;
        }
        match Pin::new(&mut self.body).data().await {
            Some(Ok(chunk)) => {
                self.buf.extend_from_slice(&chunk);
                !self.buf.is_empty()
            }
            _ => false,
        }
    }
}

#[async_trait::async_trait]
impl ServerStream for GrpcServerStream {
    async fn recv(&mut self) -> Result<Option<Vec<u8>>, String> {
        // Try to read the 5-byte gRPC frame header.
        // If the body is exhausted, return None (clean EOF).
        if !self.has_more().await {
            return Ok(None);
        }
        let header = self.read_exact(5).await?;
        let length = u32::from_be_bytes([header[1], header[2], header[3], header[4]]) as usize;
        let payload = self.read_exact(length).await?;
        Ok(Some(payload))
    }

    async fn send(&self, payload: Vec<u8>) -> Result<(), String> {
        let mut frame = Vec::with_capacity(5 + payload.len());
        frame.push(0u8); // uncompressed
        frame.extend_from_slice(&(payload.len() as u32).to_be_bytes());
        frame.extend_from_slice(&payload);
        self.tx.send(Ok(frame)).await.map_err(|e| format!("send error: {}", e))
    }
}

/// Handler trait for streaming RPCs.
/// Implementors decide per-path whether a streaming handler exists.
#[async_trait::async_trait]
pub trait HttpStreamingHandler: Send + Sync + 'static {
    /// Return `true` if `path` should be handled as a streaming RPC.
    fn is_streaming(&self, path: &str) -> bool;

    /// Execute the streaming handler.
    async fn handle_stream(&self, path: &str, stream: Box<dyn ServerStream>) -> Result<(), String>;
}

#[async_trait::async_trait]
pub trait HttpServiceHandler: Send + Sync + 'static {
    async fn handle_request(&self, path: &str, body: Vec<u8>, is_json: bool) -> Result<(Vec<u8>, String), String>;
}

#[derive(Clone, Debug)]
pub struct RestRoute {
    pub method: String,
    pub pattern: String,
    pub path_parts: Vec<String>,
    pub handler_path: String,
}

impl RestRoute {
    pub fn new(method: &str, pattern: &str, handler_path: &str) -> Self {
        let parts: Vec<String> = pattern
            .trim_start_matches('/')
            .trim_end_matches('/')
            .split('/')
            .filter(|s| !s.is_empty())
            .map(|s| s.to_string())
            .collect();
        RestRoute {
            method: method.to_uppercase(),
            pattern: pattern.to_string(),
            path_parts: parts,
            handler_path: handler_path.to_string(),
        }
    }
}

pub struct HelixHttpService<H> {
    pub handler: Arc<H>,
    pub rest_routes: Vec<RestRoute>,
    pub streaming_handler: Option<Arc<dyn HttpStreamingHandler>>,
    pub health_checker: Option<crate::health::HealthChecker>,
}

impl<H, B> Service<Request<B>> for HelixHttpService<H>
where
    H: HttpServiceHandler,
    B: hyper::body::HttpBody + Send + 'static,
    B::Data: Send,
    B::Error: Into<Box<dyn std::error::Error + Send + Sync>>,
{
    type Response = Response<Body>;
    type Error = hyper::Error;
    type Future = Pin<Box<dyn Future<Output = Result<Self::Response, Self::Error>> + Send>>;

    fn poll_ready(&mut self, _: &mut Context<'_>) -> Poll<Result<(), Self::Error>> {
        Poll::Ready(Ok(()))
    }

    fn call(&mut self, req: Request<B>) -> Self::Future {
        let handler = self.handler.clone();
        let rest_routes = self.rest_routes.clone();
        let streaming_handler = self.streaming_handler.clone();
        let health_checker = self.health_checker.clone();
        Box::pin(async move {
            let path = req.uri().path().to_string();
            let req_method = req.method().as_str().to_uppercase();

            let content_type = req.headers()
                .get("content-type")
                .and_then(|v| v.to_str().ok())
                .unwrap_or("");

            // If REST endpoint call, default content_type to application/json if empty
            let is_json = content_type == "application/json" || content_type.is_empty();
            let is_grpc = content_type == "application/grpc";

            if !is_json && !is_grpc {
                let response = Response::builder()
                    .status(StatusCode::BAD_REQUEST)
                    .body(Body::from("Unsupported Content-Type"))
                    .unwrap();
                return Ok(response);
            }

            // Get grpc-encoding header
            let grpc_encoding = req.headers()
                .get("grpc-encoding")
                .and_then(|v| v.to_str().ok())
                .unwrap_or("")
                .to_string();

            // Extract deadline
            let deadline = crate::deadline::extract_deadline(req.headers());

            // Extract metadata from request headers
            let mut md = std::collections::HashMap::new();
            for (k, v) in req.headers() {
                if let Ok(val_str) = v.to_str() {
                    md.entry(k.as_str().to_lowercase())
                        .or_insert_with(Vec::new)
                        .push(val_str.to_string());
                }
            }

            // Read request body
            let body_bytes = match hyper::body::to_bytes(req.into_body()).await {
                Ok(bytes) => bytes.to_vec(),
                Err(_) => {
                    return Ok(Response::builder()
                        .status(StatusCode::INTERNAL_SERVER_ERROR)
                        .body(Body::from("Failed to read body"))
                        .unwrap());
                }
            };

            let mut request_payload = body_bytes;

            // Intercept health check requests
            if path == "/grpc.health.v1.Health/Check" {
                if let Some(ref hc) = health_checker {
                    let mut payload = request_payload.clone();
                    if is_grpc {
                        if payload.len() >= 5 {
                            let length = u32::from_be_bytes([payload[1], payload[2], payload[3], payload[4]]) as usize;
                            payload = payload[5..5+length].to_vec();
                        } else {
                            payload = Vec::new();
                        }
                    }

                    match hc.handle_request(&payload, is_json).await {
                        Ok((resp_bytes, resp_content_type)) => {
                            if is_grpc {
                                let mut frame = Vec::with_capacity(5 + resp_bytes.len());
                                frame.push(0); // uncompressed
                                frame.extend_from_slice(&(resp_bytes.len() as u32).to_be_bytes());
                                frame.extend_from_slice(&resp_bytes);
                                let response = Response::builder()
                                    .status(StatusCode::OK)
                                    .header("content-type", "application/grpc")
                                    .header("grpc-status", "0")
                                    .body(Body::from(frame))
                                    .unwrap();
                                return Ok(response);
                            } else {
                                let response = Response::builder()
                                    .status(StatusCode::OK)
                                    .header("content-type", resp_content_type)
                                    .body(Body::from(resp_bytes))
                                    .unwrap();
                                return Ok(response);
                            }
                        }
                        Err(e) => {
                            if is_grpc {
                                let response = Response::builder()
                                    .status(StatusCode::OK)
                                    .header("content-type", "application/grpc")
                                    .header("grpc-status", "5") // NOT_FOUND
                                    .header("grpc-message", e)
                                    .body(Body::empty())
                                    .unwrap();
                                return Ok(response);
                            } else {
                                let response = Response::builder()
                                    .status(StatusCode::NOT_FOUND)
                                    .body(Body::from(e))
                                    .unwrap();
                                return Ok(response);
                            }
                        }
                    }
                }
            }

            let mut matched_path = path.clone();
            let mut path_params = std::collections::HashMap::new();

            // Match against registered REST routes
            for r in &rest_routes {
                if r.method == req_method {
                    let req_parts: Vec<&str> = path
                        .trim_start_matches('/')
                        .trim_end_matches('/')
                        .split('/')
                        .filter(|s| !s.is_empty())
                        .collect();
                    if r.path_parts.len() == req_parts.len() {
                        let mut match_ok = true;
                        let mut temp_params = std::collections::HashMap::new();
                        for (i, part) in r.path_parts.iter().enumerate() {
                            if part.starts_with('{') && part.ends_with('}') {
                                let param_name = &part[1..part.len() - 1];
                                temp_params.insert(param_name.to_string(), req_parts[i].to_string());
                            } else if part != req_parts[i] {
                                match_ok = false;
                                break;
                            }
                        }
                        if match_ok {
                            matched_path = r.handler_path.clone();
                            path_params = temp_params;
                            break;
                        }
                    }
                }
            }

            // Merge path parameters into JSON body
            if is_json && !path_params.is_empty() {
                let mut json_val: serde_json::Value = if request_payload.is_empty() {
                    serde_json::Value::Object(serde_json::Map::new())
                } else {
                    serde_json::from_slice(&request_payload).unwrap_or_else(|_| serde_json::Value::Object(serde_json::Map::new()))
                };

                if let Some(obj) = json_val.as_object_mut() {
                    for (k, v) in path_params {
                        if let Ok(num) = v.parse::<i64>() {
                            obj.insert(k, serde_json::Value::Number(num.into()));
                        } else {
                            obj.insert(k, serde_json::Value::String(v));
                        }
                    }
                    if let Ok(new_body) = serde_json::to_vec(&json_val) {
                        request_payload = new_body;
                    }
                }
            }

            // --- Streaming dispatch ---
            // If this is a gRPC call and a streaming handler is registered for
            // the path, set up the bidirectional channel and hand off.
            if is_grpc {
                if let Some(ref sh) = streaming_handler {
                    if sh.is_streaming(&matched_path) {
                        // Reconstruct the raw body from bytes already consumed.
                        let full_body = hyper::Body::from(request_payload.clone());
                        // NOT consumed yet – we gave the whole body to from().

                        let (tx, mut rx) = mpsc::channel::<Result<Vec<u8>, Box<dyn std::error::Error + Send + Sync>>>(64);
                        let stream_obj = GrpcServerStream::new(full_body, tx);

                        let sh_clone = sh.clone();
                        let stream_path = matched_path.clone();
                        let handle = tokio::spawn(async move {
                            sh_clone.handle_stream(&stream_path, Box::new(stream_obj)).await
                        });

                        // Collect the outbound frames from the channel into a Body.
                        let (mut body_tx, body_rx) = Body::channel();
                        tokio::spawn(async move {
                            while let Some(chunk) = rx.recv().await {
                                match chunk {
                                    Ok(data) => {
                                        if body_tx.send_data(hyper::body::Bytes::from(data)).await.is_err() {
                                            break;
                                        }
                                    }
                                    Err(_) => break,
                                }
                            }
                        });

                        let grpc_status = match handle.await {
                            Ok(Ok(())) => "0",
                            _ => "13",
                        };

                        let response = Response::builder()
                            .status(StatusCode::OK)
                            .header("content-type", "application/grpc")
                            .header("grpc-status", grpc_status)
                            .body(body_rx)
                            .unwrap();
                        return Ok(response);
                    }
                }

                // Unary gRPC: strip 5-byte frame header
                if request_payload.len() < 5 {
                    let res = Response::builder()
                        .status(StatusCode::OK)
                        .header("content-type", "application/grpc")
                        .header("grpc-status", "13") // INTERNAL
                        .header("grpc-message", "invalid frame header length")
                        .body(Body::empty())
                        .unwrap();
                    return Ok(res);
                }
                let compressed_flag = request_payload[0];
                let length = u32::from_be_bytes([request_payload[1], request_payload[2], request_payload[3], request_payload[4]]) as usize;
                request_payload = request_payload[5..5+length].to_vec();

                // Decompress if compressed flag is 1
                if compressed_flag == 1 {
                    if !grpc_encoding.is_empty() {
                        if let Some(compressor) = crate::compression::get_compressor(&grpc_encoding) {
                            match compressor.decompress(&request_payload) {
                                Ok(decompressed) => request_payload = decompressed,
                                Err(e) => {
                                    let res = Response::builder()
                                        .status(StatusCode::OK)
                                        .header("content-type", "application/grpc")
                                        .header("grpc-status", "13")
                                        .header("grpc-message", format!("decompression error: {}", e))
                                        .body(Body::empty())
                                        .unwrap();
                                    return Ok(res);
                                }
                            }
                        } else {
                            let res = Response::builder()
                                .status(StatusCode::OK)
                                .header("content-type", "application/grpc")
                                .header("grpc-status", "12") // UNIMPLEMENTED
                                .header("grpc-message", format!("unsupported grpc-encoding: {}", grpc_encoding))
                                .body(Body::empty())
                                .unwrap();
                            return Ok(res);
                        }
                    }
                }
            }

            // Call the handler inside the tokio task-local metadata context scope
            let handler_fut = crate::metadata::METADATA.scope(md, async move {
                handler.handle_request(&matched_path, request_payload, is_json).await
            });

            let handler_res = if let Some(timeout_duration) = deadline {
                match tokio::time::timeout(timeout_duration, handler_fut).await {
                    Ok(res) => res,
                    Err(_) => Err("deadline exceeded".to_string()),
                }
            } else {
                handler_fut.await
            };

            match handler_res {
                Ok((resp_bytes, resp_content_type)) => {
                    if is_grpc {
                        let mut final_payload = resp_bytes;
                        let mut compress_flag = 0;
                        if !grpc_encoding.is_empty() {
                            if let Some(compressor) = crate::compression::get_compressor(&grpc_encoding) {
                                match compressor.compress(&final_payload) {
                                    Ok(compressed) => {
                                        final_payload = compressed;
                                        compress_flag = 1;
                                    }
                                    Err(_) => {} // fallback to uncompressed
                                }
                            }
                        }

                        let mut frame = Vec::with_capacity(5 + final_payload.len());
                        frame.push(compress_flag);
                        frame.extend_from_slice(&(final_payload.len() as u32).to_be_bytes());
                        frame.extend_from_slice(&final_payload);

                        let mut builder = Response::builder()
                            .status(StatusCode::OK)
                            .header("content-type", "application/grpc")
                            .header("grpc-status", "0"); // OK
                        
                        if compress_flag == 1 {
                            builder = builder.header("grpc-encoding", &grpc_encoding);
                        }

                        let response = builder.body(Body::from(frame)).unwrap();
                        Ok(response)
                    } else {
                        let response = Response::builder()
                            .status(StatusCode::OK)
                            .header("content-type", resp_content_type)
                            .body(Body::from(resp_bytes))
                            .unwrap();
                        Ok(response)
                    }
                }
                Err(err_msg) => {
                    if is_grpc {
                        let status_code = if err_msg.contains("deadline exceeded") {
                            "4" // DEADLINE_EXCEEDED
                        } else {
                            "13" // INTERNAL
                        };
                        let response = Response::builder()
                            .status(StatusCode::OK)
                            .header("content-type", "application/grpc")
                            .header("grpc-status", status_code)
                            .header("grpc-message", err_msg)
                            .body(Body::empty())
                            .unwrap();
                        Ok(response)
                    } else {
                        let status = if err_msg.contains("deadline exceeded") {
                            StatusCode::GATEWAY_TIMEOUT
                        } else {
                            StatusCode::INTERNAL_SERVER_ERROR
                        };
                        let response = Response::builder()
                            .status(status)
                            .body(Body::from(err_msg))
                            .unwrap();
                        Ok(response)
                    }
                }
            }
        })
    }
}

pub async fn handle_http_connection<H>(
    stream: TcpStream,
    handler: Arc<H>,
    rest_routes: Vec<RestRoute>,
    is_http2: bool,
) where
    H: HttpServiceHandler,
{
    let service = HelixHttpService {
        handler,
        rest_routes,
        streaming_handler: None,
        health_checker: Some(crate::health::HealthChecker::new()),
    };
    let mut builder = Http::new();
    if is_http2 {
        builder.http2_only(true);
        builder.http2_initial_connection_window_size(Some(1024 * 1024 * 2));
        builder.http2_initial_stream_window_size(Some(1024 * 1024));
        builder.http2_max_concurrent_streams(Some(250));
    } else {
        builder.http1_only(true);
        builder.http1_keep_alive(true);
    }
    let _ = builder.serve_connection(stream, service).await;
}

/// Handle an HTTP connection with both unary and streaming support.
pub async fn handle_http_connection_streaming<H>(
    stream: TcpStream,
    handler: Arc<H>,
    rest_routes: Vec<RestRoute>,
    streaming_handler: Arc<dyn HttpStreamingHandler>,
    is_http2: bool,
) where
    H: HttpServiceHandler,
{
    let service = HelixHttpService {
        handler,
        rest_routes,
        streaming_handler: Some(streaming_handler),
        health_checker: Some(crate::health::HealthChecker::new()),
    };
    let mut builder = Http::new();
    if is_http2 {
        builder.http2_only(true);
        builder.http2_initial_connection_window_size(Some(1024 * 1024 * 2));
        builder.http2_initial_stream_window_size(Some(1024 * 1024));
        builder.http2_max_concurrent_streams(Some(250));
    } else {
        builder.http1_only(true);
        builder.http1_keep_alive(true);
    }
    let _ = builder.serve_connection(stream, service).await;
}

use tokio::sync::broadcast;

pub struct HelixServer<H> {
    addr: String,
    handler: Arc<H>,
    rest_routes: Vec<RestRoute>,
    streaming_handler: Option<Arc<dyn HttpStreamingHandler>>,
    tls_acceptor: Option<tokio_rustls::TlsAcceptor>,
    shutdown_tx: broadcast::Sender<()>,
    protocol_fallback: Option<Arc<Box<dyn Fn(tokio::net::TcpStream, crate::sniffer::Protocol) + Send + Sync>>>,
}

impl<H: HttpServiceHandler + Send + Sync + 'static> HelixServer<H> {
    pub fn new(addr: &str, handler: Arc<H>, rest_routes: Vec<RestRoute>) -> Self {
        let (tx, _) = broadcast::channel(1);
        Self {
            addr: addr.to_string(),
            handler,
            rest_routes,
            streaming_handler: None,
            tls_acceptor: None,
            shutdown_tx: tx,
            protocol_fallback: None,
        }
    }

    pub fn set_streaming_handler(&mut self, handler: Arc<dyn HttpStreamingHandler>) {
        self.streaming_handler = Some(handler);
    }

    pub fn set_tls_acceptor(&mut self, acceptor: tokio_rustls::TlsAcceptor) {
        self.tls_acceptor = Some(acceptor);
    }

    pub fn set_protocol_fallback<F>(&mut self, fallback: F) 
    where F: Fn(tokio::net::TcpStream, crate::sniffer::Protocol) + Send + Sync + 'static {
        self.protocol_fallback = Some(Arc::new(Box::new(fallback)));
    }

    pub async fn start(&self) -> std::io::Result<()> {
        let listener = tokio::net::TcpListener::bind(&self.addr).await?;
        let mut shutdown_rx = self.shutdown_tx.subscribe();

        loop {
            tokio::select! {
                Ok((stream, _)) = listener.accept() => {
                    let tls_acceptor = self.tls_acceptor.clone();
                    let handler = self.handler.clone();
                    let rest_routes = self.rest_routes.clone();
                    let streaming_handler = self.streaming_handler.clone();
                    let fallback = self.protocol_fallback.clone();
                    let mut conn_shutdown_rx = self.shutdown_tx.subscribe();
                    
                    tokio::spawn(async move {
                        let mut buf = [0u8; 8];
                        let _ = stream.peek(&mut buf).await;
                        
                        if buf[0] == 0x16 {
                            if let Some(acceptor) = tls_acceptor {
                                if let Ok(tls_stream) = acceptor.accept(stream).await {
                                    let mut http = Http::new();
                                    
                                    let (_, session) = tls_stream.get_ref();
                                    if let Some(alpn) = session.alpn_protocol() {
                                        if alpn == b"h2" {
                                            http.http2_only(true);
                                            http.http2_initial_connection_window_size(Some(1024 * 1024 * 2));
                                            http.http2_initial_stream_window_size(Some(1024 * 1024));
                                            http.http2_max_concurrent_streams(Some(250));
                                        } else {
                                            http.http1_only(true);
                                            http.http1_keep_alive(true);
                                        }
                                    } else {
                                        http.http1_only(true);
                                        http.http1_keep_alive(true);
                                    }
                                    
                                    let service = HelixHttpService {
                                        handler,
                                        rest_routes,
                                        streaming_handler,
                                        health_checker: Some(crate::health::HealthChecker::new()),
                                    };
                                    
                                    let conn = http.serve_connection(tls_stream, service);
                                    let mut conn = Box::pin(conn.with_upgrades());
                                    
                                    tokio::select! {
                                        _ = &mut conn => {}
                                        _ = conn_shutdown_rx.recv() => {
                                            conn.as_mut().graceful_shutdown();
                                            let _ = conn.await;
                                        }
                                    }
                                }
                            }
                        } else {
                            let protocol = crate::sniffer::sniff_protocol(&stream).await.unwrap_or(crate::sniffer::Protocol::Unknown);
                            if protocol == crate::sniffer::Protocol::Grpc || protocol == crate::sniffer::Protocol::Http {
                                let mut http = Http::new();
                                if protocol == crate::sniffer::Protocol::Grpc {
                                    http.http2_only(true);
                                    http.http2_initial_connection_window_size(Some(1024 * 1024 * 2));
                                    http.http2_initial_stream_window_size(Some(1024 * 1024));
                                    http.http2_max_concurrent_streams(Some(250));
                                } else {
                                    http.http1_only(true);
                                    http.http1_keep_alive(true);
                                }
                                let service = HelixHttpService {
                                    handler,
                                    rest_routes,
                                    streaming_handler,
                                    health_checker: Some(crate::health::HealthChecker::new()),
                                };
                                let conn = http.serve_connection(stream, service);
                                let mut conn = Box::pin(conn.with_upgrades());
                                tokio::select! {
                                    _ = &mut conn => {}
                                    _ = conn_shutdown_rx.recv() => {
                                        conn.as_mut().graceful_shutdown();
                                        let _ = conn.await;
                                    }
                                }
                            } else if let Some(fb) = fallback {
                                // For Thrift, run in a blocking task or let the fallback spawn one
                                fb(stream, protocol);
                            }
                        }
                    });
                }
                _ = shutdown_rx.recv() => {
                    break;
                }
            }
        }
        Ok(())
    }

    pub fn shutdown(&self) {
        let _ = self.shutdown_tx.send(());
    }
}
