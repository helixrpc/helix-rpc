package runtime

import (
	"strings"
	"testing"
	"time"
	"net/http/httptest"

	"github.com/gorilla/websocket"
)

type WsEchoMessage struct {
	Text string `json:"text"`
}

func TestWebSocketStream(t *testing.T) {
	// Create a new Helix server
	handler := NewGRPCHandler()

	// Register a streaming method that echoes messages back
	handler.RegisterMethod("/v1.TestService/StreamEcho", MethodInfo{
		IsStreaming: true,
		StreamHandler: func(stream ServerStream) error {
			for {
				var msg WsEchoMessage
				err := stream.Recv(&msg)
				if err != nil {
					// Expected when client disconnects
					return nil
				}
				
				// Echo it back
				msg.Text = "echo: " + msg.Text
				if err := stream.Send(&msg); err != nil {
					return err
				}
			}
		},
	})

	// Start a test server
	server := httptest.NewServer(handler)
	defer server.Close()

	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/v1.TestService/StreamEcho"

	// Connect via WebSocket
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to dial websocket: %v", err)
	}
	defer conn.Close()

	// Send a message
	reqMsg := WsEchoMessage{Text: "hello"}
	err = conn.WriteJSON(reqMsg)
	if err != nil {
		t.Fatalf("Failed to write JSON: %v", err)
	}

	// Receive the echo
	var respMsg WsEchoMessage
	err = conn.ReadJSON(&respMsg)
	if err != nil {
		t.Fatalf("Failed to read JSON: %v", err)
	}

	if respMsg.Text != "echo: hello" {
		t.Errorf("Expected 'echo: hello', got '%s'", respMsg.Text)
	}
}
