use lapin::{
    options::*, BasicProperties, Connection, ConnectionProperties, types::FieldTable,
};
use std::sync::Arc;

pub struct RabbitMQAsyncSink {
    connection: Arc<Connection>,
    channel: lapin::Channel,
    exchange: String,
}

impl RabbitMQAsyncSink {
    pub async fn new(amqp_uri: &str, exchange: &str) -> Result<Self, lapin::Error> {
        let conn = Connection::connect(
            amqp_uri,
            ConnectionProperties::default(),
        )
        .await?;

        let channel = conn.create_channel().await?;

        // Declare the exchange if it doesn't exist
        channel
            .exchange_declare(
                exchange.into(),
                lapin::ExchangeKind::Topic,
                ExchangeDeclareOptions {
                    durable: true,
                    ..Default::default()
                },
                FieldTable::default(),
            )
            .await?;

        Ok(Self {
            connection: Arc::new(conn),
            channel,
            exchange: exchange.to_string(),
        })
    }

    pub async fn publish_async(&self, routing_key: &str, payload: &[u8]) -> Result<(), String> {
        let confirm = self.channel
            .basic_publish(
                self.exchange.as_str().into(),
                routing_key.into(),
                BasicPublishOptions::default(),
                payload,
                BasicProperties::default(),
            )
            .await
            .map_err(|e| e.to_string())?;

        let _ = confirm.await.map_err(|e| e.to_string())?;
        Ok(())
    }
}
