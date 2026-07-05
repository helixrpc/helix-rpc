pub mod generated;
pub mod complex_fallback;

use generated::{UserProfile, UserProfileService};
use helix_rt::{sniff_protocol, Compressor, Protocol};
use std::net::SocketAddr;
use thrift::protocol::{
    TBinaryInputProtocol, TBinaryOutputProtocol, TCompactInputProtocol, TCompactOutputProtocol,
    TInputProtocol, TMessageIdentifier, TMessageType, TOutputProtocol, TSerializable,
};
use thrift::transport::{ReadHalf, TFramedReadTransport, TFramedWriteTransport, WriteHalf};
use tokio::net::{TcpListener, TcpStream};

// --- Streaming handler ---
struct StreamingEchoHandler;

#[async_trait::async_trait]
impl helix_rt::HttpStreamingHandler for StreamingEchoHandler {
    fn is_streaming(&self, path: &str) -> bool {
        path == "/helix_example.UserProfileService/StreamUserProfiles"
    }

    async fn handle_stream(
        &self,
        _path: &str,
        mut stream: Box<dyn helix_rt::ServerStream>,
    ) -> Result<(), String> {
        loop {
            match stream.recv().await? {
                None => return Ok(()),
                Some(payload) => {
                    // Decode the protobuf UserProfile from the payload
                    let req = <UserProfile as prost::Message>::decode(&payload[..])
                        .map_err(|e| format!("decode error: {}", e))?;
                    // Echo back with modified username
                    let resp = UserProfile {
                        user_id: req.user_id,
                        username: format!("{}-echoed", req.username),
                        email: req.email,
                    };
                    let mut resp_bytes = Vec::new();
                    <UserProfile as prost::Message>::encode(&resp, &mut resp_bytes)
                        .map_err(|e| format!("encode error: {}", e))?;
                    stream.send(resp_bytes).await?;
                }
            }
        }
    }
}

struct ServiceImpl;

#[async_trait::async_trait]
impl UserProfileService for ServiceImpl {
    async fn get_user_profile(&self, req: UserProfile) -> Result<UserProfile, thrift::Error> {
        let mut username = format!("{}-response", req.username);
        if let Some(trace_ids) = helix_rt::get_metadata("x-trace-id") {
            if !trace_ids.is_empty() {
                username = format!("{}-{}", username, trace_ids[0]);
            }
        }
        Ok(UserProfile {
            user_id: req.user_id,
            username,
            email: format!("{}-verified", req.email),
        })
    }
}

#[async_trait::async_trait]
impl helix_rt::HttpServiceHandler for ServiceImpl {
    async fn handle_request(
        &self,
        path: &str,
        body: Vec<u8>,
        is_json: bool,
    ) -> Result<(Vec<u8>, String), String> {
        if path == "/helix_example.UserProfileService/GetUserProfile" {
            if is_json {
                let req: UserProfile =
                    serde_json::from_slice(&body).map_err(|e| format!("invalid json: {}", e))?;
                let resp = self
                    .get_user_profile(req)
                    .await
                    .map_err(|e| format!("execution error: {}", e))?;
                let resp_bytes =
                    serde_json::to_vec(&resp).map_err(|e| format!("serialization error: {}", e))?;
                return Ok((resp_bytes, "application/json".to_string()));
            } else {
                let req = <UserProfile as prost::Message>::decode(&body[..])
                    .map_err(|e| format!("invalid protobuf: {}", e))?;
                let resp = self
                    .get_user_profile(req)
                    .await
                    .map_err(|e| format!("execution error: {}", e))?;
                let mut resp_bytes = Vec::new();
                <UserProfile as prost::Message>::encode(&resp, &mut resp_bytes)
                    .map_err(|e| format!("serialization error: {}", e))?;
                return Ok((resp_bytes, "application/grpc".to_string()));
            }
        }
        if path == "/helix_example.UserProfileService/GetSlowProfile" {
            tokio::time::sleep(std::time::Duration::from_millis(50)).await;
            if is_json {
                let req: UserProfile =
                    serde_json::from_slice(&body).map_err(|e| format!("invalid json: {}", e))?;
                let resp = UserProfile {
                    user_id: req.user_id,
                    username: format!("{}-slow-response", req.username),
                    email: req.email,
                };
                let resp_bytes =
                    serde_json::to_vec(&resp).map_err(|e| format!("serialization error: {}", e))?;
                return Ok((resp_bytes, "application/json".to_string()));
            } else {
                let req = <UserProfile as prost::Message>::decode(&body[..])
                    .map_err(|e| format!("invalid protobuf: {}", e))?;
                let resp = UserProfile {
                    user_id: req.user_id,
                    username: format!("{}-slow-response", req.username),
                    email: req.email,
                };
                let mut resp_bytes = Vec::new();
                <UserProfile as prost::Message>::encode(&resp, &mut resp_bytes)
                    .map_err(|e| format!("serialization error: {}", e))?;
                return Ok((resp_bytes, "application/grpc".to_string()));
            }
        }
        Err(format!("unknown path: {}", path))
    }
}

