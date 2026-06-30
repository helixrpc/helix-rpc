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
        Ok(UserProfile {
            user_id: req.user_id,
            username: format!("{}-response", req.username),
            email: format!("{}-verified", req.email),
        })
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

    // Split stream into read and write halves
    let std_stream = stream.into_std().unwrap();
    std_stream.set_nonblocking(false).unwrap(); // Configure for blocking operation inside thrift crate
    let read_conn = std_stream.try_clone().unwrap();
    let write_conn = std_stream;

    match protocol {
        Protocol::ThriftCompact => {
            let reader = TFramedReadTransport::new(read_conn);
            let writer = TFramedWriteTransport::new(write_conn);
            let mut iprot = TCompactInputProtocol::new(reader);
            let mut oprot = TCompactOutputProtocol::new(writer);
            let _ = process_thrift_request(&mut iprot, &mut oprot).await;
        }
        Protocol::ThriftBinary => {
            let reader = TFramedReadTransport::new(read_conn);
            let writer = TFramedWriteTransport::new(write_conn);
            let mut iprot = TBinaryInputProtocol::new(reader, true);
            let mut oprot = TBinaryOutputProtocol::new(writer, true);
            let _ = process_thrift_request(&mut iprot, &mut oprot).await;
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
