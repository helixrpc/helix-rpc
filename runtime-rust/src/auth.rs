use crate::errors::{ErrorCode, HelixError};
use hyper::{header::HeaderMap, Request};
use jsonwebtoken::{decode, Algorithm, DecodingKey, Validation};
use serde_json::Value;
use std::collections::HashMap;
use tokio::task_local;

task_local! {
    pub static JWT_CLAIMS: HashMap<String, Value>;
    pub static API_KEY_PRINCIPAL: String;
}

/// Retrieves the current JWT claims from the task-local context.
pub fn get_jwt_claims() -> Option<HashMap<String, Value>> {
    JWT_CLAIMS.try_with(|c| c.clone()).ok()
}

/// Retrieves the current API key principal from the task-local context.
pub fn get_api_key_principal() -> Option<String> {
    API_KEY_PRINCIPAL.try_with(|p| p.clone()).ok()
}

pub struct JwtValidator {
    decoding_key: DecodingKey,
    validation: Validation,
    required_claims: Vec<String>,
}

impl JwtValidator {
    pub fn new_hmac(secret: &[u8], required_claims: Vec<String>) -> Self {
        let mut validation = Validation::new(Algorithm::HS256);
        validation.validate_exp = true;
        validation.required_spec_claims.insert("exp".to_string());
        JwtValidator {
            decoding_key: DecodingKey::from_secret(secret),
            validation,
            required_claims,
        }
    }

    pub fn new_pem(pem: &[u8], required_claims: Vec<String>) -> Self {
        // Can be RSA or EC, let's allow RS256/ES256
        let mut validation = Validation::new(Algorithm::RS256);
        validation.validate_exp = true;
        validation.required_spec_claims.insert("exp".to_string());
        // Add ES256 to valid algorithms too
        validation.algorithms = vec![Algorithm::RS256, Algorithm::ES256];

        JwtValidator {
            decoding_key: DecodingKey::from_rsa_pem(pem)
                .or_else(|_| DecodingKey::from_ec_pem(pem))
                .unwrap_or_else(|_| DecodingKey::from_secret(pem)),
            validation,
            required_claims,
        }
    }

    pub fn validate_request<B>(
        &self,
        req: &Request<B>,
    ) -> Result<HashMap<String, Value>, HelixError> {
        let auth_header = req
            .headers()
            .get("authorization")
            .and_then(|v| v.to_str().ok());

        let token = match auth_header {
            Some(auth) => {
                if auth.to_lowercase().starts_with("bearer ") {
                    auth[7..].trim().to_string()
                } else {
                    auth.trim().to_string()
                }
            }
            None => {
                // Try X-API-Key or other headers
                if let Some(key) = req.headers().get("x-api-key").and_then(|v| v.to_str().ok()) {
                    key.trim().to_string()
                } else {
                    return Err(HelixError::new(
                        ErrorCode::Unauthenticated,
                        "missing authorization header",
                    ));
                }
            }
        };

        let token_data =
            decode::<HashMap<String, Value>>(&token, &self.decoding_key, &self.validation)
                .map_err(|e| {
                    HelixError::new(ErrorCode::Unauthenticated, &format!("invalid token: {}", e))
                })?;

        // Check required claims
        for claim in &self.required_claims {
            if !token_data.claims.contains_key(claim) {
                return Err(HelixError::new(
                    ErrorCode::PermissionDenied,
                    &format!("missing required claim '{}'", claim),
                ));
            }
        }

        Ok(token_data.claims)
    }
}

pub fn validate_api_key(
    headers: &HeaderMap,
    valid_keys: &HashMap<String, String>,
) -> Result<String, HelixError> {
    let key = headers
        .get("x-api-key")
        .and_then(|v| v.to_str().ok())
        .map(|s| s.to_string())
        .or_else(|| {
            headers
                .get("authorization")
                .and_then(|v| v.to_str().ok())
                .map(|auth| {
                    if auth.to_lowercase().starts_with("bearer ") {
                        auth[7..].trim().to_string()
                    } else {
                        auth.trim().to_string()
                    }
                })
        });

    let key = match key {
        Some(k) if !k.is_empty() => k,
        _ => {
            return Err(HelixError::new(
                ErrorCode::Unauthenticated,
                "missing API key",
            ))
        }
    };

    match valid_keys.get(&key) {
        Some(principal) => Ok(principal.clone()),
        None => Err(HelixError::new(
            ErrorCode::Unauthenticated,
            "invalid API key",
        )),
    }
}
