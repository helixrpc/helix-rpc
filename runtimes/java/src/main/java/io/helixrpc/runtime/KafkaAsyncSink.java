package io.helixrpc.runtime;

import org.apache.kafka.clients.producer.KafkaProducer;
import org.apache.kafka.clients.producer.ProducerRecord;
import org.apache.kafka.clients.producer.RecordMetadata;

import java.util.Properties;
import java.util.concurrent.CompletableFuture;

public class KafkaAsyncSink {
    private final KafkaProducer<String, byte[]> producer;

    public KafkaAsyncSink(Properties properties) {
        this.producer = new KafkaProducer<>(properties);
    }

    public CompletableFuture<RecordMetadata> publishAsync(String topic, String key, byte[] value) {
        CompletableFuture<RecordMetadata> future = new CompletableFuture<>();
        producer.send(new ProducerRecord<>(topic, key, value), (metadata, exception) -> {
            if (exception != null) {
                future.completeExceptionally(exception);
            } else {
                future.complete(metadata);
            }
        });
        return future;
    }

    public void close() {
        if (producer != null) {
            producer.close();
        }
    }
}
