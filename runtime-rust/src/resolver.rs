use std::collections::HashMap;
use std::net::ToSocketAddrs;
use std::sync::{Arc, Mutex};

pub trait Resolver: Send + Sync + 'static {
    fn resolve(&self, service_name: &str) -> Result<Vec<String>, String>;
}

#[derive(Clone)]
pub struct StaticResolver {
    targets: Arc<Mutex<HashMap<String, Vec<String>>>>,
}

impl Default for StaticResolver {
    fn default() -> Self {
        Self::new()
    }
}

impl StaticResolver {
    pub fn new() -> Self {
        StaticResolver {
            targets: Arc::new(Mutex::new(HashMap::new())),
        }
    }

    pub fn register(&self, service_name: &str, addresses: Vec<String>) {
        let mut guard = self.targets.lock().unwrap();
        guard.insert(service_name.to_string(), addresses);
    }
}

impl Resolver for StaticResolver {
    fn resolve(&self, service_name: &str) -> Result<Vec<String>, String> {
        let guard = self.targets.lock().unwrap();
        guard
            .get(service_name)
            .cloned()
            .ok_or_else(|| format!("service {} not found in resolver targets", service_name))
    }
}

pub struct DnsResolver;

impl Default for DnsResolver {
    fn default() -> Self {
        Self::new()
    }
}

impl DnsResolver {
    pub fn new() -> Self {
        DnsResolver
    }
}

impl Resolver for DnsResolver {
    fn resolve(&self, service_name: &str) -> Result<Vec<String>, String> {
        let lookup_target = if service_name.contains(':') {
            service_name.to_string()
        } else {
            format!("{}:8080", service_name)
        };

        match lookup_target.to_socket_addrs() {
            Ok(addrs) => {
                let ips: Vec<String> = addrs.map(|addr| addr.to_string()).collect();
                if ips.is_empty() {
                    Err(format!("No IP addresses resolved for {}", service_name))
                } else {
                    Ok(ips)
                }
            }
            Err(e) => Err(e.to_string()),
        }
    }
}