fn generate_test_cert() -> (Vec<rustls::Certificate>, rustls::PrivateKey) {
    let subject_alt_names = vec!["localhost".to_string(), "127.0.0.1".to_string()];
    let cert = rcgen::generate_simple_self_signed(subject_alt_names).unwrap();
    let cert_der = cert.serialize_der().unwrap();
    let key_der = cert.serialize_private_key_der();
    (
        vec![rustls::Certificate(cert_der)],
        rustls::PrivateKey(key_der),
    )
}

#[tokio::main]
async fn main() {
    let args: Vec<String> = std::env::args().collect();
    let server_only = args.contains(&"--server".to_string());
    let client_addr = args
        .iter()
        .position(|r| r == "--client")
        .and_then(|idx| args.get(idx + 1))
        .map(|s| {
            s.parse::<SocketAddr>()
                .expect("invalid client socket address")
        });

    if let Some(addr) = client_addr {
        run_thrift_compact_client(addr).await;
        run_thrift_binary_client(addr).await;
        println!("All external Rust-to-Go client tests passed successfully!");
        return;
    }

    let (certs, key) = generate_test_cert();
    let mut server_config = rustls::ServerConfig::builder()
        .with_safe_defaults()
        .with_no_client_auth()
        .with_single_cert(certs.clone(), key)
        .unwrap();
    server_config.alpn_protocols = vec![b"h2".to_vec(), b"http/1.1".to_vec()];
    let tls_acceptor = tokio_rustls::TlsAcceptor::from(std::sync::Arc::new(server_config));

    // Bind dynamic port
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();
    drop(listener);
    println!("Rust Helix server listening on {}", addr);

    let mut server = helix_rt::server::HelixServer::new(
        &addr.to_string(),
        std::sync::Arc::new(ServiceImpl),
        vec![
            helix_rt::RestRoute::new(
                "POST",
                "/v1/users",
                "/helix_example.UserProfileService/GetUserProfile",
            ),
            helix_rt::RestRoute::new(
                "GET",
                "/v1/users/{user_id}",
                "/helix_example.UserProfileService/GetUserProfile",
            ),
        ],
    );
    server.set_streaming_handler(std::sync::Arc::new(StreamingEchoHandler));
    server.set_protocol_fallback(handle_thrift_fallback);
    server.set_tls_acceptor(tls_acceptor);

    let server_arc = std::sync::Arc::new(server);
    let srv_clone = server_arc.clone();

    // Spawn server accept loop
    tokio::spawn(async move {
        srv_clone.start().await.unwrap();
    });

    if server_only {
        // Run forever until killed
        tokio::signal::ctrl_c().await.unwrap();
    } else {
        // Wait for server to bind
        tokio::time::sleep(std::time::Duration::from_millis(50)).await;

        // Run Thrift client tests
        run_thrift_compact_client(addr).await;
        run_thrift_binary_client(addr).await;
        // Run gRPC bidirectional streaming test
        run_grpc_streaming_test(addr).await;
        // Run new production features tests
        run_grpc_deadline_test(addr).await;
        run_grpc_health_test(addr).await;
        run_grpc_compression_test(addr).await;
        run_grpc_tls_test(addr).await;

        // Run FlatBuffers local codec tests
        run_flatbuffers_codec_test().await;

        // Test graceful shutdown
        println!("Testing Graceful Shutdown...");
        server_arc.shutdown();
        tokio::time::sleep(std::time::Duration::from_millis(50)).await;

        // Try to connect, it should fail
        let res = TcpStream::connect(addr).await;
        assert!(
            res.is_err(),
            "Server should have stopped accepting connections"
        );
        println!("Graceful Shutdown test passed!");

        println!("All Rust E2E tests passed successfully!");
    }
}

