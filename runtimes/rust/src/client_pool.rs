use std::collections::VecDeque;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex};
use tokio::net::TcpStream;
#[cfg(unix)]
use tokio::net::UnixStream;
use tokio::io::{AsyncRead, AsyncWrite};
use std::pin::Pin;

pub enum AnyStream {
    Tcp(TcpStream),
    #[cfg(unix)]
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
            #[cfg(unix)]
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
            #[cfg(unix)]
            AnyStream::Unix(s) => Pin::new(s).poll_write(cx, buf),
        }
    }
    fn poll_flush(
        self: Pin<&mut Self>,
        cx: &mut std::task::Context<'_>,
    ) -> std::task::Poll<std::io::Result<()>> {
        match self.get_mut() {
            AnyStream::Tcp(s) => Pin::new(s).poll_flush(cx),
            #[cfg(unix)]
            AnyStream::Unix(s) => Pin::new(s).poll_flush(cx),
        }
    }
    fn poll_shutdown(
        self: Pin<&mut Self>,
        cx: &mut std::task::Context<'_>,
    ) -> std::task::Poll<std::io::Result<()>> {
        match self.get_mut() {
            AnyStream::Tcp(s) => Pin::new(s).poll_shutdown(cx),
            #[cfg(unix)]
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
        #[cfg(unix)]
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

pub struct ConsistentHashBalancer {
    replicas: usize,
    inner: Arc<Mutex<ConsistentHashInner>>,
}

struct ConsistentHashInner {
    ring: Vec<u64>,
    hash_map: std::collections::HashMap<u64, String>,
    registered: std::collections::HashSet<String>,
}

impl ConsistentHashBalancer {
    pub fn new(replicas: usize) -> Self {
        let replicas = if replicas == 0 { 50 } else { replicas };
        ConsistentHashBalancer {
            replicas,
            inner: Arc::new(Mutex::new(ConsistentHashInner {
                ring: Vec::new(),
                hash_map: std::collections::HashMap::new(),
                registered: std::collections::HashSet::new(),
            })),
        }
    }

    fn get_hash(key: &str) -> u64 {
        use std::hash::{Hash, Hasher};
        let mut s = std::collections::hash_map::DefaultHasher::new();
        key.hash(&mut s);
        s.finish()
    }

    pub fn next_with_key(&self, targets: &[String], key: &str) -> Result<String, String> {
        if targets.is_empty() {
            return Err("no targets available for load balancing".to_string());
        }

        let mut guard = self.inner.lock().unwrap();
        let mut needs_sort = false;

        for target in targets {
            if !guard.registered.contains(target) {
                guard.registered.insert(target.clone());
                for i in 0..self.replicas {
                    let virtual_node_key = format!("{}#{}", target, i);
                    let hash = Self::get_hash(&virtual_node_key);
                    guard.ring.push(hash);
                    guard.hash_map.insert(hash, target.clone());
                }
                needs_sort = true;
            }
        }

        if needs_sort {
            guard.ring.sort_unstable();
        }

        if guard.ring.is_empty() {
            return Ok(targets[0].clone());
        }

        let key_hash = Self::get_hash(key);
        let idx = match guard.ring.binary_search(&key_hash) {
            Ok(i) => i,
            Err(i) => i,
        };

        let target_set: std::collections::HashSet<&String> = targets.iter().collect();
        let start_idx = if idx >= guard.ring.len() { 0 } else { idx };
        let mut curr_idx = start_idx;

        loop {
            let hash_val = guard.ring[curr_idx];
            if let Some(node) = guard.hash_map.get(&hash_val) {
                if target_set.contains(node) {
                    return Ok(node.clone());
                }
            }
            curr_idx = (curr_idx + 1) % guard.ring.len();
            if curr_idx == start_idx {
                break;
            }
        }

        Ok(targets[0].clone())
    }
}

impl Balancer for ConsistentHashBalancer {
    fn next(&self, targets: &[String]) -> Result<String, String> {
        self.next_with_key(targets, "default-key")
    }
}

