import re

with open('src/server.rs', 'r') as f:
    content = f.read()

# 1. Add HttpSseHandler trait
trait = """
/// Handler trait for Server-Sent Events (SSE).
#[async_trait::async_trait]
pub trait HttpSseHandler: Send + Sync {
    /// Return `true` if `path` should be handled as an SSE stream.
    fn is_sse(&self, path: &str) -> bool;
    /// Execute the SSE handler. Returns a channel receiver for the text events.
    async fn handle_sse(&self, path: &str, body: Vec<u8>, is_json: bool) -> Result<tokio::sync::mpsc::Receiver<Result<String, String>>, String>;
}
"""
content = content.replace(
    "    async fn handle_stream(&self, path: &str, stream: Box<dyn ServerStream>) -> Result<(), String>;\n}",
    "    async fn handle_stream(&self, path: &str, stream: Box<dyn ServerStream>) -> Result<(), String>;\n}\n" + trait
)

# 2. Add sse_handler to HelixHttpService
content = content.replace(
    "pub streaming_handler: Option<Arc<dyn HttpStreamingHandler>>,",
    "pub streaming_handler: Option<Arc<dyn HttpStreamingHandler>>,\n    pub sse_handler: Option<Arc<dyn HttpSseHandler>>,"
)

# 3. Add sse_handler to HelixServer
content = content.replace(
    "    rest_routes: Vec<RestRoute>,\n    streaming_handler: Option<Arc<dyn HttpStreamingHandler>>,\n    tls_acceptor",
    "    rest_routes: Vec<RestRoute>,\n    streaming_handler: Option<Arc<dyn HttpStreamingHandler>>,\n    sse_handler: Option<Arc<dyn HttpSseHandler>>,\n    tls_acceptor"
)
content = content.replace(
    "            streaming_handler: None,\n            tls_acceptor: None,",
    "            streaming_handler: None,\n            sse_handler: None,\n            tls_acceptor: None,"
)

# 4. Add set_sse_handler to HelixServer
set_sse = """
    pub fn set_sse_handler(&mut self, handler: Arc<dyn HttpSseHandler>) {
        self.sse_handler = Some(handler);
    }
"""
content = content.replace(
    "    pub fn set_streaming_handler(&mut self, handler: Arc<dyn HttpStreamingHandler>) {\n        self.streaming_handler = Some(handler);\n    }",
    "    pub fn set_streaming_handler(&mut self, handler: Arc<dyn HttpStreamingHandler>) {\n        self.streaming_handler = Some(handler);\n    }\n" + set_sse
)

# 5. Fix handle_http_connection instantiations
content = content.replace(
    "        streaming_handler: None,\n        health_checker: Some(crate::health::HealthChecker::new()),",
    "        streaming_handler: None,\n        sse_handler: None,\n        health_checker: Some(crate::health::HealthChecker::new()),"
)
content = content.replace(
    "        streaming_handler: Some(streaming_handler),\n        health_checker: Some(crate::health::HealthChecker::new()),",
    "        streaming_handler: Some(streaming_handler),\n        sse_handler: None,\n        health_checker: Some(crate::health::HealthChecker::new()),"
)

# 6. Fix HelixServer loop instantiations
content = content.replace(
    "let streaming_handler = self.streaming_handler.clone();",
    "let streaming_handler = self.streaming_handler.clone();\n                    let sse_handler = self.sse_handler.clone();"
)
content = content.replace(
    "                                        streaming_handler,\n                                        health_checker: Some(crate::health::HealthChecker::new()),",
    "                                        streaming_handler,\n                                        sse_handler: sse_handler.clone(),\n                                        health_checker: Some(crate::health::HealthChecker::new()),"
)
content = content.replace(
    "                                    streaming_handler,\n                                    health_checker: Some(crate::health::HealthChecker::new()),",
    "                                    streaming_handler,\n                                    sse_handler: sse_handler.clone(),\n                                    health_checker: Some(crate::health::HealthChecker::new()),"
)

# 7. Add SSE routing logic before if is_grpc
sse_logic = """
            // --- SSE dispatch ---
            let accepts_sse = req.headers()
                .get(hyper::header::ACCEPT)
                .map(|v| v.as_bytes() == b"text/event-stream")
                .unwrap_or(false);

            if accepts_sse {
                if let Some(ref sse_h) = self.sse_handler {
                    if sse_h.is_sse(&matched_path) {
                        let sse_h_clone = sse_h.clone();
                        let matched_path_clone = matched_path.clone();
                        
                        match sse_h_clone.handle_sse(&matched_path_clone, request_payload, is_json).await {
                            Ok(mut rx) => {
                                let (mut body_tx, body_rx) = Body::channel();
                                tokio::spawn(async move {
                                    while let Some(msg) = rx.recv().await {
                                        match msg {
                                            Ok(text) => {
                                                let mut escaped_text = text.replace('\\\\', "\\\\\\\\").replace('"', "\\\\\"").replace('\\n', "\\\\n");
                                                let json_data = format!("{{\\"choices\\":[{{\\"delta\\":{{\\"content\\":\\"{}\\"}}}}]}}", escaped_text);
                                                let sse_msg = format!("data: {}\\n\\n", json_data);
                                                
                                                if body_tx.send_data(hyper::body::Bytes::from(sse_msg)).await.is_err() {
                                                    break;
                                                }
                                            }
                                            Err(e) => {
                                                let err_msg = format!("data: {{\\"error\\": \\"{}\\"}}\\n\\n", e.replace('"', "\\\\\""));
                                                let _ = body_tx.send_data(hyper::body::Bytes::from(err_msg)).await;
                                                break;
                                            }
                                        }
                                    }
                                    let _ = body_tx.send_data(hyper::body::Bytes::from("data: [DONE]\\n\\n"));
                                });

                                let response = Response::builder()
                                    .status(StatusCode::OK)
                                    .header("content-type", "text/event-stream")
                                    .header("cache-control", "no-cache")
                                    .header("connection", "keep-alive")
                                    .body(body_rx)
                                    .unwrap();
                                return Ok(response);
                            }
                            Err(e) => {
                                let response = Response::builder()
                                    .status(StatusCode::INTERNAL_SERVER_ERROR)
                                    .body(Body::from(e))
                                    .unwrap();
                                return Ok(response);
                            }
                        }
                    }
                }
            }

            // --- Streaming dispatch ---
"""
content = content.replace("            // --- Streaming dispatch ---", sse_logic)

with open('src/server.rs', 'w') as f:
    f.write(content)