fn handle_thrift_fallback(stream: TcpStream, protocol: helix_rt::sniffer::Protocol) {
    if protocol == helix_rt::sniffer::Protocol::ThriftCompact
        || protocol == helix_rt::sniffer::Protocol::ThriftBinary
    {
        let rt = tokio::runtime::Handle::current();
        std::thread::spawn(move || {
            let std_stream = stream.into_std().unwrap();
            std_stream.set_nonblocking(false).unwrap();
            let read_conn = std_stream.try_clone().unwrap();
            let write_conn = std_stream;

            if protocol == helix_rt::sniffer::Protocol::ThriftCompact {
                let reader = TFramedReadTransport::new(read_conn);
                let writer = TFramedWriteTransport::new(write_conn);
                let mut iprot = TCompactInputProtocol::new(reader);
                let mut oprot = TCompactOutputProtocol::new(writer);
                let _ = process_thrift_request(&rt, &mut iprot, &mut oprot);
            } else {
                let reader = TFramedReadTransport::new(read_conn);
                let writer = TFramedWriteTransport::new(write_conn);
                let mut iprot = TBinaryInputProtocol::new(reader, true);
                let mut oprot = TBinaryOutputProtocol::new(writer, true);
                let _ = process_thrift_request(&rt, &mut iprot, &mut oprot);
            }
        });
    }
}

fn process_thrift_request<I: TInputProtocol, O: TOutputProtocol>(
    rt: &tokio::runtime::Handle,
    iprot: &mut I,
    oprot: &mut O,
) -> Result<(), thrift::Error> {
    let msg_ident = iprot.read_message_begin()?;
    if msg_ident.name != "GetUserProfile" {
        return Err(thrift::Error::Application(thrift::ApplicationError::new(
            thrift::ApplicationErrorKind::UnknownMethod,
            format!("unknown method {}", msg_ident.name),
        )));
    }

    let req = UserProfile::read_from_in_protocol(iprot)?;
    iprot.read_message_end()?;

    let handler = ServiceImpl;
    let resp = rt.block_on(handler.get_user_profile(req))?;

    oprot.write_message_begin(&TMessageIdentifier::new(
        "GetUserProfile",
        TMessageType::Reply,
        msg_ident.sequence_number,
    ))?;
    resp.write_to_out_protocol(oprot)?;
    oprot.write_message_end()?;
    oprot.flush()?;

    Ok(())
}

async fn run_thrift_compact_client(addr: SocketAddr) {
    let stream = std::net::TcpStream::connect(addr).unwrap();
    let reader = TFramedReadTransport::new(stream.try_clone().unwrap());
    let writer = TFramedWriteTransport::new(stream);
    let mut iprot = TCompactInputProtocol::new(reader);
    let mut oprot = TCompactOutputProtocol::new(writer);

    let req = UserProfile {
        user_id: 111,
        username: "rust-client-compact".to_string(),
        email: "compact@test.com".to_string(),
    };

    // Write message
    oprot
        .write_message_begin(&TMessageIdentifier::new(
            "GetUserProfile",
            TMessageType::Call,
            1,
        ))
        .unwrap();
    req.write_to_out_protocol(&mut oprot).unwrap();
    oprot.write_message_end().unwrap();
    oprot.flush().unwrap();

    // Read response
    let msg_ident = iprot.read_message_begin().unwrap();
    assert_eq!(msg_ident.name, "GetUserProfile");
    let resp = UserProfile::read_from_in_protocol(&mut iprot).unwrap();
    iprot.read_message_end().unwrap();

    assert_eq!(resp.user_id, 111);
    assert_eq!(resp.username, "rust-client-compact-response");
    assert_eq!(resp.email, "compact@test.com-verified");
    println!("Rust Thrift Compact test passed!");
}

