package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
)

// CloudEvent standard structure
type CloudEvent struct {
	SpecVersion     string      `json:"specversion"`
	ID              string      `json:"id"`
	Source          string      `json:"source"`
	Type            string      `json:"type"`
	Subject         string      `json:"subject,omitempty"`
	DataContentType string      `json:"datacontenttype,omitempty"`
	Time            time.Time   `json:"time"`
	Data            interface{} `json:"data"`
}

func main() {
	// Initialize Kafka Reader
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:   []string{"localhost:9092"},
		Topic:     "helix-events",
		Partition: 0,
		MinBytes:  10e3, // 10KB
		MaxBytes:  10e6, // 10MB
	})

	log.Println("Starting Helix Kafka Bridge...")

	for {
		m, err := r.ReadMessage(context.Background())
		if err != nil {
			log.Fatalf("could not read message: %v", err)
			break
		}

		// Transform incoming Kafka message to CloudEvent
		var rawData interface{}
		err = json.Unmarshal(m.Value, &rawData)
		if err != nil {
			log.Printf("Warning: failed to unmarshal JSON payload: %v", err)
			rawData = string(m.Value)
		}

		event := CloudEvent{
			SpecVersion:     "1.0",
			ID:              string(m.Key),
			Source:          "helix/kafka-bridge",
			Type:            "io.helixrpc.kafka.event",
			Subject:         "helix-events",
			DataContentType: "application/json",
			Time:            m.Time,
			Data:            rawData,
		}

		// Serialize to CloudEvent JSON
		ceJSON, err := json.Marshal(event)
		if err != nil {
			log.Printf("Failed to marshal CloudEvent: %v", err)
			continue
		}

		// In a real implementation, we would broadcast `ceJSON` to 
		// connected WebSockets here using our internal multiplexer.
		log.Printf("Broadcasted CloudEvent: %s\n", string(ceJSON))
	}

	if err := r.Close(); err != nil {
		log.Fatal("failed to close reader:", err)
	}
}
