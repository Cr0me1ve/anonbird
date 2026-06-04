//go:build js

package ws

import (
	"net"

	"github.com/coder/websocket"
)

func createDialOptions(_ string, _ *net.Conn, _, _, _, _ string, _ uint8, _ uint8) *websocket.DialOptions {
	// WASM version doesn't support HTTPClient or custom TLS config.
	return &websocket.DialOptions{}
}
