use async_trait::async_trait;
use helix_rt::server::{HelixServer, HttpServiceHandler, HttpSseHandler, RestRoute};
use helix_rt::pyo3_runner::PyModelHandler;
use std::sync::Arc;
use tokio::time::Duration;

struct DummyHttpHandler;
#[async_trait]
impl HttpServiceHandler for DummyHttpHandler {
    async fn handle_request(
        &self,
        _path: &str,
        _body: Vec<u8>,
        _is_json: bool,
    ) -> Result<(Vec<u8>, String), String> {
        Ok((b"{}".to_vec(), "application/json".to_string()))
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
        _body: Vec<u8>,
        _is_json: bool,
    ) -> Result<tokio::sync::mpsc::Receiver<Result<String, String>>, String> {
        // We will simulate parsing the body to get the prompt
        let prompt = "Explain quantum computing in simple terms.".to_string();
        
        let (tx, rx) = tokio::sync::mpsc::channel(64);
        
        let model_clone = self.model.clone();
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
        Arc::new(DummyHttpHandler),
        vec![RestRoute::new("POST", "/v1/chat/completions", "/v1/chat/completions")],
    );
    
    server.set_sse_handler(Arc::new(StreamSseHandler { model: py_handler }));

    println!("Starting Rust API Gateway on http://127.0.0.1:8081...");
    
    // Start server in background
    tokio::spawn(async move {
        if let Err(e) = server.start().await {
            eprintln!("Server error: {}", e);
        }
    });

    // Wait for server to boot
    tokio::time::sleep(Duration::from_millis(500)).await;
    
    println!("Simulating REST Client connecting via Server-Sent Events (SSE)...");
    
    // Use curl to stream the SSE response directly
    let status = std::process::Command::new("curl")
        .arg("-N") // no buffer
        .arg("-s") // silent
        .arg("-H")
        .arg("Accept: text/event-stream")
        .arg("-X")
        .arg("POST")
        .arg("http://127.0.0.1:8081/v1/chat/completions")
        .status()
        .expect("Failed to execute curl");

    if status.success() {
        println!("\nStreaming test completed successfully!");
    } else {
        eprintln!("\nStreaming test failed!");
        std::process::exit(1);
    }
}
