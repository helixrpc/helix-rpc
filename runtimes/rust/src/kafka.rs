use rdkafka::config::ClientConfig;
use rdkafka::producer::{FutureProducer, FutureRecord};
use rdkafka::util::Timeout;
use std::time::Duration;

pub struct KafkaAsyncSink {
    producer: FutureProducer,
}

impl KafkaAsyncSink {
    pub fn new(brokers: &str) -> Result<Self, String> {
        let producer: FutureProducer = ClientConfig::new()
            .set("bootstrap.servers", brokers)
            .set("message.timeout.ms", "5000")
            .create()
            .map_err(|e| format!("Producer creation error: {}", e))?;

        Ok(Self { producer })
    }

    pub async fn publish_async(&self, topic: &str, key: &str, payload: &[u8]) -> Result<(), String> {
        let record = FutureRecord::to(topic)
            .payload(payload)
            .key(key);
            
        self.producer
            .send(record, Timeout::After(Duration::from_secs(0)))
            .await
            .map_err(|(e, _)| format!("Failed to enqueue message: {}", e))?;
            
        Ok(())
    }
}
