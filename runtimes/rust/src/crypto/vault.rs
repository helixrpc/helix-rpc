use reqwest::Client;
use serde::Deserialize;
use std::collections::HashMap;
use std::sync::Arc;
use std::time::Duration;
use tokio::sync::RwLock;
use tracing::{error, info};

#[derive(Clone)]
pub struct HelixVault {
    secrets: Arc<RwLock<HashMap<String, String>>>,
}

#[derive(Deserialize)]
struct VaultResponse {
    data: Option<VaultData>,
}

#[derive(Deserialize)]
struct VaultData {
    data: Option<HashMap<String, String>>,
}

impl HelixVault {
    pub fn new(vault_addr: String, token: String, secret_path: String, poll_interval: Duration) -> Self {
        let secrets = Arc::new(RwLock::new(HashMap::new()));
        let secrets_clone = secrets.clone();

        tokio::spawn(async move {
            let client = Client::new();
            loop {
                match Self::fetch_secrets(&client, &vault_addr, &token, &secret_path).await {
                    Ok(new_secrets) => {
                        let mut w = secrets_clone.write().await;
                        *w = new_secrets;
                        info!("Vault secrets reloaded successfully");
                    }
                    Err(e) => {
                        error!("Failed to fetch secrets from Vault: {:?}", e);
                    }
                }
                tokio::time::sleep(poll_interval).await;
            }
        });

        Self { secrets }
    }

    async fn fetch_secrets(
        client: &Client,
        addr: &str,
        token: &str,
        path: &str,
    ) -> Result<HashMap<String, String>, reqwest::Error> {
        let url = format!("{}/v1/{}", addr, path);
        let resp = client
            .get(&url)
            .header("X-Vault-Token", token)
            .send()
            .await?
            .error_for_status()?;

        let v: VaultResponse = resp.json().await?;
        Ok(v.data.and_then(|d| d.data).unwrap_or_default())
    }

    pub async fn get_secret(&self, key: &str) -> Option<String> {
        let r = self.secrets.read().await;
        r.get(key).cloned()
    }
}
