pub mod generated;

use std::net::SocketAddr;
use tokio::net::{TcpListener, TcpStream};
use thrift::protocol::{
    TBinaryInputProtocol, TBinaryOutputProtocol, TCompactInputProtocol, TCompactOutputProtocol,
    TInputProtocol, TOutputProtocol, TMessageIdentifier, TMessageType, TSerializable
};
use thrift::transport::{ReadHalf, WriteHalf, TFramedReadTransport, TFramedWriteTransport};
use generated::{UserProfile, UserProfileService};
use helix_rt::{sniff_protocol, Protocol};

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
    async fn handle_request(&self, path: &str, body: Vec<u8>, is_json: bool) -> Result<(Vec<u8>, String), String> {
        if path == "/helix_example.UserProfileService/GetUserProfile" {
            if is_json {
                let req: UserProfile = serde_json::from_slice(&body)
                    .map_err(|e| format!("invalid json: {}", e))?;
                let resp = self.get_user_profile(req).await
                    .map_err(|e| format!("execution error: {}", e))?;
                let resp_bytes = serde_json::to_vec(&resp)
                    .map_err(|e| format!("serialization error: {}", e))?;
                return Ok((resp_bytes, "application/json".to_string()));
            } else {
                let req = <UserProfile as prost::Message>::decode(&body[..])
                    .map_err(|e| format!("invalid protobuf: {}", e))?;
                let resp = self.get_user_profile(req).await
                    .map_err(|e| format!("execution error: {}", e))?;
                let mut resp_bytes = Vec::new();
                <UserProfile as prost::Message>::encode(&resp, &mut resp_bytes)
                    .map_err(|e| format!("serialization error: {}", e))?;
                return Ok((resp_bytes, "application/grpc".to_string()));
            }
        }
        Err(format!("unknown path: {}", path))
    }
}

#[tokio::main]
async fn main() {
    let args: Vec<String> = std::env::args().collect();
    let server_only = args.contains(&"--server".to_string());
    let client_addr = args.iter().position(|r| r == "--client")
        .and_then(|idx| args.get(idx + 1))
        .map(|s| s.parse::<SocketAddr>().expect("invalid client socket address"));

    if let Some(addr) = client_addr {
        run_thrift_compact_client(addr).await;
        run_thrift_binary_client(addr).await;
        println!("All external Rust-to-Go client tests passed successfully!");
        return;
    }

    // Bind dynamic port
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();
    println!("Rust Helix server listening on {}", addr);

    // Spawn server accept loop
    tokio::spawn(async move {
        loop {
            let (stream, _) = match listener.accept().await {
                Ok(val) => val,
                Err(_) => break,
            };

            tokio::spawn(async move {
                handle_connection(stream).await;
            });
        }
    });

    if server_only {
        // Run forever until killed
        tokio::signal::ctrl_c().await.unwrap();
    } else {
        // Run Thrift client tests
        run_thrift_compact_client(addr).await;
        run_thrift_binary_client(addr).await;
        println!("All Rust E2E tests passed successfully!");
    }
}

async fn handle_connection(stream: TcpStream) {
    let protocol = match sniff_protocol(&stream).await {
        Ok(p) => p,
        Err(_) => return,
    };

    println!("Detected protocol: {:?}", protocol);

    // Only convert stream to blocking std::net::TcpStream for Thrift
    match protocol {
        Protocol::ThriftCompact | Protocol::ThriftBinary => {
            let std_stream = stream.into_std().unwrap();
            std_stream.set_nonblocking(false).unwrap();
            let read_conn = std_stream.try_clone().unwrap();
            let write_conn = std_stream;

            if protocol == Protocol::ThriftCompact {
                let reader = TFramedReadTransport::new(read_conn);
                let writer = TFramedWriteTransport::new(write_conn);
                let mut iprot = TCompactInputProtocol::new(reader);
                let mut oprot = TCompactOutputProtocol::new(writer);
                let _ = process_thrift_request(&mut iprot, &mut oprot).await;
            } else {
                let reader = TFramedReadTransport::new(read_conn);
                let writer = TFramedWriteTransport::new(write_conn);
                let mut iprot = TBinaryInputProtocol::new(reader, true);
                let mut oprot = TBinaryOutputProtocol::new(writer, true);
                let _ = process_thrift_request(&mut iprot, &mut oprot).await;
            }
        }
        Protocol::Grpc => {
            let handler = std::sync::Arc::new(ServiceImpl);
            let rest_routes = vec![
                helix_rt::RestRoute::new("POST", "/v1/users", "/helix_example.UserProfileService/GetUserProfile"),
                helix_rt::RestRoute::new("GET", "/v1/users/{user_id}", "/helix_example.UserProfileService/GetUserProfile"),
            ];
            helix_rt::handle_http_connection(stream, handler, rest_routes, true).await;
        }
        Protocol::Http => {
            let handler = std::sync::Arc::new(ServiceImpl);
            let rest_routes = vec![
                helix_rt::RestRoute::new("POST", "/v1/users", "/helix_example.UserProfileService/GetUserProfile"),
                helix_rt::RestRoute::new("GET", "/v1/users/{user_id}", "/helix_example.UserProfileService/GetUserProfile"),
            ];
            helix_rt::handle_http_connection(stream, handler, rest_routes, false).await;
        }
        _ => {
            println!("Skipping unsupported protocol");
        }
    }
}

async fn process_thrift_request<I: TInputProtocol, O: TOutputProtocol>(
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
    let resp = handler.get_user_profile(req).await?;

    oprot.write_message_begin(&TMessageIdentifier::new("GetUserProfile", TMessageType::Reply, msg_ident.sequence_number))?;
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
    oprot.write_message_begin(&TMessageIdentifier::new("GetUserProfile", TMessageType::Call, 1)).unwrap();
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
    oprot.write_message_begin(&TMessageIdentifier::new("GetUserProfile", TMessageType::Call, 2)).unwrap();
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
