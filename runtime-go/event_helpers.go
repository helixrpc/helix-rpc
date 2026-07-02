package runtime

import (
	"context"
)

// Message represents a generic payload sent or received from an event queue (Kafka, RabbitMQ, etc.).
type Message struct {
	Key       []byte
	Value     []byte
	Topic     string
	Timestamp int64
}

// EventProducer defines a clean interface for dispatching streaming events.
type EventProducer interface {
	Produce(ctx context.Context, msg Message) error
	Close() error
}

// EventConsumer defines a clean interface for subscribing to event streaming topics.
type EventConsumer interface {
	Consume(ctx context.Context, topic string, handler func(msg Message) error) error
	Close() error
}
