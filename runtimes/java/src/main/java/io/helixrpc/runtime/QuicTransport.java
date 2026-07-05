package io.helixrpc.runtime;

import java.io.IOException;
import java.net.DatagramPacket;
import java.net.DatagramSocket;
import java.net.InetAddress;
import java.net.SocketAddress;
import java.nio.ByteBuffer;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.BlockingQueue;
import java.util.concurrent.LinkedBlockingQueue;

public class QuicTransport {
    public static class QuicVirtualStream {
        private final int streamId;
        private final DatagramSocket socket;
        private final SocketAddress remoteAddr;
        private final BlockingQueue<byte[]> queue = new LinkedBlockingQueue<>();

        public QuicVirtualStream(int streamId, DatagramSocket socket, SocketAddress remoteAddr) {
            this.streamId = streamId;
            this.socket = socket;
            this.remoteAddr = remoteAddr;
        }

        public byte[] read() throws InterruptedException {
            return queue.take();
        }

        public void write(byte[] data) throws IOException {
            ByteBuffer packet = ByteBuffer.allocate(4 + data.length);
            packet.putInt(streamId);
            packet.put(data);
            byte[] bytes = packet.array();
            DatagramPacket dp = new DatagramPacket(bytes, bytes.length, remoteAddr);
            socket.send(dp);
        }

        public void pushData(byte[] data) {
            queue.add(data);
        }
    }

    public static class QuicListener {
        private final DatagramSocket socket;
        private final Map<String, QuicVirtualStream> streams = new ConcurrentHashMap<>();
        private final BlockingQueue<QuicVirtualStream> acceptQueue = new LinkedBlockingQueue<>();
        private final Thread listenThread;
        private volatile boolean running = true;

        public QuicListener(int port) throws IOException {
            this.socket = new DatagramSocket(port);
            this.listenThread = new Thread(this::runLoop);
            this.listenThread.start();
        }

        public int getPort() {
            return socket.getLocalPort();
        }

        public QuicVirtualStream accept() throws InterruptedException {
            return acceptQueue.take();
        }

        public void close() {
            running = false;
            socket.close();
            listenThread.interrupt();
        }

        private void runLoop() {
            byte[] buf = new byte[65535];
            while (running) {
                try {
                    DatagramPacket dp = new DatagramPacket(buf, buf.length);
                    socket.receive(dp);

                    if (dp.getLength() < 4) continue;

                    ByteBuffer temp = ByteBuffer.wrap(dp.getData(), dp.getOffset(), dp.getLength());
                    int streamId = temp.getInt();

                    String key = dp.getSocketAddress().toString() + ":" + streamId;
                    QuicVirtualStream stream = streams.get(key);
                    if (stream == null) {
                        stream = new QuicVirtualStream(streamId, socket, dp.getSocketAddress());
                        streams.put(key, stream);
                        acceptQueue.put(stream);
                    }

                    byte[] payload = new byte[dp.getLength() - 4];
                    temp.get(payload);
                    stream.pushData(payload);
                } catch (Exception e) {
                    if (!running) break;
                }
            }
        }
    }
}
