use std::collections::VecDeque;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex};
use tokio::net::{TcpStream, UnixStream};
use tokio::io::{AsyncRead, AsyncWrite};
use std::pin::Pin;

pub enum AnyStream {
    Tcp(TcpStream),
    Unix(UnixStream),
}

impl AsyncRead for AnyStream {
    fn poll_read(
        self: Pin<&mut Self>,
        cx: &mut std::task::Context<'_>,
        buf: &mut tokio::io::ReadBuf<'_>,
    ) -> std::task::Poll<std::io::Result<()>> {
        match self.get_mut() {
            AnyStream::Tcp(s) => Pin::new(s).poll_read(cx, buf),
            AnyStream::Unix(s) => Pin::new(s).poll_read(cx, buf),
        }
    }
}

impl AsyncWrite for AnyStream {
    fn poll_write(
        self: Pin<&mut Self>,
        cx: &mut std::task::Context<'_>,
        buf: &[u8],
    ) -> std::task::Poll<std::io::Result<usize>> {
        match self.get_mut() {
            AnyStream::Tcp(s) => Pin::new(s).poll_write(cx, buf),
            AnyStream::Unix(s) => Pin::new(s).poll_write(cx, buf),
        }
    }
    fn poll_flush(
        self: Pin<&mut Self>,
        cx: &mut std::task::Context<'_>,
    ) -> std::task::Poll<std::io::Result<()>> {
        match self.get_mut() {
            AnyStream::Tcp(s) => Pin::new(s).poll_flush(cx),
            AnyStream::Unix(s) => Pin::new(s).poll_flush(cx),
        }
    }
    fn poll_shutdown(
        self: Pin<&mut Self>,
        cx: &mut std::task::Context<'_>,
    ) -> std::task::Poll<std::io::Result<()>> {
        match self.get_mut() {
            AnyStream::Tcp(s) => Pin::new(s).poll_shutdown(cx),
            AnyStream::Unix(s) => Pin::new(s).poll_shutdown(cx),
        }
    }
}

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

    pub async fn get(&self) -> Result<AnyStream, std::io::Error> {
        if crate::ebpf::has_unix_prefix(&self.addr) {
            let path = crate::ebpf::strip_unix_prefix(&self.addr);
            let stream = UnixStream::connect(path).await?;
            return Ok(AnyStream::Unix(stream));
        }
        let stream = {
            let mut guard = self.conns.lock().unwrap();
            guard.pop_front()
        };
        if let Some(conn) = stream {
            Ok(AnyStream::Tcp(conn))
        } else {
            let stream = TcpStream::connect(&self.addr).await?;
            Ok(AnyStream::Tcp(stream))
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
