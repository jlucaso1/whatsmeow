package net

import (
	"context"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"go.mau.fi/whatsmeow/iface"
)

func NewDefaultGorillaDialer() *GorillaDialer {
	return &GorillaDialer{
		Dialer: &websocket.Dialer{},
	}
}

type gorillaConn struct {
	*websocket.Conn
}

var _ iface.WebSocketConnection = (*gorillaConn)(nil)

func (c *gorillaConn) SetReadDeadline(t time.Time) error {
	return c.Conn.SetReadDeadline(t)
}

func (c *gorillaConn) SetWriteDeadline(t time.Time) error {
	return c.Conn.SetWriteDeadline(t)
}

func (c *gorillaConn) SetCloseHandler(handler func(code int, text string) error) {
	c.Conn.SetCloseHandler(handler)
}

type GorillaDialer struct {
	*websocket.Dialer
}

var _ iface.WebSocketDialer = (*GorillaDialer)(nil)

func NewGorillaDialer(dialer *websocket.Dialer) *GorillaDialer {
	return &GorillaDialer{dialer}
}

func (d *GorillaDialer) DialContext(ctx context.Context, urlStr string, requestHeader http.Header) (iface.WebSocketConnection, *http.Response, error) {
	conn, resp, err := d.Dialer.DialContext(ctx, urlStr, requestHeader)
	if err != nil {
		return nil, resp, err
	}
	return &gorillaConn{conn}, resp, nil
}
