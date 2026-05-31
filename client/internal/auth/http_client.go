package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/net/proxy"

	"github.com/netbirdio/netbird/client/internal/anonymous"
	"github.com/netbirdio/netbird/shared/anonymous/i2psam"
	"github.com/netbirdio/netbird/util/embeddedroots"
)

const (
	defaultProviderHTTPTimeout   = 10 * time.Second
	anonymousProviderHTTPTimeout = 60 * time.Second
)

func newProviderHTTPClient(clientCert *tls.Certificate, timeout time.Duration, dialContext func(context.Context, string, string) (net.Conn, error)) *http.Client {
	httpTransport := http.DefaultTransport.(*http.Transport).Clone()
	httpTransport.MaxIdleConns = 5

	if dialContext != nil {
		httpTransport.Proxy = nil
		httpTransport.DialContext = dialContext
	}

	certPool, err := x509.SystemCertPool()
	if err != nil || certPool == nil {
		log.Debugf("System cert pool not available; falling back to embedded cert, error: %v", err)
		certPool = embeddedroots.Get()
	} else {
		log.Debug("Using system certificate pool.")
	}

	tlsConfig := &tls.Config{
		RootCAs: certPool,
	}
	if clientCert != nil {
		tlsConfig.Certificates = []tls.Certificate{*clientCert}
	}
	httpTransport.TLSClientConfig = tlsConfig

	return &http.Client{
		Timeout:   timeout,
		Transport: httpTransport,
	}
}

func newAnonymousProviderHTTPClient(transport anonymous.TransportConfig, clientCert *tls.Certificate) (*http.Client, error) {
	transport = anonymous.NormalizeTransport(transport)
	if err := anonymous.ValidateTransport(transport); err != nil {
		return nil, err
	}

	switch transport.Type {
	case anonymous.TransportTorRelayOnly:
		dialer, err := proxy.SOCKS5("tcp", transport.TorSOCKS5, nil, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("create SOCKS5 dialer %s: %w", transport.TorSOCKS5, err)
		}
		contextDialer, ok := dialer.(proxy.ContextDialer)
		if !ok {
			return nil, fmt.Errorf("SOCKS5 dialer %s does not support context dialing", transport.TorSOCKS5)
		}
		return newProviderHTTPClient(clientCert, anonymousProviderHTTPTimeout, contextDialer.DialContext), nil
	case anonymous.TransportI2PDatagram:
		dialer := i2psam.Dialer{
			SAMAddress:     transport.I2PSAM,
			Timeout:        anonymousProviderHTTPTimeout,
			TunnelLength:   transport.I2PTunnelLength,
			TunnelQuantity: transport.I2PTunnelQuantity,
		}
		return newProviderHTTPClient(clientCert, anonymousProviderHTTPTimeout, dialer.DialContext), nil
	default:
		return nil, anonymous.Violation(fmt.Sprintf("unsupported anonymous transport %q", transport.Type))
	}
}
