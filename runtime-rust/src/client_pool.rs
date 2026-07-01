use std::collections::VecDeque;
use std::sync::{Arc, Mutex};
use std::sync::atomic::{AtomicU64, Ordering};
use tokio::net::TcpStream;

pub struct ClientConnPool {
    addr: String,
    conns: Arc<Mutex<VecDeque<TcpStream>>>,
}

impl ClientConnPool {
    pub fn new(addr: &str) -> Self {
        ClientConnPool {
            addr: addr.to_string(),
            conns: Arc::new(Mutex::new(VecDeque::new())),
        }
    }

    pub async fn get(&self) -> Result<TcpStream, std::io::Error> {
        let stream = {
            let mut guard = self.conns.lock().unwrap();
            guard.pop_front()
        };
        if let Some(conn) = stream {
            Ok(conn)
        } else {
            TcpStream::connect(&self.addr).await
        }
    }

    pub fn put(&self, stream: TcpStream) {
        let mut guard = self.conns.lock().unwrap();
        // Set pool size limit to 32 idle connections
        if guard.len() < 32 {
            guard.push_back(stream);
        }
    }
}

pub trait Balancer: Send + Sync + 'static {
    fn next(&self, targets: &[String]) -> Result<String, String>;
}

pub struct RoundRobinBalancer {
    counter: AtomicU64,
}

impl Default for RoundRobinBalancer {
    fn default() -> Self {
        Self::new()
    }
}

impl RoundRobinBalancer {
    pub fn new() -> Self {
        RoundRobinBalancer {
            counter: AtomicU64::new(0),
        }
    }
}

impl Balancer for RoundRobinBalancer {
    fn next(&self, targets: &[String]) -> Result<String, String> {
        if targets.is_empty() {
            return Err("no targets available for load balancing".to_string());
        }
        let val = self.counter.fetch_add(1, Ordering::SeqCst);
        let idx = (val % (targets.len() as u64)) as usize;
        Ok(targets[idx].clone())
    }
}
