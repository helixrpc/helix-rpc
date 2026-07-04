use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::RwLock;

/// gRPC health-check status values, matching the `grpc.health.v1` protocol.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
#[repr(i32)]
pub enum HealthStatus {
    Unknown = 0,
    Serving = 1,
    NotServing = 2,
}

/// Thread-safe health checker that tracks per-service serving status.
#[derive(Clone)]
pub struct HealthChecker {
    statuses: Arc<RwLock<HashMap<String, HealthStatus>>>,
}

impl Default for HealthChecker {
    fn default() -> Self {
        Self::new()
    }
}

impl HealthChecker {
    /// Create a new `HealthChecker`.  The empty-string service (representing
    /// the overall server) defaults to `Serving`.
    pub fn new() -> Self {
        let mut m = HashMap::new();
        m.insert(String::new(), HealthStatus::Serving);
        HealthChecker {
            statuses: Arc::new(RwLock::new(m)),
        }
    }

    /// Set the serving status for a named service.
    pub async fn set_serving_status(&self, service: &str, status: HealthStatus) {
        let mut map = self.statuses.write().await;
        map.insert(service.to_string(), status);
    }

    /// Check the serving status for a named service.
    pub async fn check(&self, service: &str) -> Option<HealthStatus> {
        let map = self.statuses.read().await;
        map.get(service).copied()
    }

    /// Handle a health-check request.
    ///
    /// For JSON (`is_json == true`) the body is expected to be
    /// `{"service": "..."}` and the response is `{"status": "SERVING"}` etc.
    ///
    /// For protobuf (`is_json == false`) the body is the
    /// `HealthCheckRequest` message (field 1 = service name, string) and the
    /// response is a `HealthCheckResponse` (field 1 = enum status).
    pub async fn handle_request(
        &self,
        body: &[u8],
        is_json: bool,
    ) -> Result<(Vec<u8>, String), String> {
        if is_json {
            self.handle_json(body).await
        } else {
            self.handle_proto(body).await
        }
    }

    // -----------------------------------------------------------------------
    // JSON handling
    // -----------------------------------------------------------------------
    async fn handle_json(&self, body: &[u8]) -> Result<(Vec<u8>, String), String> {
        let service = if body.is_empty() {
            String::new()
        } else {
            #[derive(serde::Deserialize)]
            struct Req {
                #[serde(default)]
                service: String,
            }
            let req: Req =
                serde_json::from_slice(body).map_err(|e| format!("invalid json: {}", e))?;
            req.service
        };

        match self.check(&service).await {
            Some(status) => {
                let status_str = match status {
                    HealthStatus::Unknown => "UNKNOWN",
                    HealthStatus::Serving => "SERVING",
                    HealthStatus::NotServing => "NOT_SERVING",
                };
                let resp = serde_json::json!({ "status": status_str });
                let bytes = serde_json::to_vec(&resp).map_err(|e| format!("json encode: {}", e))?;
                Ok((bytes, "application/json".to_string()))
            }
            None => Err(format!("unknown service: {}", service)),
        }
    }

    // -----------------------------------------------------------------------
    // Protobuf handling (hand-rolled, no codegen dependency)
    //
    // HealthCheckRequest  { string service = 1; }
    // HealthCheckResponse { HealthStatus status = 1; }
    // -----------------------------------------------------------------------
    async fn handle_proto(&self, body: &[u8]) -> Result<(Vec<u8>, String), String> {
        let service = decode_proto_string(body);

        match self.check(&service).await {
            Some(status) => {
                let value = status as i32;
                let resp = encode_proto_varint_field(1, value);
                Ok((resp, "application/grpc".to_string()))
            }
            None => Err(format!("unknown service: {}", service)),
        }
    }
}

// ---------------------------------------------------------------------------
// Minimal protobuf helpers (field tag 1 only)
// ---------------------------------------------------------------------------

/// Decode a protobuf message containing a single string field (tag 1).
fn decode_proto_string(buf: &[u8]) -> String {
    if buf.is_empty() {
        return String::new();
    }
    let mut pos = 0;
    while pos < buf.len() {
        let (tag_wire, bytes_read) = decode_varint(&buf[pos..]);
        pos += bytes_read;
        let field_number = tag_wire >> 3;
        let wire_type = tag_wire & 0x07;
        match wire_type {
            // length-delimited
            2 => {
                let (len, br) = decode_varint(&buf[pos..]);
                pos += br;
                let len = len as usize;
                if field_number == 1 {
                    return String::from_utf8_lossy(&buf[pos..pos + len]).to_string();
                }
                pos += len;
            }
            // varint
            0 => {
                let (_, br) = decode_varint(&buf[pos..]);
                pos += br;
            }
            _ => break,
        }
    }
    String::new()
}

/// Encode a single varint field (tag `field`, value `value`).
fn encode_proto_varint_field(field: u32, value: i32) -> Vec<u8> {
    let mut out = Vec::new();
    // tag = (field << 3) | 0  (wire-type 0 = varint)
    encode_varint_to(&mut out, (field as u64) << 3);
    encode_varint_to(&mut out, value as u64);
    out
}

fn decode_varint(buf: &[u8]) -> (u64, usize) {
    let mut result: u64 = 0;
    let mut shift = 0u32;
    for (i, &b) in buf.iter().enumerate() {
        result |= ((b & 0x7F) as u64) << shift;
        if b & 0x80 == 0 {
            return (result, i + 1);
        }
        shift += 7;
    }
    (result, buf.len())
}

fn encode_varint_to(buf: &mut Vec<u8>, mut val: u64) {
    loop {
        let mut byte = (val & 0x7F) as u8;
        val >>= 7;
        if val != 0 {
            byte |= 0x80;
        }
        buf.push(byte);
        if val == 0 {
            break;
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_health_check_default_serving() {
        let hc = HealthChecker::new();
        assert_eq!(hc.check("").await, Some(HealthStatus::Serving));
    }

    #[tokio::test]
    async fn test_set_and_check() {
        let hc = HealthChecker::new();
        hc.set_serving_status("my.Service", HealthStatus::NotServing)
            .await;
        assert_eq!(hc.check("my.Service").await, Some(HealthStatus::NotServing));
    }

    #[tokio::test]
    async fn test_unknown_service() {
        let hc = HealthChecker::new();
        assert_eq!(hc.check("nonexistent").await, None);
    }

    #[tokio::test]
    async fn test_json_request() {
        let hc = HealthChecker::new();
        let body = b"{}";
        let (resp, ct) = hc.handle_request(body, true).await.unwrap();
        assert_eq!(ct, "application/json");
        let v: serde_json::Value = serde_json::from_slice(&resp).unwrap();
        assert_eq!(v["status"], "SERVING");
    }
}
