use std::time::Duration;

/// Parse a gRPC-Timeout header value.
///
/// The format is `<value><unit>` where unit is one of:
/// - `n` — nanoseconds
/// - `u` — microseconds
/// - `m` — milliseconds
/// - `S` — seconds
/// - `M` — minutes
/// - `H` — hours
///
/// Returns `None` if the value is malformed.
pub fn parse_grpc_timeout(val: &str) -> Option<Duration> {
    if val.is_empty() {
        return None;
    }
    let (num_str, unit) = val.split_at(val.len() - 1);
    let num: u64 = num_str.parse().ok()?;
    match unit {
        "n" => Some(Duration::from_nanos(num)),
        "u" => Some(Duration::from_micros(num)),
        "m" => Some(Duration::from_millis(num)),
        "S" => Some(Duration::from_secs(num)),
        "M" => Some(Duration::from_secs(num * 60)),
        "H" => Some(Duration::from_secs(num * 3600)),
        _ => None,
    }
}

/// Extract the deadline duration from request headers by reading the
/// `grpc-timeout` header.
pub fn extract_deadline(headers: &hyper::HeaderMap) -> Option<Duration> {
    headers
        .get("grpc-timeout")
        .and_then(|v| v.to_str().ok())
        .and_then(parse_grpc_timeout)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_grpc_timeout_units() {
        assert_eq!(parse_grpc_timeout("100n"), Some(Duration::from_nanos(100)));
        assert_eq!(parse_grpc_timeout("200u"), Some(Duration::from_micros(200)));
        assert_eq!(parse_grpc_timeout("300m"), Some(Duration::from_millis(300)));
        assert_eq!(parse_grpc_timeout("5S"), Some(Duration::from_secs(5)));
        assert_eq!(parse_grpc_timeout("2M"), Some(Duration::from_secs(120)));
        assert_eq!(parse_grpc_timeout("1H"), Some(Duration::from_secs(3600)));
    }

    #[test]
    fn test_parse_grpc_timeout_invalid() {
        assert_eq!(parse_grpc_timeout(""), None);
        assert_eq!(parse_grpc_timeout("x"), None);
        assert_eq!(parse_grpc_timeout("abc"), None);
    }
}