async fn run_thrift_binary_client(addr: SocketAddr) {
    let stream = std::net::TcpStream::connect(addr).unwrap();
    let reader = TFramedReadTransport::new(stream.try_clone().unwrap());
    let writer = TFramedWriteTransport::new(stream);
    let mut iprot = TBinaryInputProtocol::new(reader, true);
    let mut oprot = TBinaryOutputProtocol::new(writer, true);

    let req = UserProfile {
        user_id: 222,
        username: "rust-client-binary".to_string(),
        email: "binary@test.com".to_string(),
    };

    // Write message
    oprot
        .write_message_begin(&TMessageIdentifier::new(
            "GetUserProfile",
            TMessageType::Call,
            2,
        ))
        .unwrap();
    req.write_to_out_protocol(&mut oprot).unwrap();
    oprot.write_message_end().unwrap();
    oprot.flush().unwrap();

    // Read response
    let msg_ident = iprot.read_message_begin().unwrap();
    assert_eq!(msg_ident.name, "GetUserProfile");
    let resp = UserProfile::read_from_in_protocol(&mut iprot).unwrap();
    iprot.read_message_end().unwrap();

    assert_eq!(resp.user_id, 222);
    assert_eq!(resp.username, "rust-client-binary-response");
    assert_eq!(resp.email, "binary@test.com-verified");
    println!("Rust Thrift Binary test passed!");
}

async fn run_grpc_streaming_test(addr: std::net::SocketAddr) {
    use hyper::body::HttpBody;
    use hyper::{Body, Client, Request};

    // Build an HTTP/2 client (h2c / prior-knowledge)
    let client = Client::builder().http2_only(true).build_http::<Body>();

    // Create a streaming body using a channel
    let (mut body_tx, body_rx) = Body::channel();

    let uri = format!(
        "http://{}/helix_example.UserProfileService/StreamUserProfiles",
        addr
    );
    let req = Request::builder()
        .method("POST")
        .uri(uri)
        .header("content-type", "application/grpc")
        .body(body_rx)
        .unwrap();

    // Send the request in the background; the response will arrive once
    // the server starts sending frames back.
    let resp_fut = tokio::spawn(async move { client.request(req).await });

    // Send 3 gRPC frames
    for i in 1..=3i64 {
        let profile = UserProfile {
            user_id: i,
            username: format!("stream-user-{}", i),
            email: String::new(),
        };
        let mut payload = Vec::new();
        <UserProfile as prost::Message>::encode(&profile, &mut payload).unwrap();

        let mut frame = Vec::with_capacity(5 + payload.len());
        frame.push(0u8); // uncompressed
        frame.extend_from_slice(&(payload.len() as u32).to_be_bytes());
        frame.extend_from_slice(&payload);

        body_tx
            .send_data(hyper::body::Bytes::from(frame))
            .await
            .unwrap();
        tokio::time::sleep(std::time::Duration::from_millis(50)).await;
    }
    // Signal end of client stream
    drop(body_tx);

    let resp = resp_fut.await.unwrap().unwrap();
    assert_eq!(resp.status(), hyper::StatusCode::OK);

    let mut body = resp.into_body();
    let mut buf = Vec::new();

    // Collect all body data
    while let Some(chunk) = body.data().await {
        buf.extend_from_slice(&chunk.unwrap());
    }

    // Parse 3 response frames from the collected buffer
    let mut offset = 0;
    for i in 1..=3i64 {
        assert!(
            buf.len() >= offset + 5,
            "not enough data for frame {} header",
            i
        );
        let length = u32::from_be_bytes([
            buf[offset + 1],
            buf[offset + 2],
            buf[offset + 3],
            buf[offset + 4],
        ]) as usize;
        offset += 5;
        assert!(
            buf.len() >= offset + length,
            "not enough data for frame {} payload",
            i
        );
        let payload = &buf[offset..offset + length];
        offset += length;

        let resp_profile = <UserProfile as prost::Message>::decode(payload).unwrap();
        assert_eq!(resp_profile.user_id, i);
        let expected = format!("stream-user-{}-echoed", i);
        assert_eq!(
            resp_profile.username, expected,
            "frame {} username mismatch",
            i
        );
    }

    println!("Rust gRPC Bidirectional Streaming test passed!");
}

