use std::net::SocketAddr;
use std::sync::{Arc, Mutex};
use tokio::net::UdpSocket;
use tokio::sync::mpsc::{channel, Receiver, Sender};
use std::pin::Pin;
use std::task::{Context, Poll};
use tokio::io::{AsyncRead, AsyncWrite, ReadBuf};

pub struct QuicListener {
    socket: Arc<UdpSocket>,
    stream_sender: Sender<QuicStream>,
}

impl QuicListener {
    pub fn local_addr(&self) -> Result<SocketAddr, String> {
        self.socket.local_addr().map_err(|e| e.to_string())
    }

    pub async fn bind(addr: SocketAddr) -> Result<(Self, Receiver<QuicStream>), String> {
        let socket = UdpSocket::bind(addr).await.map_err(|e| e.to_string())?;
        let socket = Arc::new(socket);
        let (tx, rx) = channel(100);

        let listener = QuicListener {
            socket: socket.clone(),
            stream_sender: tx,
        };

        // Spawn read loop
        let socket_clone = socket.clone();
        let tx_clone = listener.stream_sender.clone();
        tokio::spawn(async move {
            let mut buf = vec![0u8; 65535];
            let active_streams: Arc<Mutex<std::collections::HashMap<String, Sender<Vec<u8>>>>> =
                Arc::new(Mutex::new(std::collections::HashMap::new()));

            loop {
                match socket_clone.recv_from(&mut buf).await {
                    Ok((n, remote_addr)) => {
                        if n < 4 {
                            continue;
                        }
                        let stream_id = u32::from_be_bytes([buf[0], buf[1], buf[2], buf[3]]);
                        let key = format!("{}:{}", remote_addr, stream_id);

                        let (tx, stream_to_send) = {
                            let mut guard = active_streams.lock().unwrap();
                            let mut stream_to_send = None;
                            let tx = if let Some(tx) = guard.get(&key) {
                                tx.clone()
                            } else {
                                let (tx_stream, rx_stream) = channel(100);
                                guard.insert(key.clone(), tx_stream.clone());

                                let socket_write = socket_clone.clone();
                                let virtual_stream = QuicStream {
                                    remote_addr,
                                    read_receiver: rx_stream,
                                    leftover: Vec::new(),
                                    write_fn: Arc::new(move |payload| {
                                        let socket = socket_write.clone();
                                        Box::pin(async move {
                                            let mut packet = Vec::with_capacity(4 + payload.len());
                                            packet.extend_from_slice(&stream_id.to_be_bytes());
                                            packet.extend_from_slice(&payload);
                                            let _ = socket.send_to(&packet, remote_addr).await;
                                        })
                                    }),
                                };
                                stream_to_send = Some(virtual_stream);
                                tx_stream
                            };
                            (tx, stream_to_send)
                        };

                        if let Some(vs) = stream_to_send {
                            let _ = tx_clone.send(vs).await;
                        }

                        let payload = buf[4..n].to_vec();
                        let _ = tx.send(payload).await;
                    }
                    Err(_) => break,
                }
            }
        });

        Ok((listener, rx))
    }
}

pub struct QuicStream {
    pub remote_addr: SocketAddr,
    read_receiver: Receiver<Vec<u8>>,
    leftover: Vec<u8>,
    write_fn: Arc<dyn Fn(Vec<u8>) -> Pin<Box<dyn std::future::Future<Output = ()> + Send>> + Send + Sync>,
}

impl AsyncRead for QuicStream {
    fn poll_read(
        mut self: Pin<&mut Self>,
        cx: &mut Context<'_>,
        buf: &mut ReadBuf<'_>,
    ) -> Poll<std::io::Result<()>> {
        if !self.leftover.is_empty() {
            let to_read = std::cmp::min(buf.remaining(), self.leftover.len());
            let data = self.leftover.drain(..to_read).collect::<Vec<u8>>();
            buf.put_slice(&data);
            return Poll::Ready(Ok(()));
        }

        match self.read_receiver.poll_recv(cx) {
            Poll::Ready(Some(data)) => {
                let to_read = std::cmp::min(buf.remaining(), data.len());
                buf.put_slice(&data[..to_read]);
                if to_read < data.len() {
                    self.leftover = data[to_read..].to_vec();
                }
                Poll::Ready(Ok(()))
            }
            Poll::Ready(None) => Poll::Ready(Ok(())), // EOF
            Poll::Pending => Poll::Pending,
        }
    }
}

impl AsyncWrite for QuicStream {
    fn poll_write(
        self: Pin<&mut Self>,
        _cx: &mut Context<'_>,
        buf: &[u8],
    ) -> Poll<std::io::Result<usize>> {
        let fut = (self.write_fn)(buf.to_vec());
        tokio::spawn(fut);
        Poll::Ready(Ok(buf.len()))
    }

    fn poll_flush(self: Pin<&mut Self>, _cx: &mut Context<'_>) -> Poll<std::io::Result<()>> {
        Poll::Ready(Ok(()))
    }

    fn poll_shutdown(self: Pin<&mut Self>, _cx: &mut Context<'_>) -> Poll<std::io::Result<()>> {
        Poll::Ready(Ok(()))
    }
}
