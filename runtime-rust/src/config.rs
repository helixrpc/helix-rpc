use std::fs;
use std::time::{Duration, SystemTime};
use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct Config {
    pub host: String,
    pub port: u16,
    pub disable_metrics: bool,
    pub disable_health: bool,
    pub disable_gzip: bool,
    pub disable_deadline: bool,
    pub rate_limit_rate: f64,
    pub rate_limit_burst: u32,
    pub enable_jwt_auth: bool,
    pub jwt_secret: String,
    pub enable_api_key_auth: bool,
    pub api_key: String,
}

impl Default for Config {
    fn default() -> Self {
        Config {
            host: "127.0.0.1".to_string(),
            port: 8080,
            disable_metrics: false,
            disable_health: false,
            disable_gzip: false,
            disable_deadline: false,
            rate_limit_rate: 100.0,
            rate_limit_burst: 10,
            enable_jwt_auth: false,
            jwt_secret: "".to_string(),
            enable_api_key_auth: false,
            api_key: "".to_string(),
        }
    }
}

pub fn load_config(path: &str) -> Result<Config, String> {
    let content = fs::read_to_string(path).map_err(|e| e.to_string())?;
    serde_json::from_str(&content).map_err(|e| e.to_string())
}

pub fn watch_config<F>(path: String, on_change: F)
where
    F: Fn(Config) + Send + Sync + 'static,
{
    tokio::spawn(async move {
        let mut last_mod = fs::metadata(&path)
            .and_then(|m| m.modified())
            .unwrap_or_else(|_| SystemTime::now());

        loop {
            tokio::time::sleep(Duration::from_secs(2)).await;
            if let Ok(metadata) = fs::metadata(&path) {
                if let Ok(modified) = metadata.modified() {
                    if modified > last_mod {
                        last_mod = modified;
                        if let Ok(cfg) = load_config(&path) {
                            println!("🧬 [Helix] Dynamic config reload from {} succeeded.", path);
                            on_change(cfg);
                        } else {
                            eprintln!("✗ [Helix] Failed to reload config from {}", path);
                        }
                    }
                }
            }
        }
    });
}
