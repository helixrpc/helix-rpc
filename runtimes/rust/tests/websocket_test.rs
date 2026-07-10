use helix_rt::server::{HelixHttpService, HttpStreamingHandler, HttpServiceHandler, ServerStream};
use futures_util::{SinkExt, StreamExt};
use std::sync::Arc;
use tokio::net::TcpListener;
use tokio_tungstenite::connect_async;
use tokio_tungstenite::tungstenite::Message;
use hyper::{Server, Request, Response, Body, StatusCode};
use hyper::service::make_service_fn;
use std::convert::Infallible;

struct MockServiceHandler;

#[async_trait::async_trait]
impl HttpServiceHandler for MockServiceHandler {
    async fn handle_request(&self, _path: &str, _req: Vec<u8>, _is_json: bool) -> Result<(Vec<u8>, String), String> {
        Ok((vec![], "".to_string()))
    }
}

struct MockStreamingHandler;

#[async_trait::async_trait]
impl HttpStreamingHandler for MockStreamingHandler {
    fn is_streaming(&self, path: &str) -> bool {
        path == "/v1.TestService/StreamEcho"
    }

    async fn handle_stream(
        &self,
        _path: &str,
        mut stream: Box<dyn ServerStream>,
    ) -> Result<(), String> {
        while let Ok(Some(bytes)) = stream.recv().await {
            // Echo back "echo: " + payload
            let str_val = String::from_utf8(bytes.to_vec()).unwrap_or_default();
            let response = format!("echo: {}", str_val);
            let _ = stream.send(response.into_bytes()).await;
        }
        Ok(())
    }
}

#[tokio::test]
async fn test_websocket_stream() {
    let service = HelixHttpService {
        handler: Arc::new(MockServiceHandler),
        rest_routes: vec![],
        streaming_handler: Some(Arc::new(MockStreamingHandler)),
        sse_handler: None,
        health_checker: None,
        disable_metrics: true,
    };

    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();

    let make_svc = make_service_fn(move |_conn| {
        let svc = HelixHttpService {
            handler: service.handler.clone(),
            rest_routes: service.rest_routes.clone(),
            streaming_handler: service.streaming_handler.clone(),
            sse_handler: service.sse_handler.clone(),
            health_checker: service.health_checker.clone(),
            disable_metrics: service.disable_metrics,
        };
        async move { Ok::<_, Infallible>(svc) }
    });

    let server = Server::builder(hyper::server::accept::from_stream(
        tokio_stream::wrappers::TcpListenerStream::new(listener),
    ))
    .serve(make_svc);

    tokio::spawn(async move {
        let _ = server.await;
    });

    // Wait a bit for the server to start
    tokio::time::sleep(tokio::time::Duration::from_millis(100)).await;

    // Connect with WebSocket
    let ws_url = format!("ws://{}/v1.TestService/StreamEcho", addr);
    let (mut ws_stream, _) = connect_async(&ws_url).await.expect("Failed to connect");

    // Send a message
    ws_stream.send(Message::Text("hello".to_string())).await.unwrap();

    // Read the echoed message
    if let Some(Ok(msg)) = ws_stream.next().await {
        assert_eq!(msg.into_text().unwrap(), "echo: hello");
    } else {
        panic!("Did not receive response");
    }
}
