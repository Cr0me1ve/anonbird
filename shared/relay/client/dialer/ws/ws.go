package ws

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"github.com/coder/websocket"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/proxy"

	nbnet "github.com/netbirdio/netbird/client/net"
	"github.com/netbirdio/netbird/shared/anonymous/i2psam"
	"github.com/netbirdio/netbird/shared/relay"
	"github.com/netbirdio/netbird/util/embeddedroots"
)

type Dialer struct {
	Socks5Proxy       string
	I2PSAM            string
	I2PTunnelLength   uint8
	I2PTunnelQuantity uint8
}

func (d Dialer) Protocol() string {
	return "WS"
}

func (d Dialer) Dial(ctx context.Context, address, serverName string) (net.Conn, error) {
	wsURL, err := prepareURL(address)
	if err != nil {
		return nil, err
	}

	var underlying net.Conn
	opts := createDialOptions(serverName, &underlying, d.Socks5Proxy, d.I2PSAM, d.I2PTunnelLength, d.I2PTunnelQuantity)

	wsConn, resp, err := websocket.Dial(ctx, wsURL, opts)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		log.Errorf("failed to dial to Relay server '%s': %s", wsURL, err)
		return nil, err
	}
	if resp.Body != nil {
		_ = resp.Body.Close()
	}

	conn := NewConn(wsConn, address, underlying)
	return conn, nil
}

// prepareURL rewrites a rel://host[:port] or rels://host[:port] address into a
// ws://host[:port]/relay or wss://host[:port]/relay URL, preserving any
// non-standard port from the input.
func prepareURL(address string) (string, error) {
	parsed, err := url.Parse(address)
	if err != nil {
		return "", fmt.Errorf("parse relay address %q: %w", address, err)
	}
	switch parsed.Scheme {
	case "rel":
		parsed.Scheme = "ws"
	case "rels":
		parsed.Scheme = "wss"
	default:
		return "", fmt.Errorf("unsupported scheme: %s", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("missing host in relay address %q", address)
	}
	parsed.Path = relay.WebSocketURLPath
	return parsed.String(), nil
}

// httpClientNbDialer builds the http client used by the websocket library.
// underlyingOut, when non-nil, is populated with the raw conn from the
// transport's DialContext so the caller can read its RemoteAddr.
func httpClientNbDialer(serverName string, underlyingOut *net.Conn, socks5Proxy string, i2pSAM string, i2pTunnelLength, i2pTunnelQuantity uint8) *http.Client {
	customDialer := nbnet.NewDialer()
	var socksDialer proxy.ContextDialer
	var socksErr error
	if socks5Proxy != "" {
		dialer, err := proxy.SOCKS5("tcp", socks5Proxy, nil, proxy.Direct)
		if err != nil {
			socksErr = fmt.Errorf("create SOCKS5 dialer %s: %w", socks5Proxy, err)
		} else {
			var ok bool
			socksDialer, ok = dialer.(proxy.ContextDialer)
			if !ok {
				socksErr = fmt.Errorf("SOCKS5 dialer %s does not support context dialing", socks5Proxy)
			}
		}
	}

	certPool, err := x509.SystemCertPool()
	if err != nil || certPool == nil {
		log.Debugf("System cert pool not available; falling back to embedded cert, error: %v", err)
		certPool = embeddedroots.Get()
	}

	customTransport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if socksErr != nil {
				return nil, socksErr
			}
			var c net.Conn
			var err error
			if i2pSAM != "" {
				c, err = i2psam.Dialer{
					SAMAddress:     i2pSAM,
					TunnelLength:   i2pTunnelLength,
					TunnelQuantity: i2pTunnelQuantity,
				}.DialContext(ctx, network, addr)
			} else if socksDialer != nil {
				c, err = socksDialer.DialContext(ctx, network, addr)
			} else {
				c, err = customDialer.DialContext(ctx, network, addr)
			}
			if err == nil && underlyingOut != nil {
				*underlyingOut = c
			}
			return c, err
		},
		TLSClientConfig: &tls.Config{
			RootCAs:    certPool,
			ServerName: serverName,
		},
	}

	return &http.Client{
		Transport: customTransport,
	}
}
