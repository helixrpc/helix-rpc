package runtime

import (
	"testing"
)

func TestNewRabbitMQAsyncSink_InvalidURI(t *testing.T) {
	// Provide an invalid URI that fails parsing or connection immediately
	_, err := NewRabbitMQAsyncSink("invalid-uri", "test-exchange")
	if err == nil {
		t.Fatal("Expected error when connecting to an invalid RabbitMQ URI, got nil")
	}
}
