use std::collections::HashMap;
use std::sync::{Arc, RwLock};
use std::time::Duration;

lazy_static::lazy_static! {
    pub static ref GLOBAL_METRICS: Arc<MetricsCollector> = Arc::new(MetricsCollector::new());
}

pub struct MetricsCollector {
    // key: (method, path, status) -> count
    requests: RwLock<HashMap<(String, String, u16), u64>>,
    // key: (method, path) -> count
    errors: RwLock<HashMap<(String, String), u64>>,
    // key: (method, path) -> list of durations in seconds
    durations: RwLock<HashMap<(String, String), Vec<f64>>>,
}

impl MetricsCollector {
    pub fn new() -> Self {
        MetricsCollector {
            requests: RwLock::new(HashMap::new()),
            errors: RwLock::new(HashMap::new()),
            durations: RwLock::new(HashMap::new()),
        }
    }

    pub fn record(&self, method: &str, path: &str, status: u16, duration: Duration) {
        let dur_secs = duration.as_secs_f64();
        
        let mut reqs = self.requests.write().unwrap();
        *reqs.entry((method.to_string(), path.to_string(), status)).or_insert(0) += 1;

        let mut durs = self.durations.write().unwrap();
        durs.entry((method.to_string(), path.to_string())).or_default().push(dur_secs);

        if status >= 400 {
            let mut errs = self.errors.write().unwrap();
            *errs.entry((method.to_string(), path.to_string())).or_insert(0) += 1;
        }
    }

    pub fn format_prometheus(&self) -> String {
        let mut lines = Vec::new();

        lines.push("# HELP helix_requests_total Total number of RPC requests.".to_string());
        lines.push("# TYPE helix_requests_total counter".to_string());
        {
            let reqs = self.requests.read().unwrap();
            for ((method, path, status), count) in reqs.iter() {
                lines.push(format!(
                    "helix_requests_total{{method=\"{}\",path=\"{}\",status=\"{}\"}} {}",
                    method, path, status, count
                ));
            }
        }

        lines.push("".to_string());
        lines.push("# HELP helix_errors_total Total number of RPC errors.".to_string());
        lines.push("# TYPE helix_errors_total counter".to_string());
        {
            let errs = self.errors.read().unwrap();
            for ((method, path), count) in errs.iter() {
                lines.push(format!(
                    "helix_errors_total{{method=\"{}\",path=\"{}\"}} {}",
                    method, path, count
                ));
            }
        }

        let buckets = vec![0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0];
        lines.push("".to_string());
        lines.push("# HELP helix_request_duration_seconds Request latency histogram.".to_string());
        lines.push("# TYPE helix_request_duration_seconds histogram".to_string());
        {
            let durs = self.durations.read().unwrap();
            for ((method, path), durations) in durs.iter() {
                let total: f64 = durations.iter().sum();
                let count = durations.len();
                
                for &le in &buckets {
                    let bucket_count = durations.iter().filter(|&&d| d <= le).count();
                    lines.push(format!(
                        "helix_request_duration_seconds_bucket{{method=\"{}\",path=\"{}\",le=\"{}\"}} {}",
                        method, path, le, bucket_count
                    ));
                }
                lines.push(format!(
                    "helix_request_duration_seconds_bucket{{method=\"{}\",path=\"{}\",le=\"+Inf\"}} {}",
                    method, path, count
                ));
                lines.push(format!(
                    "helix_request_duration_seconds_sum{{method=\"{}\",path=\"{}\"}} {:.6}",
                    method, path, total
                ));
                lines.push(format!(
                    "helix_request_duration_seconds_count{{method=\"{}\",path=\"{}\"}} {}",
                    method, path, count
                ));
            }
        }

        lines.join("\n") + "\n"
    }
}

impl Default for MetricsCollector {
    fn default() -> Self {
        Self::new()
    }
}
