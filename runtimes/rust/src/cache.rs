use memcache_async::ascii::Protocol;
use tokio::net::TcpStream;
use sha2::{Sha256, Digest};
use std::sync::Arc;
use tokio::sync::Mutex;
use tokio_util::compat::{TokioAsyncReadCompatExt, Compat};

pub struct CacheInterceptor {
    client: Arc<Mutex<Protocol<Compat<TcpStream>>>>,
    ttl: u32,
}

impl CacheInterceptor {
    pub async fn new(server: &str, ttl: u32) -> Result<Self, Box<dyn std::error::Error + Send + Sync>> {
        let stream = TcpStream::connect(server).await?;
        let client = Protocol::new(stream.compat());
        Ok(Self {
            client: Arc::new(Mutex::new(client)),
            ttl,
        })
    }

    pub fn generate_cache_key(&self, method: &str, payload: &[u8]) -> String {
        Self::generate_key(method, payload)
    }

    pub fn generate_key(method: &str, payload: &[u8]) -> String {
        let mut hasher = Sha256::new();
        hasher.update(method.as_bytes());
        hasher.update(payload);
        hex::encode(hasher.finalize())
    }

    pub async fn get(&self, key: &str) -> Option<Vec<u8>> {
        let mut client = self.client.lock().await;
        // This is a guess of the API, let's see what cargo check says
        client.get(key).await.ok()
    }

    pub async fn set(&self, key: String, payload: Vec<u8>) {
        let client = self.client.clone();
        let ttl = self.ttl;
        tokio::spawn(async move {
            let mut client = client.lock().await;
            let _ = client.set(&key, &payload, ttl).await;
        });
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_generate_key() {
        let key = CacheInterceptor::generate_key("GET", b"test_payload");
        assert_eq!(key.len(), 64); // SHA256 hex is 64 chars
    }
}
