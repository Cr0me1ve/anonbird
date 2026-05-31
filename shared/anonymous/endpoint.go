package anonymous

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/proxy"

	"github.com/netbirdio/netbird/shared/anonymous/i2psam"
)

const (
	EndpointTransportTor = "tor"
	EndpointTransportI2P = "i2p"

	DefaultTorSOCKS5 = "127.0.0.1:9050"
	DefaultI2PSAM    = "127.0.0.1:7656"
)

func EndpointIsAnonymous(endpoint string) bool {
	return EndpointTransport(endpoint) != ""
}

func EndpointTransport(endpoint string) string {
	if parsed, err := url.Parse(endpoint); err == nil && parsed.Hostname() != "" {
		return HostTransport(parsed.Hostname())
	}
	return HostTransport(endpoint)
}

func HostTransport(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	host = strings.TrimSuffix(host, ".")
	if host == "" {
		return ""
	}
	if strings.Contains(host, ":") {
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = strings.Trim(h, "[]")
		}
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		return ""
	}
	switch {
	case strings.HasSuffix(host, ".onion"):
		return EndpointTransportTor
	case strings.HasSuffix(host, ".b32.i2p"):
		return EndpointTransportI2P
	default:
		return ""
	}
}

func HTTPClientForEndpoint(endpoint string, timeout time.Duration) (*http.Client, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	httpTransport := http.DefaultTransport.(*http.Transport).Clone()
	httpTransport.Proxy = nil

	switch EndpointTransport(endpoint) {
	case EndpointTransportTor:
		dialer, err := proxy.SOCKS5("tcp", DefaultTorSOCKS5, nil, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("create SOCKS5 dialer %s: %w", DefaultTorSOCKS5, err)
		}
		contextDialer, ok := dialer.(proxy.ContextDialer)
		if !ok {
			return nil, fmt.Errorf("SOCKS5 dialer %s does not support context dialing", DefaultTorSOCKS5)
		}
		httpTransport.DialContext = contextDialer.DialContext
	case EndpointTransportI2P:
		dialer := i2psam.Dialer{
			SAMAddress:     DefaultI2PSAM,
			Timeout:        timeout,
			TunnelLength:   i2psam.DefaultTunnelLength,
			TunnelQuantity: i2psam.DefaultTunnelQuantity,
		}
		httpTransport.DialContext = dialer.DialContext
	default:
		httpTransport.DialContext = (&net.Dialer{}).DialContext
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: httpTransport,
	}, nil
}
