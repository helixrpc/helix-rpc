package runtime

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for the unified gateway
	},
}

// WebSocketStream implements ServerStream over a WebSocket connection using JSON
type WebSocketStream struct {
	ctx  context.Context
	conn *websocket.Conn
}

func (s *WebSocketStream) Context() context.Context {
	return s.ctx
}

func (s *WebSocketStream) Recv(v interface{}) error {
	// Read next JSON message from the websocket
	_, data, err := s.conn.ReadMessage()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func (s *WebSocketStream) Send(v interface{}) error {
	// Serialize the struct to JSON and send
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.conn.WriteMessage(websocket.TextMessage, data)
}

// IsWebSocketUpgrade checks if the request is trying to upgrade to WebSockets
func IsWebSocketUpgrade(r *http.Request) bool {
	return websocket.IsWebSocketUpgrade(r)
}

// UpgradeToWebSocket upgrades the HTTP connection and returns a ServerStream
func UpgradeToWebSocket(w http.ResponseWriter, r *http.Request) (ServerStream, error) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	return &WebSocketStream{
		ctx:  r.Context(),
		conn: conn,
	}, nil
}
