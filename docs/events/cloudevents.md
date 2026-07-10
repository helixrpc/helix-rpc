# Helix RPC CloudEvents Specification

Helix RPC aims to unify synchronous WebSockets, gRPC, and FlatBuffers with asynchronous event-driven architectures. To standardize asynchronous messaging pushed from the backend to the client, Helix strictly adheres to the CNCF **CloudEvents (v1.0)** specification.

## JSON Payload Schema

When a client establishes a long-lived multiplexed WebSocket, asynchronous events (e.g., pushed from Kafka or RabbitMQ) will arrive at the client utilizing the following JSON schema:

```json
{
  "specversion": "1.0",
  "id": "123e4567-e89b-12d3-a456-426614174000",
  "source": "helix/kafka-bridge",
  "type": "io.helixrpc.kafka.event",
  "subject": "users.profile.updated",
  "datacontenttype": "application/json",
  "time": "2026-07-10T12:00:00Z",
  "data": {
    "user_id": 99,
    "status": "online"
  }
}
```

### Standard Fields:
- **`specversion`**: Always `"1.0"`.
- **`id`**: A globally unique identifier for the event (e.g. UUID or Kafka offset).
- **`source`**: The origin of the event, such as `helix/kafka-bridge` or `helix/rabbitmq-sink`.
- **`type`**: The fully qualified event type (e.g. `io.helixrpc.kafka.event`).
- **`subject`**: (Optional) The specific channel, topic, or entity the event relates to (e.g., the Kafka topic name).
- **`datacontenttype`**: The MIME type of the `data` field. Usually `"application/json"`.
- **`time`**: The RFC3339 timestamp of the event generation.
- **`data`**: The business-specific payload.

## Parsing Events on the Client

Frontend applications using the TypeScript runtime will automatically receive these events through their `Stream` handlers. 

Example TypeScript integration:
```typescript
const stream = await client.openStream("EventBridge");

stream.on("data", (payload) => {
    // payload is guaranteed to match the CloudEvent schema if originating from Helix Bridge
    const event = JSON.parse(new TextDecoder().decode(payload));
    
    if (event.type === "io.helixrpc.kafka.event") {
        console.log(`Received Kafka Event from topic ${event.subject}`);
        console.log(`Data payload:`, event.data);
    }
});
```

By standardizing on CloudEvents, Helix RPC avoids proprietary message envelopes and natively integrates into existing Serverless architectures (e.g., Knative, AWS EventBridge, Azure Event Grid) with zero friction.
