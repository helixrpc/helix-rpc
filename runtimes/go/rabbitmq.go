package runtime

import (
	"context"
	"fmt"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

// RabbitMQAsyncSink provides a fire-and-forget AMQP publisher
type RabbitMQAsyncSink struct {
	conn     *amqp.Connection
	channel  *amqp.Channel
	exchange string
}

func NewRabbitMQAsyncSink(amqpURI, exchange string) (*RabbitMQAsyncSink, error) {
	conn, err := amqp.Dial(amqpURI)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open a channel: %w", err)
	}

	err = ch.ExchangeDeclare(
		exchange, // name
		"topic",  // type
		true,     // durable
		false,    // auto-deleted
		false,    // internal
		false,    // no-wait
		nil,      // arguments
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to declare an exchange: %w", err)
	}

	return &RabbitMQAsyncSink{
		conn:     conn,
		channel:  ch,
		exchange: exchange,
	}, nil
}

func (s *RabbitMQAsyncSink) PublishAsync(ctx context.Context, routingKey string, payload []byte) error {
	err := s.channel.PublishWithContext(ctx,
		s.exchange,
		routingKey,
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			ContentType:  "application/octet-stream",
			DeliveryMode: amqp.Persistent,
			Body:         payload,
		})
	if err != nil {
		log.Printf("Failed to publish message: %v", err)
		return err
	}
	return nil
}

func (s *RabbitMQAsyncSink) Close() error {
	if s.channel != nil {
		s.channel.Close()
	}
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}
