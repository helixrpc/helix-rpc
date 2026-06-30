use tokio::net::TcpStream;
use std::time::Duration;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Protocol {
    Grpc,
    Http,
    ThriftBinary,
    ThriftCompact,
    Unknown,
}

pub async fn sniff_protocol(stream: &TcpStream) -> std::io::Result<Protocol> {
    let mut buf = [0u8; 8];
    let peek_fut = stream.peek(&mut buf);
    
    // Sniff timeout to prevent slow-loris connection blocking
    let bytes_peeked = match tokio::time::timeout(Duration::from_millis(100), peek_fut).await {
        Ok(Ok(n)) => n,
        _ => return Ok(Protocol::Unknown),
    };

    if bytes_peeked >= 4 && &buf[..4] == b"PRI " {
        return Ok(Protocol::Grpc);
    }
    if bytes_peeked >= 3 {
        let prefix = &buf[..3];
        if prefix == b"GET" || prefix == b"POS" || prefix == b"PUT" || prefix == b"DEL" {
            return Ok(Protocol::Http);
        }
    }
    if bytes_peeked >= 1 && buf[0] == 0x82 {
        return Ok(Protocol::ThriftCompact);
    }
    if bytes_peeked >= 2 && buf[0] == 0x80 && buf[1] == 0x01 {
        return Ok(Protocol::ThriftBinary);
    }
    if bytes_peeked >= 6 && buf[0] == 0x00 && buf[1] == 0x00 {
        if buf[4] == 0x82 {
            return Ok(Protocol::ThriftCompact);
        }
        if buf[4] == 0x80 && buf[5] == 0x01 {
            return Ok(Protocol::ThriftBinary);
        }
    }
    Ok(Protocol::Unknown)
}
