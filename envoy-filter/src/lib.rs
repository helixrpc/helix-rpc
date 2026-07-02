use log::info;
use proxy_wasm::traits::*;
use proxy_wasm::types::*;
use std::time::SystemTime;

proxy_wasm::main!({{
    proxy_wasm::set_log_level(LogLevel::Info);
    proxy_wasm::set_root_context(|_| -> Box<dyn RootContext> {
        Box::new(HelixRootContext)
    });
}});

struct HelixRootContext;

impl Context for HelixRootContext {}

impl RootContext for HelixRootContext {
    fn create_http_context(&self, _context_id: u32) -> Option<Box<dyn HttpContext>> {
        Some(Box::new(HelixHttpContext))
    }

    fn on_vm_start(&mut self, _vm_configuration_size: usize) -> bool {
        info!("Helix VM started successfully.");
        true
    }
}

struct HelixHttpContext;

impl Context for HelixHttpContext {}

impl HttpContext for HelixHttpContext {
    fn on_http_request_headers(&mut self, _num_headers: usize, _end_of_stream: bool) -> Action {
        info!("Intercepted HTTP request headers in Helix Wasm Filter.");

        // 1. Inspect API Key
        if let Some(api_key) = self.get_http_request_header("x-api-key") {
            if api_key == "blocked-token" {
                info!("Helix Filter: Request blocked due to invalid api-key.");
                self.send_http_response(
                    401,
                    vec![("content-type", "application/json")],
                    Some(b"{\"error\":\"Unauthorized API Key\"}"),
                );
                return Action::Pause;
            }
        }

        // 2. Deadline Propagation Header Check
        if let Some(timeout) = self.get_http_request_header("grpc-timeout") {
            info!("Helix Filter: Extracted incoming request deadline: {}", timeout);
        }

        // 3. Trace Context Propagation
        let trace_id = match self.get_http_request_header("x-trace-id") {
            Some(tid) => tid,
            None => {
                let now = self.get_current_time()
                    .duration_since(SystemTime::UNIX_EPOCH)
                    .map(|d| d.as_nanos())
                    .unwrap_or(0);
                let generated = format!("trace-{}", now);
                self.set_http_request_header("x-trace-id", Some(&generated));
                info!("Helix Filter: Generated and injected x-trace-id: {}", generated);
                generated
            }
        };

        info!("Helix Filter: Request routed with Trace ID: {}", trace_id);
        Action::Continue
    }
}
