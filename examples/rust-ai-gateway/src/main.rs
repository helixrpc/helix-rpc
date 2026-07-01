use async_trait::async_trait;
use helix_rt::server::{HelixServer, HttpServiceHandler, HttpSseHandler, RestRoute};
use helix_rt::pyo3_runner::PyModelHandler;
use std::sync::Arc;
use tokio::time::Duration;

struct ChatAppHandler;
#[async_trait]
impl HttpServiceHandler for ChatAppHandler {
    async fn handle_request(
        &self,
        path: &str,
        _body: Vec<u8>,
        _is_json: bool,
    ) -> Result<(Vec<u8>, String), String> {
        if path == "/" || path == "/index.html" {
            // Serve the frontend UI!
            let html = std::fs::read_to_string("../frontend-chat-ui/index.html").unwrap_or_else(|_| "<h1>Frontend not found</h1>".to_string());
            Ok((html.into_bytes(), "text/html".to_string()))
        } else if path == "/index.css" {
            let css = std::fs::read_to_string("../frontend-chat-ui/index.css").unwrap_or_default();
            Ok((css.into_bytes(), "text/css".to_string()))
        } else if path == "/app.js" {
            let js = std::fs::read_to_string("../frontend-chat-ui/app.js").unwrap_or_default();
            Ok((js.into_bytes(), "application/javascript".to_string()))
        } else {
            Ok((b"{\"error\": \"Not Found\"}".to_vec(), "application/json".to_string()))
        }
    }
}

struct StreamSseHandler {
    model: Arc<PyModelHandler>,
}

#[async_trait]
impl HttpSseHandler for StreamSseHandler {
    fn is_sse(&self, path: &str) -> bool {
        path == "/v1/chat/completions"
    }

    async fn handle_sse(
        &self,
        _path: &str,
        body: Vec<u8>,
        _is_json: bool,
    ) -> Result<tokio::sync::mpsc::Receiver<Result<String, String>>, String> {
        // Parse the JSON body to extract the prompt, default if not provided
        let mut prompt = "Explain quantum computing in simple terms.".to_string();
        if let Ok(json_body) = serde_json::from_slice::<serde_json::Value>(&body) {
            if let Some(messages) = json_body.get("messages").and_then(|m| m.as_array()) {
                if let Some(last_message) = messages.last() {
                    if let Some(content) = last_message.get("content").and_then(|c| c.as_str()) {
                        prompt = content.to_string();
                    }
                }
            }
        }
        
        let (tx, rx) = tokio::sync::mpsc::channel(64);
        
        let model_clone = self.model.clone();
        
        // Use tokio::task::spawn_blocking wrapper which is inside generate_stream
        model_clone.generate_stream(prompt, tx);
        
        Ok(rx)
    }
}

#[tokio::main]
async fn main() {
    println!("Initializing Zero-Serialization PyO3 Streaming Server...");

    let python_path = ".";
    let module_name = "model";
    let class_name = "DummyModel";

    let py_handler = match PyModelHandler::new(python_path, module_name, class_name) {
        Ok(h) => Arc::new(h),
        Err(e) => {
            eprintln!("Failed to load python model: {}", e);
            std::process::exit(1);
        }
    };

    println!("Successfully embedded Python interpreter and instantiated DummyModel!");

    let mut server = HelixServer::new(
        "127.0.0.1:8081",
        Arc::new(ChatAppHandler),
        vec![
            RestRoute::new("GET", "/", "/"),
            RestRoute::new("GET", "/index.html", "/index.html"),
            RestRoute::new("GET", "/index.css", "/index.css"),
            RestRoute::new("GET", "/app.js", "/app.js"),
            RestRoute::new("POST", "/v1/chat/completions", "/v1/chat/completions"),
        ],
    );
    
    server.set_sse_handler(Arc::new(StreamSseHandler { model: py_handler }));

    println!("Starting Rust API Gateway + UI Server on http://127.0.0.1:8081...");
    
    // Start server, blocking forever
    if let Err(e) = server.start().await {
        eprintln!("Server error: {}", e);
    }
}
