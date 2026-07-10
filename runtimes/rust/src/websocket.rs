use crate::server::ServerStream;
use futures_util::{SinkExt, StreamExt};
use hyper_tungstenite::WebSocketStream;
use hyper::upgrade::Upgraded;
use tokio::sync::Mutex;
use hyper_tungstenite::tungstenite::Message;

pub struct WsServerStream {
    ws: Mutex<WebSocketStream<Upgraded>>,
}

impl WsServerStream {
    pub fn new(ws: WebSocketStream<Upgraded>) -> Self {
        Self { ws: Mutex::new(ws) }
    }
}

#[async_trait::async_trait]
impl ServerStream for WsServerStream {
    async fn recv(&mut self) -> Result<Option<bytes::Bytes>, String> {
        let mut ws = self.ws.lock().await;
        loop {
            match ws.next().await {
                Some(Ok(msg)) => {
                    if msg.is_text() || msg.is_binary() {
                        return Ok(Some(bytes::Bytes::from(msg.into_data())));
                    } else if msg.is_close() {
                        return Ok(None);
                    }
                    // ignore Ping/Pong and continue looping
                }
                Some(Err(e)) => return Err(e.to_string()),
                None => return Ok(None),
            }
        }
    }

    async fn send(&self, payload: Vec<u8>) -> Result<(), String> {
        let mut ws = self.ws.lock().await;
        // If it's valid UTF-8, send as Text; otherwise, Binary
        let msg = match std::str::from_utf8(&payload) {
            Ok(s) => Message::Text(s.to_string()),
            Err(_) => Message::Binary(payload),
        };
        ws.send(msg).await.map_err(|e| e.to_string())
    }
}
