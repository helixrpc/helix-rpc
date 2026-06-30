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

pub struct HelixHttpService<H> {
    pub handler: Arc<H>,
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
        Box::pin(async move {
            let path = req.uri().path().to_string();
            let content_type = req.headers()
                .get("content-type")
                .and_then(|v| v.to_str().ok())
                .unwrap_or("");

            let is_json = content_type == "application/json";
            let is_grpc = content_type == "application/grpc";

            if !is_json && !is_grpc {
                let response = Response::builder()
                    .status(StatusCode::BAD_REQUEST)
                    .body(Body::from("Unsupported Content-Type"))
                    .unwrap();
                return Ok(response);
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

            // Call the handler
            match handler.handle_request(&path, request_payload, is_json).await {
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

pub async fn handle_http_connection<H>(stream: TcpStream, handler: Arc<H>, is_http2: bool)
where
    H: HttpServiceHandler,
{
    let service = HelixHttpService { handler };
    let mut builder = Http::new();
    if is_http2 {
        builder.http2_only(true);
    } else {
        builder.http1_only(true);
    }
    let _ = builder.serve_connection(stream, service).await;
}
