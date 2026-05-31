package anonymous

import (
	"context"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netbirdio/netbird/client/system"
	"github.com/netbirdio/netbird/shared/anonymous/i2psam"
	mgmProto "github.com/netbirdio/netbird/shared/management/proto"
)

func TestHostIsAnonymous(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"exampleabcdefghijklmnop.onion", true},
		{"exampleabcdefghijklmnop.onion:80", true},
		{"example.b32.i2p", true},
		{"api.anonbird.cloud", false},
		{"93.177.116.58", false},
		{"[::1]:443", false},
		{"localhost", false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			require.Equal(t, tt.want, HostIsAnonymous(tt.host))
		})
	}
}

func TestValidateTransportDefaultsTorSOCKS(t *testing.T) {
	cfg := NormalizeTransport(TransportConfig{})

	require.Equal(t, TransportTorRelayOnly, cfg.Type)
	require.Equal(t, DefaultTorSOCKS5, cfg.TorSOCKS5)
	require.True(t, cfg.RequireAnonymous)
	require.NoError(t, ValidateTransport(cfg))
}

func TestValidateTransportDefaultsI2PSAM(t *testing.T) {
	cfg := NormalizeTransport(TransportConfig{Type: TransportI2PDatagram})

	require.Equal(t, TransportI2PDatagram, cfg.Type)
	require.Equal(t, DefaultI2PSAM, cfg.I2PSAM)
	require.Equal(t, uint8(DefaultI2PTunnelLength), cfg.I2PTunnelLength)
	require.Equal(t, uint8(DefaultI2PTunnelQuantity), cfg.I2PTunnelQuantity)
	require.Equal(t, DefaultI2PDaemonMode, cfg.I2PDaemonMode)
	require.Equal(t, DefaultI2PDaemonPath, cfg.I2PDaemonPath)
	require.True(t, cfg.RequireAnonymous)
	require.NoError(t, ValidateTransport(cfg))
}

func TestValidateTransportRejectsInvalidI2PTunnelSettings(t *testing.T) {
	cfg := TransportConfig{Type: TransportI2PDatagram, I2PTunnelLength: 8}
	err := ValidateTransport(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "i2p tunnel length")

	cfg = TransportConfig{Type: TransportI2PDatagram, I2PTunnelQuantity: 17}
	err = ValidateTransport(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "i2p tunnel quantity")
}

func TestValidateTransportRejectsInvalidI2PDaemonMode(t *testing.T) {
	cfg := TransportConfig{Type: TransportI2PDatagram, I2PDaemonMode: "maybe"}

	err := ValidateTransport(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported i2p daemon mode")
}

func TestValidateTransportRejectsIncompleteI2PDestination(t *testing.T) {
	cfg := TransportConfig{
		Type:                 TransportI2PDatagram,
		I2PDestinationPublic: "public-destination",
	}

	err := ValidateTransport(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "i2p destination is incomplete")
}

func TestEnsureI2PDestinationGeneratesPersistentIdentity(t *testing.T) {
	previousGenerator := generateI2PDestination
	t.Cleanup(func() {
		generateI2PDestination = previousGenerator
	})

	var samAddress string
	generateI2PDestination = func(_ context.Context, sam string) (i2psam.Destination, error) {
		samAddress = sam
		return i2psam.Destination{Public: "public-destination", Private: "private-destination"}, nil
	}

	cfg, changed, err := EnsureI2PDestination(context.Background(), TransportConfig{
		Type:   TransportI2PDatagram,
		I2PSAM: "127.0.0.1:17656",
	})
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "127.0.0.1:17656", samAddress)
	require.Equal(t, "public-destination", cfg.I2PDestinationPublic)
	require.Equal(t, "private-destination", cfg.I2PDestinationPrivate)
}

func TestEnsureI2PDestinationKeepsExistingIdentity(t *testing.T) {
	previousGenerator := generateI2PDestination
	t.Cleanup(func() {
		generateI2PDestination = previousGenerator
	})

	generateI2PDestination = func(_ context.Context, _ string) (i2psam.Destination, error) {
		t.Fatal("destination generator must not be called")
		return i2psam.Destination{}, nil
	}

	cfg, changed, err := EnsureI2PDestination(context.Background(), TransportConfig{
		Type:                  TransportI2PDatagram,
		I2PDestinationPublic:  "public-destination",
		I2PDestinationPrivate: "private-destination",
	})
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, "public-destination", cfg.I2PDestinationPublic)
	require.Equal(t, "private-destination", cfg.I2PDestinationPrivate)
}

