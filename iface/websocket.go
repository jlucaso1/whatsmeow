package iface

import (
	"context"
	"encoding/binary" // <-- Add this import
	"net/http"
	"time"
)

// Constants for WebSocket message types.
// These values are defined in RFC 6455 and are now independent of any specific library.
const (
	BinaryMessage = 2
	CloseMessage  = 8
)

const (
	CloseNormalClosure           = 1000
)

// FormatClosePayload creates a WebSocket close message payload from a code.
// This is a simplified equivalent of gorilla/websocket's FormatCloseMessage(code, "").
func FormatClosePayload(code int) []byte {
	// A close payload must have at least 2 bytes for the code.
	payload := make([]byte, 2)
	binary.BigEndian.PutUint16(payload, uint16(code))
	return payload
}

// WebSocketConnection represents the functionality needed from an active WebSocket connection.
type WebSocketConnection interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	SetCloseHandler(handler func(code int, text string) error)
	Close() error
}

// WebSocketDialer is responsible for creating a WebSocket connection.
type WebSocketDialer interface {
	DialContext(ctx context.Context, urlStr string, requestHeader http.Header) (WebSocketConnection, *http.Response, error)
}