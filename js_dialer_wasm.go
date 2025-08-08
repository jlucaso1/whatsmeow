// Filename: js_dialer_wasm.go
package whatsmeow

import (
	"context"
	"errors"
	"fmt"
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

	conn.onOpenFunc = js.FuncOf(conn.onOpen)
	conn.onMessageFunc = js.FuncOf(conn.onMessage)
	conn.onErrorFunc = js.FuncOf(conn.onError)
	conn.onCloseFunc = js.FuncOf(conn.onClose)

	callbacks := js.ValueOf(map[string]interface{}{
		"onOpen":    conn.onOpenFunc,
		"onMessage": conn.onMessageFunc,
		"onError":   conn.onErrorFunc,
		"onClose":   conn.onCloseFunc,
	})

	// Ensure callbacks are released when the connection is done to prevent memory leaks
	go func() {
		<-conn.closeCh
		conn.onOpenFunc.Release()
		conn.onMessageFunc.Release()
		conn.onErrorFunc.Release()
		conn.onCloseFunc.Release()
	}()

	jsConn := js.Global().Call("dialWebSocket", urlStr, callbacks)
	if !jsConn.Truthy() {
		return nil, nil, errors.New("failed to dial websocket via JS bridge: dialWebSocket call failed")
	}
	conn.jsConn = jsConn

	select {
	case <-conn.openChan:
		// The response is faked since the JS WebSocket API doesn't expose it.
		return conn, &http.Response{StatusCode: http.StatusSwitchingProtocols}, nil
	case <-conn.closeCh:
		return nil, nil, conn.closeErr
	case <-ctx.Done():
		conn.closeWithError(ctx.Err())
		return nil, nil, ctx.Err()
	}
}

// jsWebSocketConnection implements the WebSocketConnection interface.
type jsWebSocketConnection struct {
	jsConn js.Value

	readChan chan []byte
	openChan chan struct{}

	closeOnce sync.Once
	closeErr  error
	closeCh   chan struct{} // This channel is closed to signal that the connection is down.

	onOpenFunc    js.Func
	onMessageFunc js.Func
	onErrorFunc   js.Func
	onCloseFunc   js.Func
}

func newJSWebSocketConnection() *jsWebSocketConnection {
	return &jsWebSocketConnection{
		readChan: make(chan []byte, 100),
		openChan: make(chan struct{}, 1),
		closeCh:  make(chan struct{}),
	}
}

// closeWithError is the single, thread-safe entry point for shutting down the connection.
func (c *jsWebSocketConnection) closeWithError(err error) {
	c.closeOnce.Do(func() {
		c.closeErr = err
		close(c.closeCh) // Signal closure to all listeners.
		close(c.readChan)
		// Attempt to close the JS websocket gracefully.
		if c.jsConn.Truthy() {
			c.jsConn.Call("close", 1000, "Normal Closure")
		}
	})
}

// Callbacks for JS to invoke
func (c *jsWebSocketConnection) onOpen(this js.Value, args []js.Value) any {
	close(c.openChan)
	return nil
}

func (c *jsWebSocketConnection) onMessage(this js.Value, args []js.Value) any {
	select {
	case <-c.closeCh:
		return nil // Connection is closed, ignore incoming messages
	default:
	}
	jsData := args[0]
	goBytes := make([]byte, jsData.Get("length").Int())
	js.CopyBytesToGo(goBytes, jsData)
	c.readChan <- goBytes
	return nil
}

func (c *jsWebSocketConnection) onError(this js.Value, args []js.Value) any {
	c.closeWithError(fmt.Errorf("websocket error: %s", args[0].String()))
	return nil
}

func (c *jsWebSocketConnection) onClose(this js.Value, args []js.Value) any {
	c.closeWithError(errors.New("websocket closed by remote"))
	return nil
}

// Interface implementations
func (c *jsWebSocketConnection) ReadMessage() (messageType int, p []byte, err error) {
	select {
	case <-c.closeCh:
		return -1, nil, c.closeErr
	case msg, ok := <-c.readChan:
		if !ok {
			return -1, nil, c.closeErr
		}
		return iface.BinaryMessage, msg, nil
	}
}

func (c *jsWebSocketConnection) WriteMessage(messageType int, data []byte) error {
	select {
	case <-c.closeCh:
		return c.closeErr
	default:
		jsBuffer := js.Global().Get("Uint8Array").New(len(data))
		js.CopyBytesToJS(jsBuffer, data)
		c.jsConn.Call("writeMessage", jsBuffer)
		return nil
	}
}

func (c *jsWebSocketConnection) Close() error {
	c.closeWithError(errors.New("connection closed by client"))
	return nil
}

// The following methods are no-ops in a browser environment but are required to fulfill the interface.
func (c *jsWebSocketConnection) SetReadDeadline(t time.Time) error                         { return nil }
func (c *jsWebSocketConnection) SetWriteDeadline(t time.Time) error                        { return nil }
func (c *jsWebSocketConnection) SetCloseHandler(handler func(code int, text string) error) {}
