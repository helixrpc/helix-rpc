use log::{debug, info};
use proxy_wasm::traits::*;
use proxy_wasm::types::*;

proxy_wasm::main! {{
    proxy_wasm::set_log_level(LogLevel::Trace);
    proxy_wasm::set_root_context(|_| -> Box<dyn RootContext> {
        Box::new(HelixRootContext)
    });
}}

struct HelixRootContext;

impl Context for HelixRootContext {}

impl RootContext for HelixRootContext {
    fn on_vm_start(&mut self, _vm_configuration_size: usize) -> bool {
        info!("Helix RPC Envoy Wasm Filter: VM started");
        true
    }

    fn create_http_context(&self, context_id: u32) -> Option<Box<dyn HttpContext>> {
        debug!("Helix RPC Envoy Wasm Filter: created HTTP context {}", context_id);
        Some(Box::new(HelixHttpContext { context_id }))
    }
}

struct HelixHttpContext {
    context_id: u32,
}

impl Context for HelixHttpContext {}

impl HttpContext for HelixHttpContext {
    fn on_http_request_headers(&mut self, _: usize, _: bool) -> Action {
        let path = self.get_http_request_header(":path").unwrap_or_default();
        let content_type = self.get_http_request_header("content-type").unwrap_or_default();
        let upgrade = self.get_http_request_header("upgrade").unwrap_or_default();
        let accept = self.get_http_request_header("accept").unwrap_or_default();

        let mut helix_protocol = "json"; // Default fallback

        if upgrade.to_lowercase() == "websocket" {
            helix_protocol = "websocket";
        } else if content_type.starts_with("application/grpc") {
            if content_type.contains("+flatbuffers") {
                helix_protocol = "grpc-flatbuffers";
            } else {
                helix_protocol = "grpc";
            }
        } else if content_type.starts_with("application/x-flatbuffers") {
            helix_protocol = "flatbuffers";
        } else if accept.contains("text/event-stream") || content_type.contains("text/event-stream") {
            helix_protocol = "sse";
        }

        debug!(
            "#{}: Intercepted request to {}. Detected Helix Protocol: {}",
            self.context_id, path, helix_protocol
        );

        // Inject the internal classification header for downstream mesh telemetry or routing
        self.set_http_request_header("x-helix-protocol", Some(helix_protocol));

        // Let the request continue
        Action::Continue
    }

    fn on_http_response_headers(&mut self, _: usize, _: bool) -> Action {
        Action::Continue
    }

    fn on_log(&mut self) {
        let path = self.get_http_request_header(":path").unwrap_or_default();
        let status = self.get_http_response_header(":status").unwrap_or_default();
        let protocol = self.get_http_request_header("x-helix-protocol").unwrap_or_default();

        info!(
            "#{}: Helix Request Completed | Path: {} | Protocol: {} | Status: {}",
            self.context_id, path, protocol, status
        );
    }
}