async fn run_grpc_deadline_test(addr: SocketAddr) {
    use hyper::{Body, Client, Request};

    let client = Client::builder().http2_only(true).build_http::<Body>();

    let profile = UserProfile {
        user_id: 42,
        username: "deadline-user".to_string(),
        email: "deadline@test.com".to_string(),
    };
    let mut payload = Vec::new();
    <UserProfile as prost::Message>::encode(&profile, &mut payload).unwrap();

    let mut frame = Vec::with_capacity(5 + payload.len());
    frame.push(0u8);
    frame.extend_from_slice(&(payload.len() as u32).to_be_bytes());
    frame.extend_from_slice(&payload);

    let uri = format!(
        "http://{}/helix_example.UserProfileService/GetSlowProfile",
        addr
    );
    let req = Request::builder()
        .method("POST")
        .uri(uri)
        .header("content-type", "application/grpc")
        // Set short deadline: 10 milliseconds (server takes 50ms)
        .header("grpc-timeout", "10m")
        .body(Body::from(frame))
        .unwrap();

    let resp = client.request(req).await.unwrap();
    assert_eq!(resp.status(), hyper::StatusCode::OK);

    let grpc_status = resp.headers().get("grpc-status").unwrap().to_str().unwrap();
    // gRPC status code 4 is DEADLINE_EXCEEDED
    assert_eq!(grpc_status, "4");
    println!("Rust gRPC deadline propagation test passed!");
}

async fn run_grpc_health_test(addr: SocketAddr) {
    use hyper::body::HttpBody;
    use hyper::{Body, Client, Request};

    let client = Client::builder().http2_only(true).build_http::<Body>();

    let frame = vec![0u8; 5]; // empty service name request

    let uri = format!("http://{}/grpc.health.v1.Health/Check", addr);
    let req = Request::builder()
        .method("POST")
        .uri(uri)
        .header("content-type", "application/grpc")
        .body(Body::from(frame))
        .unwrap();

    let resp = client.request(req).await.unwrap();
    assert_eq!(resp.status(), hyper::StatusCode::OK);
    assert_eq!(
        resp.headers().get("grpc-status").unwrap().to_str().unwrap(),
        "0"
    );

    let mut body = resp.into_body();
    let mut buf = Vec::new();
    while let Some(chunk) = body.data().await {
        buf.extend_from_slice(&chunk.unwrap());
    }

    assert!(buf.len() >= 5);
    let length = u32::from_be_bytes([buf[1], buf[2], buf[3], buf[4]]) as usize;
    let payload = &buf[5..5 + length];
    // status should be 1 (Serving) -> [0x08, 0x01]
    assert_eq!(payload, &[0x08, 0x01]);
    println!("Rust gRPC Health Check test passed!");
}

async fn run_grpc_compression_test(addr: SocketAddr) {
    use hyper::body::HttpBody;
    use hyper::{Body, Client, Request};

    let client = Client::builder().http2_only(true).build_http::<Body>();

    let profile = UserProfile {
        user_id: 88,
        username: "compressed-user".to_string(),
        email: "compress@test.com".to_string(),
    };
    let mut payload = Vec::new();
    <UserProfile as prost::Message>::encode(&profile, &mut payload).unwrap();

    // Compress payload using gzip
    let compressor = helix_rt::GzipCompressor;
    let compressed_payload = compressor.compress(&payload).unwrap();

    let mut frame = Vec::with_capacity(5 + compressed_payload.len());
    frame.push(1u8); // compressed flag
    frame.extend_from_slice(&(compressed_payload.len() as u32).to_be_bytes());
    frame.extend_from_slice(&compressed_payload);

    let uri = format!(
        "http://{}/helix_example.UserProfileService/GetUserProfile",
        addr
    );
    let req = Request::builder()
        .method("POST")
        .uri(uri)
        .header("content-type", "application/grpc")
        .header("grpc-encoding", "gzip")
        .body(Body::from(frame))
        .unwrap();

    let resp = client.request(req).await.unwrap();
    assert_eq!(resp.status(), hyper::StatusCode::OK);
    assert_eq!(
        resp.headers().get("grpc-status").unwrap().to_str().unwrap(),
        "0"
    );
    assert_eq!(
        resp.headers()
            .get("grpc-encoding")
            .unwrap()
            .to_str()
            .unwrap(),
        "gzip"
    );

    let mut body = resp.into_body();
    let mut buf = Vec::new();
    while let Some(chunk) = body.data().await {
        buf.extend_from_slice(&chunk.unwrap());
    }

    assert!(buf.len() >= 5);
    let compressed_flag = buf[0];
    assert_eq!(compressed_flag, 1);
    let length = u32::from_be_bytes([buf[1], buf[2], buf[3], buf[4]]) as usize;
    let payload = &buf[5..5 + length];

    // Decompress payload
    let decompressed = compressor.decompress(payload).unwrap();
    let resp_profile = <UserProfile as prost::Message>::decode(&decompressed[..]).unwrap();
    assert_eq!(resp_profile.user_id, 88);
    assert_eq!(resp_profile.username, "compressed-user-response");
    println!("Rust gRPC compression test passed!");
}

