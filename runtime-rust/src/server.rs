use std::future::Future;
use std::pin::Pin;
use std::sync::Arc;
use tokio::net::TcpStream;
use hyper::{Body, Request, Response, StatusCode};
use hyper::service::Service;
use hyper::server::conn::Http;
use std::task::{Context, Poll};

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

            if is_grpc {
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
                let length = u32::from_be_bytes([request_payload[1], request_payload[2], request_payload[3], request_payload[4]]) as usize;
                request_payload = request_payload[5..5+length].to_vec();
            }

            // Call the handler inside the tokio task-local metadata context scope
            let handler_fut = crate::metadata::METADATA.scope(md, async move {
                handler.handle_request(&matched_path, request_payload, is_json).await
            });

            match handler_fut.await {
                Ok((resp_bytes, resp_content_type)) => {
                    if is_grpc {
                        let mut frame = Vec::with_capacity(5 + resp_bytes.len());
                        frame.push(0); // uncompressed
                        frame.extend_from_slice(&(resp_bytes.len() as u32).to_be_bytes());
                        frame.extend_from_slice(&resp_bytes);

                        let response = Response::builder()
                            .status(StatusCode::OK)
                            .header("content-type", "application/grpc")
                            .header("grpc-status", "0") // OK
                            .body(Body::from(frame))
                            .unwrap();
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
                        let response = Response::builder()
                            .status(StatusCode::OK)
                            .header("content-type", "application/grpc")
                            .header("grpc-status", "13") // INTERNAL
                            .header("grpc-message", err_msg)
                            .body(Body::empty())
                            .unwrap();
                        Ok(response)
                    } else {
                        let response = Response::builder()
                            .status(StatusCode::INTERNAL_SERVER_ERROR)
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
    let service = HelixHttpService { handler, rest_routes };
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
