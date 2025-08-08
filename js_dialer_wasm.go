//go:build wasm

package whatsmeow

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"syscall/js"
	"time"

	"go.mau.fi/whatsmeow/iface"
)

// jsWebSocketDialer implements the WebSocketDialer interface using JS bindings.
type jsWebSocketDialer struct{}

func NewJSDialer() iface.WebSocketDialer {
	return &jsWebSocketDialer{}
}

func (d *jsWebSocketDialer) DialContext(ctx context.Context, urlStr string, requestHeader http.Header) (iface.WebSocketConnection, *http.Response, error) {
	conn := newJSWebSocketConnection()

	callbacks := js.ValueOf(map[string]interface{}{
		"onOpen":    js.FuncOf(conn.onOpen),
		"onMessage": js.FuncOf(conn.onMessage),
		"onError":   js.FuncOf(conn.onError),
		"onClose":   js.FuncOf(conn.onClose),
	})

	jsConn := js.Global().Call("dialWebSocket", urlStr, callbacks)
	if !jsConn.Truthy() {
		return nil, nil, errors.New("failed to dial websocket via JS bridge")
	}
	conn.jsConn = jsConn

	// Wait for the connection to be established or for the context to be cancelled.
	select {
	case <-conn.openChan:
		// The response is faked since the JS WebSocket API doesn't expose it.
		return conn, &http.Response{StatusCode: http.StatusSwitchingProtocols}, nil
	case err := <-conn.errChan:
		return nil, nil, err
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
}

// jsWebSocketConnection implements the WebSocketConnection interface.
type jsWebSocketConnection struct {
	jsConn js.Value // The JS object with { writeMessage, close }

	readChan chan []byte
	errChan  chan error
	openChan chan struct{}

	closeOnce sync.Once
	readMutex sync.Mutex
}

func newJSWebSocketConnection() *jsWebSocketConnection {
	return &jsWebSocketConnection{
		readChan: make(chan []byte, 100), // Buffer to hold incoming messages
		errChan:  make(chan error, 1),
		openChan: make(chan struct{}, 1),
	}
}

// Callbacks for JS to invoke
func (c *jsWebSocketConnection) onOpen(this js.Value, args []js.Value) any {
	close(c.openChan)
	return nil
}
func (c *jsWebSocketConnection) onMessage(this js.Value, args []js.Value) any {
	jsData := args[0]
	goBytes := make([]byte, jsData.Get("length").Int())
	js.CopyBytesToGo(goBytes, jsData)
	c.readChan <- goBytes
	return nil
}
func (c *jsWebSocketConnection) onError(this js.Value, args []js.Value) any {
	errStr := args[0].String()
	c.errChan <- errors.New(errStr)
	return nil
}
func (c *jsWebSocketConnection) onClose(this js.Value, args []js.Value) any {
	c.errChan <- errors.New("websocket closed by remote")
	return nil
}

// Interface implementations
func (c *jsWebSocketConnection) ReadMessage() (messageType int, p []byte, err error) {
	c.readMutex.Lock()
	defer c.readMutex.Unlock()
	select {
	case msg := <-c.readChan:
		return iface.BinaryMessage, msg, nil
	case err := <-c.errChan:
		return -1, nil, err
	}
}
func (c *jsWebSocketConnection) WriteMessage(messageType int, data []byte) error {
	jsBuffer := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(jsBuffer, data)
	c.jsConn.Call("writeMessage", jsBuffer)
	return nil
}
func (c *jsWebSocketConnection) Close() error {
	c.closeOnce.Do(func() {
		c.jsConn.Call("close", 1000, "Normal Closure")
		close(c.errChan)
		close(c.readChan)
	})
	return nil
}
func (c *jsWebSocketConnection) SetReadDeadline(t time.Time) error                         { return nil } // No-op in JS
func (c *jsWebSocketConnection) SetWriteDeadline(t time.Time) error                        { return nil } // No-op in JS
func (c *jsWebSocketConnection) SetCloseHandler(handler func(code int, text string) error) {}             // No-op in JS