struct DummyVerifier;
impl rustls::client::ServerCertVerifier for DummyVerifier {
    fn verify_server_cert(
        &self,
        _end_entity: &rustls::Certificate,
        _intermediates: &[rustls::Certificate],
        _server_name: &rustls::ServerName,
        _scts: &mut dyn Iterator<Item = &[u8]>,
        _ocsp_response: &[u8],
        _now: std::time::SystemTime,
    ) -> Result<rustls::client::ServerCertVerified, rustls::Error> {
        Ok(rustls::client::ServerCertVerified::assertion())
    }
}

async fn run_grpc_tls_test(addr: SocketAddr) {
    use hyper::{Body, Client, Request};

    let mut client_config = rustls::ClientConfig::builder()
        .with_safe_defaults()
        .with_custom_certificate_verifier(std::sync::Arc::new(DummyVerifier))
        .with_no_client_auth();

    let https = hyper_rustls::HttpsConnectorBuilder::new()
        .with_tls_config(client_config)
        .https_only()
        .enable_http2()
        .build();
    let client: Client<_, Body> = Client::builder().http2_only(true).build(https);

    let profile = UserProfile {
        user_id: 99,
        username: "tls-user".to_string(),
        email: "tls@test.com".to_string(),
    };
    let mut payload = Vec::new();
    <UserProfile as prost::Message>::encode(&profile, &mut payload).unwrap();

    let mut frame = Vec::with_capacity(5 + payload.len());
    frame.push(0u8);
    frame.extend_from_slice(&(payload.len() as u32).to_be_bytes());
    frame.extend_from_slice(&payload);

    let uri = format!(
        "https://localhost:{}/helix_example.UserProfileService/GetUserProfile",
        addr.port()
    );
    let req = Request::builder()
        .method("POST")
        .uri(uri)
        .header("content-type", "application/grpc")
        .body(Body::from(frame))
        .unwrap();

    let resp = client.request(req).await.unwrap();
    assert_eq!(resp.status(), hyper::StatusCode::OK);

    let mut body = resp.into_body();
    use hyper::body::HttpBody;
    let mut buf = Vec::new();
    while let Some(chunk) = body.data().await {
        buf.extend_from_slice(&chunk.unwrap());
    }

    let length = u32::from_be_bytes([buf[1], buf[2], buf[3], buf[4]]) as usize;
    let payload = &buf[5..5 + length];

    let resp_profile = <UserProfile as prost::Message>::decode(payload).unwrap();
    assert_eq!(resp_profile.user_id, 99);
    assert_eq!(resp_profile.username, "tls-user-response");

    println!("Rust gRPC TLS test passed!");
}

async fn run_flatbuffers_codec_test() {
    let original = UserProfile {
        user_id: 123456,
        username: "rust_flatbuffer_user".to_string(),
        email: "rust_flat@test.com".to_string(),
    };

    let buf = original.marshal_flatbuffers();
    assert!(
        !buf.is_empty(),
        "flatbuffers: marshal returned empty buffer"
    );

    let decoded = UserProfile::unmarshal_flatbuffers(&buf).unwrap();
    assert_eq!(decoded.user_id, original.user_id);
    assert_eq!(decoded.username, original.username);
    assert_eq!(decoded.email, original.email);

    println!("Rust FlatBuffers local codec test passed!");
}

#[cfg(test)]
mod advanced_optimization_tests {
    use super::generated::UserProfile;
    use prost::Message;

    fn make_proto_bytes() -> Vec<u8> {
        let profile = UserProfile {
            user_id: 42,
            username: "zero_copy_hero".to_string(),
            email: "hero@helix.rpc".to_string(),
        };
        let mut buf = Vec::new();
        profile.encode(&mut buf).unwrap();
        buf
    }

    #[test]
    fn test_zero_copy_view_parse() {
        let buf = make_proto_bytes();
        let view = super::generated::UserProfileView::parse(&buf)
            .expect("UserProfileView::parse should succeed");
        assert_eq!(view.user_id, 42);
        assert_eq!(view.username, "zero_copy_hero");
        assert_eq!(view.email, "hero@helix.rpc");
    }

