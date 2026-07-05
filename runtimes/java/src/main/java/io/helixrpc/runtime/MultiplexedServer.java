package io.helixrpc.runtime;

import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.io.PushbackInputStream;
import java.net.ServerSocket;
import java.net.Socket;
import java.nio.charset.StandardCharsets;

public class MultiplexedServer implements AutoCloseable {
    public interface ConnectionHandler {
        void handle(Socket socket) throws Exception;
    }

    private final ServerSocket serverSocket;
    private final Thread listenThread;
    private volatile boolean running = true;

    public MultiplexedServer(int port) throws IOException {
        this.serverSocket = new ServerSocket(port);
        this.listenThread = new Thread(this::listenLoop);
    }

    public int getPort() {
        return serverSocket.getLocalPort();
    }

    public void start(ConnectionHandler grpcHandler, ConnectionHandler httpHandler) {
        listenThread.start();
    }

    private void listenLoop() {
        while (running) {
            try {
                Socket socket = serverSocket.accept();
                socket.setKeepAlive(true);
                socket.setTcpNoDelay(true);
                socket.setPerformancePreferences(0, 2, 1); // Prioritize low latency (connection time: 0, latency: 2, bandwidth: 1)
                new Thread(() -> {
                    try {
                        PushbackInputStream pbis = new PushbackInputStream(socket.getInputStream(), 8);
                        byte[] peekBytes = new byte[8];
                        int n = pbis.read(peekBytes);
                        if (n > 0) {
                            pbis.unread(peekBytes, 0, n);
                        }

                        if (n >= 4 && new String(peekBytes, 0, 4, StandardCharsets.UTF_8).equals("PRI ")) {
                            // gRPC handler
                            // Stub dispatch or handle
                        } else {
                            // HTTP handler
                            // Stub dispatch or handle
                        }
                        socket.close();
                    } catch (Exception e) {
                        // ignore
                    }
                }).start();
            } catch (IOException e) {
                if (!running) break;
            }
        }
    }

    @Override
    public void close() throws Exception {
        running = false;
        if (serverSocket != null && !serverSocket.isClosed()) {
            serverSocket.close();
        }
        listenThread.interrupt();
    }

    public static void writeSseChunk(Socket socket, String data) throws IOException {
        OutputStream os = socket.getOutputStream();
        String chunk = "data: " + data + "\n\n";
        os.write(chunk.getBytes(StandardCharsets.UTF_8));
        os.flush();
    }
}
