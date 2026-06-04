//go:build !js

package ws

import (
	"net"

	"github.com/coder/websocket"
)

func createDialOptions(serverName string, underlyingOut *net.Conn, socks5Proxy, socks5Username, socks5Password string, i2pSAM string, i2pTunnelLength, i2pTunnelQuantity uint8) *websocket.DialOptions {
	return &websocket.DialOptions{
		HTTPClient: httpClientNbDialer(serverName, underlyingOut, socks5Proxy, socks5Username, socks5Password, i2pSAM, i2pTunnelLength, i2pTunnelQuantity),
	}
}
