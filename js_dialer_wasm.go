//go:build wasm

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
		return conn, &http.Response{StatusCode: http.StatusSwitchingProtocols}, nil
	case <-conn.closeCh:
		return nil, nil, conn.closeErr
	case <-ctx.Done():
		conn.closeWithError(ctx.Err())
		return nil, nil, ctx.Err()
	}
}

type jsWebSocketConnection struct {
	jsConn js.Value

	readChan chan []byte
	openChan chan struct{}

	closeOnce sync.Once
	closeErr  error
	closeCh   chan struct{}

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

func (c *jsWebSocketConnection) closeWithError(err error) {
	c.closeOnce.Do(func() {
		c.closeErr = err
		close(c.closeCh)
		close(c.readChan)
		if c.jsConn.Truthy() {
			c.jsConn.Call("close", 1000, "Normal Closure")
		}
	})
}

func (c *jsWebSocketConnection) onOpen(this js.Value, args []js.Value) any {
	close(c.openChan)
	return nil
}

func (c *jsWebSocketConnection) onMessage(this js.Value, args []js.Value) any {
	select {
	case <-c.closeCh:
		return nil
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

func (c *jsWebSocketConnection) SetReadDeadline(t time.Time) error                         { return nil }
func (c *jsWebSocketConnection) SetWriteDeadline(t time.Time) error                        { return nil }
func (c *jsWebSocketConnection) SetCloseHandler(handler func(code int, text string) error) {}
