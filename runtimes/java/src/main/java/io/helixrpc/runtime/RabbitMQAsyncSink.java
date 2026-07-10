package io.helixrpc.runtime;

import com.rabbitmq.client.Channel;
import com.rabbitmq.client.Connection;
import com.rabbitmq.client.ConnectionFactory;

import java.io.IOException;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.TimeoutException;

public class RabbitMQAsyncSink {
    private final Connection connection;
    private final Channel channel;

    public RabbitMQAsyncSink(String host, int port) throws IOException, TimeoutException {
        ConnectionFactory factory = new ConnectionFactory();
        factory.setHost(host);
        factory.setPort(port);
        this.connection = factory.newConnection();
        this.channel = connection.createChannel();
    }

    public CompletableFuture<Void> publishAsync(String exchange, String routingKey, byte[] body) {
        return CompletableFuture.runAsync(() -> {
            try {
                channel.basicPublish(exchange, routingKey, null, body);
            } catch (IOException e) {
                throw new RuntimeException(e);
            }
        });
    }

    public void close() throws IOException, TimeoutException {
        if (channel != null) {
            channel.close();
        }
        if (connection != null) {
            connection.close();
        }
    }
}
