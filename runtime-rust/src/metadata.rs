use std::collections::HashMap;
use tokio::task_local;

task_local! {
    pub static METADATA: HashMap<String, Vec<String>>;
}

/// Retrieves a value from the task-local metadata context.
pub fn get_metadata(key: &str) -> Option<Vec<String>> {
    METADATA.try_with(|m| m.get(&key.to_lowercase()).cloned()).ok().flatten()
}