    #[test]
    fn test_zero_copy_view_lifetime_borrows_buffer() {
        let buf = make_proto_bytes();
        let view = super::generated::UserProfileView::parse(&buf)
            .expect("parse should succeed");
        // username and email are &str slices pointing INTO buf (no allocation)
        // Verify they are subslices of buf by checking pointer range
        let buf_start = buf.as_ptr() as usize;
        let buf_end = buf_start + buf.len();
        let username_ptr = view.username.as_ptr() as usize;
        assert!(
            username_ptr >= buf_start && username_ptr < buf_end,
            "username pointer should be within original buffer (zero-copy)"
        );
        let email_ptr = view.email.as_ptr() as usize;
        assert!(
            email_ptr >= buf_start && email_ptr < buf_end,
            "email pointer should be within original buffer (zero-copy)"
        );
    }

    #[test]
    fn test_lazy_smart_fields_user_id() {
        let buf = make_proto_bytes();
        let lazy = super::generated::LazyUserProfile::new(&buf);
        assert_eq!(lazy.get_user_id().unwrap(), 42);
    }

    #[test]
    fn test_lazy_smart_fields_username() {
        let buf = make_proto_bytes();
        let lazy = super::generated::LazyUserProfile::new(&buf);
        assert_eq!(lazy.get_username().unwrap(), "zero_copy_hero");
    }

    #[test]
    fn test_lazy_smart_fields_email() {
        let buf = make_proto_bytes();
        let lazy = super::generated::LazyUserProfile::new(&buf);
        assert_eq!(lazy.get_email().unwrap(), "hero@helix.rpc");
    }

    #[test]
    fn test_lazy_smart_fields_independent_field_access() {
        // Verify each getter can be called independently without full deserialization
        let buf = make_proto_bytes();
        let lazy = super::generated::LazyUserProfile::new(&buf);
        // Only access email — should work without ever decoding username or user_id
        let email = lazy.get_email().expect("get_email should succeed");
        assert_eq!(email, "hero@helix.rpc");
    }

    #[test]
    fn test_transpile_protobuf_to_thrift_compact_succeeds() {
        let buf = make_proto_bytes();
        let mut output = Vec::new();
        UserProfile::transpile_protobuf_to_thrift_compact(&buf, &mut output)
            .expect("transpile should succeed");
        // Must end with Thrift STOP byte
        assert!(!output.is_empty(), "output should not be empty");
        assert_eq!(*output.last().unwrap(), 0x00, "last byte must be Thrift STOP");
    }

    #[test]
    fn test_transpile_contains_field_strings() {
        let buf = make_proto_bytes();
        let mut output = Vec::new();
        UserProfile::transpile_protobuf_to_thrift_compact(&buf, &mut output).unwrap();
        // The string "zero_copy_hero" and "hero@helix.rpc" must appear verbatim in Thrift output
        let contains_username = output.windows(14).any(|w| w == b"zero_copy_hero");
        let contains_email = output.windows(14).any(|w| w == b"hero@helix.rpc");
        assert!(contains_username, "username string should appear in Thrift compact output");
        assert!(contains_email, "email string should appear in Thrift compact output");
    }

    #[test]
    fn test_transpile_roundtrip_no_data_loss() {
        // Transpile, then verify we can still see all field data
        let profile = UserProfile {
            user_id: 99,
            username: "roundtrip".to_string(),
            email: "rt@test.com".to_string(),
        };
        let mut proto_buf = Vec::new();
        profile.encode(&mut proto_buf).unwrap();
        let mut thrift_buf = Vec::new();
        UserProfile::transpile_protobuf_to_thrift_compact(&proto_buf, &mut thrift_buf).unwrap();
        let contains_username = thrift_buf.windows(9).any(|w| w == b"roundtrip");
        let contains_email = thrift_buf.windows(11).any(|w| w == b"rt@test.com");
        assert!(contains_username, "roundtrip username should be in thrift output");
        assert!(contains_email, "roundtrip email should be in thrift output");
    }

    #[test]
    fn test_ebpf_sockmap_fallback_on_non_linux() {
        // On macOS (CI) this must return Err (graceful fallback)
        let result = helix_rt::load_bpf_sockmap("127.0.0.1:9090");
        #[cfg(not(target_os = "linux"))]
        assert!(result.is_err(), "expected fallback Err on non-Linux");
        // On Linux without root this would also be Err
    }

