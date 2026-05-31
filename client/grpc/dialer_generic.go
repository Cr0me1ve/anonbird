//go:build !js

package grpc

import (
	"context"
	"fmt"
	"net"
	"os/user"
	"runtime"
	"time"

	"golang.org/x/net/proxy"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	nbnet "github.com/netbirdio/netbird/client/net"
	"github.com/netbirdio/netbird/shared/anonymous/i2psam"
)

func WithCustomDialer(_ bool, _ string) grpc.DialOption {
	return grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
		if runtime.GOOS == "linux" {
			currentUser, err := user.Current()
			if err != nil {
				return nil, status.Errorf(codes.FailedPrecondition, "failed to get current user: %v", err)
			}

			// the custom dialer requires root permissions which are not required for use cases run as non-root
			if currentUser.Uid != "0" {
				log.Debug("Not running as root, using standard dialer")
				dialer := &net.Dialer{}
				return dialer.DialContext(ctx, "tcp", addr)
			}
		}

		conn, err := nbnet.NewDialer().DialContext(ctx, "tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("nbnet.NewDialer().DialContext: %w", err)
		}
		return conn, nil
	})
}

func WithSOCKS5Dialer(socks5Proxy string) grpc.DialOption {
	return grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
		dialer, err := proxy.SOCKS5("tcp", socks5Proxy, nil, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("create SOCKS5 dialer %s: %w", socks5Proxy, err)
		}
		contextDialer, ok := dialer.(proxy.ContextDialer)
		if !ok {
			return nil, fmt.Errorf("SOCKS5 dialer %s does not support context dialing", socks5Proxy)
		}
		return contextDialer.DialContext(ctx, "tcp", addr)
	})
}

func WithI2PDialer(i2pSAM string, tunnelLength, tunnelQuantity uint8) grpc.DialOption {
	return WithI2PDialerTimeout(i2pSAM, tunnelLength, tunnelQuantity, 0)
}

func WithI2PDialerTimeout(i2pSAM string, tunnelLength, tunnelQuantity uint8, timeout time.Duration) grpc.DialOption {
	return grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
		return i2psam.Dialer{
			SAMAddress:     i2pSAM,
			Timeout:        timeout,
			TunnelLength:   tunnelLength,
			TunnelQuantity: tunnelQuantity,
		}.DialContext(ctx, "tcp", addr)
	})
}
