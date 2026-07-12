use aws_sdk_kms::Client;
use aws_sdk_kms::primitives::Blob;
use std::sync::Arc;

#[derive(Clone)]
pub struct HelixKMS {
    client: Arc<Client>,
    key_id: String,
}

impl HelixKMS {
    pub async fn new(key_id: String) -> Self {
        let config = aws_config::load_from_env().await;
        let client = Arc::new(Client::new(&config));
        Self { client, key_id }
    }

    pub async fn encrypt_payload(&self, plaintext: &[u8]) -> Result<Vec<u8>, aws_sdk_kms::Error> {
        let resp = self
            .client
            .encrypt()
            .key_id(&self.key_id)
            .plaintext(Blob::new(plaintext.to_vec()))
            .send()
            .await?;
        
        Ok(resp.ciphertext_blob().unwrap().clone().into_inner())
    }

    pub async fn decrypt_payload(&self, ciphertext: &[u8]) -> Result<Vec<u8>, aws_sdk_kms::Error> {
        let resp = self
            .client
            .decrypt()
            .key_id(&self.key_id)
            .ciphertext_blob(Blob::new(ciphertext.to_vec()))
            .send()
            .await?;
            
        Ok(resp.plaintext().unwrap().clone().into_inner())
    }
}