    #[test]
    fn test_ebpf_unix_prefix_detection() {
        assert!(helix_rt::has_unix_prefix("unix:///tmp/helix.sock"));
        assert!(!helix_rt::has_unix_prefix("127.0.0.1:9090"));
        assert!(!helix_rt::has_unix_prefix("localhost:8080"));
    }

    #[test]
    fn test_ebpf_strip_unix_prefix() {
        assert_eq!(helix_rt::strip_unix_prefix("unix:///tmp/helix.sock"), "/tmp/helix.sock");
        assert_eq!(helix_rt::strip_unix_prefix("127.0.0.1:9090"), "127.0.0.1:9090");
    }

    #[tokio::test]
    async fn test_e2e_grpc_web() {
        use std::sync::Arc;
        use hyper::{Client, Request, Body, StatusCode};
        use base64::Engine;

        // Bind dynamic port
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let addr = listener.local_addr().unwrap();
        drop(listener);

        let server = helix_rt::server::HelixServer::new(
            &addr.to_string(),
            Arc::new(super::ServiceImpl),
            vec![],
        );

        let srv = Arc::new(server);
        let srv_clone = srv.clone();
        tokio::spawn(async move {
            srv_clone.start().await.unwrap();
        });

        // Wait for server to start
        tokio::time::sleep(std::time::Duration::from_millis(50)).await;

        let client = Client::new();

        // 1. Binary mode
        let req_obj = UserProfile {
            user_id: 12345,
            username: "rust_grpc_web".to_string(),
            email: "rust@grpcweb.com".to_string(),
        };
        let mut payload = Vec::new();
        req_obj.encode(&mut payload).unwrap();

        let mut frame = Vec::new();
        frame.push(0);
        let length = payload.len() as u32;
        frame.extend_from_slice(&length.to_be_bytes());
        frame.extend_from_slice(&payload);

        let req = Request::builder()
            .method("POST")
            .uri(format!("http://{}/helix_example.UserProfileService/GetUserProfile", addr))
            .header("content-type", "application/grpc-web")
            .body(Body::from(frame.clone()))
            .unwrap();

        let res = client.request(req).await.unwrap();
        assert_eq!(res.status(), StatusCode::OK);
        assert_eq!(res.headers().get("content-type").unwrap(), "application/grpc-web");

        let body_bytes = hyper::body::to_bytes(res.into_body()).await.unwrap();
        assert!(body_bytes.len() > 10);

        // Read message frame
        let mut msg_len_bytes = [0u8; 4];
        msg_len_bytes.copy_from_slice(&body_bytes[1..5]);
        let msg_len = u32::from_be_bytes(msg_len_bytes) as usize;
        let msg_payload = &body_bytes[5..5+msg_len];
        let decoded = UserProfile::decode(msg_payload).unwrap();
        assert_eq!(decoded.user_id, 12345);
        assert!(decoded.username.contains("rust_grpc_web-response"));

        // Read trailers frame
        let trailers_header = &body_bytes[5+msg_len..5+msg_len+5];
        assert_eq!(trailers_header[0], 0x80);

        // 2. Text mode
        let encoded_frame = base64::engine::general_purpose::STANDARD.encode(&frame);
        let req_text = Request::builder()
            .method("POST")
            .uri(format!("http://{}/helix_example.UserProfileService/GetUserProfile", addr))
            .header("content-type", "application/grpc-web-text")
            .body(Body::from(encoded_frame))
            .unwrap();

        let res_text = client.request(req_text).await.unwrap();
        assert_eq!(res_text.status(), StatusCode::OK);
        assert_eq!(res_text.headers().get("content-type").unwrap(), "application/grpc-web-text");

        let text_body_bytes = hyper::body::to_bytes(res_text.into_body()).await.unwrap();
        let decoded_body_bytes = base64::engine::general_purpose::STANDARD.decode(text_body_bytes).unwrap();

        let mut msg_len_bytes_t = [0u8; 4];
        msg_len_bytes_t.copy_from_slice(&decoded_body_bytes[1..5]);
        let msg_len_t = u32::from_be_bytes(msg_len_bytes_t) as usize;
        let msg_payload_t = &decoded_body_bytes[5..5+msg_len_t];
        let decoded_t = UserProfile::decode(msg_payload_t).unwrap();
        assert_eq!(decoded_t.user_id, 12345);
    }
}