func TestSanitizeSystemInfo(t *testing.T) {
	info := &system.Info{
		Hostname: "real-host",
		NetworkAddresses: []system.NetworkAddress{
			{NetIP: netip.MustParsePrefix("192.168.1.10/24"), Mac: "aa:bb:cc:dd:ee:ff"},
		},
		SystemSerialNumber: "serial",
		SystemProductName:  "product",
		SystemManufacturer: "vendor",
		Environment:        system.Environment{Cloud: "aws", Platform: "ec2"},
	}

	SanitizeSystemInfo(info, "public-key")

	require.Equal(t, StableHostname("public-key"), info.Hostname)
	require.Empty(t, info.NetworkAddresses)
	require.Empty(t, info.SystemSerialNumber)
	require.Empty(t, info.SystemProductName)
	require.Empty(t, info.SystemManufacturer)
	require.Empty(t, info.Environment.Cloud)
	require.Empty(t, info.Environment.Platform)
	require.True(t, info.AnonymousMode)
}

func TestSetSystemInfoTransport(t *testing.T) {
	info := &system.Info{}

	SetSystemInfoTransport(info, TransportConfig{
		Type:                 TransportI2PDatagram,
		I2PDestinationPublic: "public-destination",
	})

	require.Equal(t, TransportI2PDatagram, info.AnonymousTransport)
	require.Equal(t, "public-destination", info.I2PDestination)
}

func TestValidateNetbirdConfigRejectsSTUNAndClearnetRelay(t *testing.T) {
	err := ValidateNetbirdConfig(&mgmProto.NetbirdConfig{
		Signal: &mgmProto.HostConfig{
			Uri:      "signalexampleabcdefghijklmnop.onion:443",
			Protocol: mgmProto.HostConfig_HTTPS,
		},
		Stuns: []*mgmProto.HostConfig{{Uri: "stun:stun.l.google.com:19302"}},
		Relay: &mgmProto.RelayConfig{Urls: []string{"relayexampleabcdefghijklmnop.onion:443"}},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "STUN")

	err = ValidateNetbirdConfig(&mgmProto.NetbirdConfig{
		Signal: &mgmProto.HostConfig{
			Uri:      "signalexampleabcdefghijklmnop.onion:443",
			Protocol: mgmProto.HostConfig_HTTPS,
		},
		Relay: &mgmProto.RelayConfig{Urls: []string{"relay.netbird.io:443"}},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "relay endpoint")
}

func TestValidateNetbirdConfigForTransportRejectsTransportMismatch(t *testing.T) {
	cfg := &mgmProto.NetbirdConfig{
		Signal: &mgmProto.HostConfig{
			Uri:      "signalexampleabcdefghijklmnop.onion:443",
			Protocol: mgmProto.HostConfig_HTTPS,
		},
		Relay: &mgmProto.RelayConfig{Urls: []string{"relayexampleabcdefghijklmnop.onion:443"}},
	}

	err := ValidateNetbirdConfigForTransport(cfg, TransportConfig{Type: TransportI2PDatagram})
	require.Error(t, err)
	require.Contains(t, err.Error(), "i2p-datagram")

	cfg.Signal.Uri = "signalexample.b32.i2p:443"
	cfg.Relay.Urls = []string{"relayexample.b32.i2p:443"}
	require.NoError(t, ValidateNetbirdConfigForTransport(cfg, TransportConfig{Type: TransportI2PDatagram}))
}

func TestValidateEndpointForTransport(t *testing.T) {
	err := ValidateEndpointForTransport("oauth token", "https://idp.example.com/token", TransportConfig{Type: TransportTorRelayOnly})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not an anonymous")

	err = ValidateEndpointForTransport("oauth token", "https://idpexample.onion/token", TransportConfig{Type: TransportI2PDatagram})
	require.Error(t, err)
	require.Contains(t, err.Error(), TransportI2PDatagram)

	err = ValidateEndpointForTransport("oauth token", "https://idpexample.onion/token", TransportConfig{Type: TransportTorRelayOnly})
	require.NoError(t, err)

	err = ValidateEndpointForTransport("oauth token", "https://idpexample.b32.i2p/token", TransportConfig{Type: TransportI2PDatagram})
	require.NoError(t, err)
}
