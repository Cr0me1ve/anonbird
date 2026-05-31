package grpc

import (
	"context"
	"errors"
	"net"
	"time"

	"google.golang.org/grpc"

	"github.com/netbirdio/netbird/util/wsproxy/client"
)

// WithCustomDialer returns a gRPC dial option that uses WebSocket transport for WASM/JS environments.
// The component parameter specifies the WebSocket proxy component path (e.g., "/management", "/signal").
func WithCustomDialer(tlsEnabled bool, component string) grpc.DialOption {
	return client.WithWebSocketDialer(tlsEnabled, component)
}

func WithSOCKS5Dialer(_ string) grpc.DialOption {
	return grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return nil, errors.New("SOCKS5 dialing is not supported in js/wasm builds")
	})
}

func WithI2PDialer(_ string, _, _ uint8) grpc.DialOption {
	return WithI2PDialerTimeout("", 0, 0, 0)
}

func WithI2PDialerTimeout(_ string, _, _ uint8, _ time.Duration) grpc.DialOption {
	return grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return nil, errors.New("I2P SAM dialing is not supported in js/wasm builds")
	})
}
