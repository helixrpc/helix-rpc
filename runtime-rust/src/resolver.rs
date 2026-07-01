use std::collections::HashMap;
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
