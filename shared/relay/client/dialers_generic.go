//go:build !js

package client

import (
	"fmt"
	"os"
	"strings"

	"github.com/netbirdio/netbird/client/iface"
	"github.com/netbirdio/netbird/shared/relay/client/dialer"
	"github.com/netbirdio/netbird/shared/relay/client/dialer/quic"
	"github.com/netbirdio/netbird/shared/relay/client/dialer/ws"
)

const torSOCKSAuthExtensionUsername = "<torS0X>0"

const envAnonRelayTorSOCKSIsolation = "NB_ANON_RELAY_TOR_SOCKS_ISOLATION"

// getDialers returns the list of dialers to use for connecting to the relay server.
func (c *Client) getDialers() []dialer.DialeFn {
	if c.socks5Proxy != "" {
		c.log.Infof("anonymous relay transport enabled, forcing WebSocket over SOCKS5")
		wsDialer := ws.Dialer{Socks5Proxy: c.socks5Proxy}
		if torRelaySOCKSIsolationEnabled() {
			wsDialer.Socks5Username = torSOCKSAuthExtensionUsername
			wsDialer.Socks5Password = torRelayIsolationToken(c.relayChannelID)
		}
		return []dialer.DialeFn{wsDialer}
	}
	if c.i2pSAM != "" {
		c.log.Infof("anonymous relay transport enabled, forcing WebSocket over I2P SAM")
		return []dialer.DialeFn{ws.Dialer{
			I2PSAM:            c.i2pSAM,
			I2PTunnelLength:   c.i2pTunnelLength,
			I2PTunnelQuantity: c.i2pTunnelQuantity,
		}}
	}
	if c.mtu > 0 && c.mtu > iface.DefaultMTU {
		c.log.Infof("MTU %d exceeds default (%d), forcing WebSocket transport to avoid DATAGRAM frame size issues", c.mtu, iface.DefaultMTU)
		return []dialer.DialeFn{ws.Dialer{}}
	}
	return []dialer.DialeFn{quic.Dialer{}, ws.Dialer{}}
}

func torRelayIsolationToken(channelID uint32) string {
	return fmt.Sprintf("anonbird-relay-channel-%d", channelID)
}

func torRelaySOCKSIsolationEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envAnonRelayTorSOCKSIsolation))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
