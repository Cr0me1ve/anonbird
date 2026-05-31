package grpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"runtime"
	"time"

	"github.com/cenkalti/backoff/v4"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	"github.com/netbirdio/netbird/util/embeddedroots"
)

const (
	defaultDialTimeout   = 30 * time.Second
	anonymousDialTimeout = 3 * time.Minute
	i2pDialTimeout       = 2 * time.Minute
)

// Backoff returns a backoff configuration for gRPC calls
func Backoff(ctx context.Context) backoff.BackOff {
	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = 10 * time.Second
	b.Clock = backoff.SystemClock
	return backoff.WithContext(b, ctx)
}

// CreateConnection creates a gRPC client connection with the appropriate transport options.
// The component parameter specifies the WebSocket proxy component path (e.g., "/management", "/signal").
func CreateConnection(ctx context.Context, addr string, tlsEnabled bool, component string, extraOpts ...grpc.DialOption) (*grpc.ClientConn, error) {
	return createConnection(ctx, addr, tlsEnabled, WithCustomDialer(tlsEnabled, component), extraOpts...)
}

func CreateConnectionThroughSOCKS5(ctx context.Context, addr string, tlsEnabled bool, component string, socks5Proxy string, extraOpts ...grpc.DialOption) (*grpc.ClientConn, error) {
	return createConnectionWithTimeoutAndKeepalive(ctx, addr, tlsEnabled, anonymousDialTimeout, anonymousKeepaliveParams(), WithSOCKS5Dialer(socks5Proxy), extraOpts...)
}

func CreateConnectionThroughI2P(ctx context.Context, addr string, tlsEnabled bool, component string, i2pSAM string, tunnelLength, tunnelQuantity uint8, extraOpts ...grpc.DialOption) (*grpc.ClientConn, error) {
	return createConnectionWithTimeoutAndKeepalive(ctx, addr, tlsEnabled, i2pDialTimeout, anonymousKeepaliveParams(), WithI2PDialerTimeout(i2pSAM, tunnelLength, tunnelQuantity, i2pDialTimeout), extraOpts...)
}

func createConnection(ctx context.Context, addr string, tlsEnabled bool, dialerOption grpc.DialOption, extraOpts ...grpc.DialOption) (*grpc.ClientConn, error) {
	return createConnectionWithTimeout(ctx, addr, tlsEnabled, defaultDialTimeout, dialerOption, extraOpts...)
}

func createConnectionWithTimeout(ctx context.Context, addr string, tlsEnabled bool, dialTimeout time.Duration, dialerOption grpc.DialOption, extraOpts ...grpc.DialOption) (*grpc.ClientConn, error) {
	return createConnectionWithTimeoutAndKeepalive(ctx, addr, tlsEnabled, dialTimeout, defaultKeepaliveParams(), dialerOption, extraOpts...)
}

func createConnectionWithTimeoutAndKeepalive(ctx context.Context, addr string, tlsEnabled bool, dialTimeout time.Duration, keepaliveParams keepalive.ClientParameters, dialerOption grpc.DialOption, extraOpts ...grpc.DialOption) (*grpc.ClientConn, error) {
	transportOption := grpc.WithTransportCredentials(insecure.NewCredentials())
	// for js, the outer websocket layer takes care of tls
	if tlsEnabled && runtime.GOOS != "js" {
		certPool, err := x509.SystemCertPool()
		if err != nil || certPool == nil {
			log.Debugf("System cert pool not available; falling back to embedded cert, error: %v", err)
			certPool = embeddedroots.Get()
		}

		transportOption = grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			RootCAs: certPool,
		}))
	}

	connCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()

	opts := []grpc.DialOption{
		transportOption,
		dialerOption,
		grpc.WithBlock(),
		grpc.WithKeepaliveParams(keepaliveParams),
	}
	opts = append(opts, extraOpts...)

	conn, err := grpc.DialContext(connCtx, addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("dial context: %w", err)
	}

	return conn, nil
}

func defaultKeepaliveParams() keepalive.ClientParameters {
	return keepalive.ClientParameters{
		Time:    30 * time.Second,
		Timeout: 10 * time.Second,
	}
}

func anonymousKeepaliveParams() keepalive.ClientParameters {
	return keepalive.ClientParameters{
		Time:    2 * time.Minute,
		Timeout: 2 * time.Minute,
	}
}
