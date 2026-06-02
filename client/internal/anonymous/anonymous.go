package anonymous

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/netbirdio/netbird/client/system"
	"github.com/netbirdio/netbird/shared/anonymous/i2psam"
	mgmProto "github.com/netbirdio/netbird/shared/management/proto"
)

const (
	ProjectName = "AnonBird"

	TransportTorRelayOnly = "tor-relay-only"
	TransportI2PDatagram  = "i2p-datagram"

	EndpointTransportTor = "tor"
	EndpointTransportI2P = "i2p"

	DefaultTorSOCKS5 = "127.0.0.1:9050"
	DefaultI2PSAM    = "127.0.0.1:7656"

	DefaultTorDaemonPath = "tor"

	DefaultI2PTunnelLength   = 1
	DefaultI2PTunnelQuantity = 3

	I2PDaemonExternal = "external"
	I2PDaemonAuto     = "auto"
	I2PDaemonManaged  = "managed"

	DefaultI2PDaemonMode = I2PDaemonAuto
	DefaultI2PDaemonPath = "i2pd"
)

type TransportConfig struct {
	Type                  string `json:"type,omitempty"`
	RequireAnonymous      bool   `json:"require_anonymous"`
	TorSOCKS5             string `json:"tor_socks5,omitempty"`
	I2PSAM                string `json:"i2p_sam,omitempty"`
	I2PTunnelLength       uint8  `json:"i2p_tunnel_length,omitempty"`
	I2PTunnelQuantity     uint8  `json:"i2p_tunnel_quantity,omitempty"`
	I2PDestinationPublic  string `json:"i2p_destination_public,omitempty"`
	I2PDestinationPrivate string `json:"i2p_destination_private,omitempty"`
	I2PDaemonMode         string `json:"i2p_daemon_mode,omitempty"`
	I2PDaemonPath         string `json:"i2p_daemon_path,omitempty"`
	I2PDataDir            string `json:"i2p_data_dir,omitempty"`
}

var generateI2PDestination = i2psam.GenerateDestination

func DefaultTransport() TransportConfig {
	return TransportConfig{
		Type:             TransportTorRelayOnly,
		RequireAnonymous: true,
		TorSOCKS5:        DefaultTorSOCKS5,
	}
}

func NormalizeTransport(in TransportConfig) TransportConfig {
	if in.Type == "" {
		in.Type = TransportTorRelayOnly
	}
	in.Type = strings.ToLower(strings.TrimSpace(in.Type))
	in.TorSOCKS5 = strings.TrimSpace(in.TorSOCKS5)
	in.I2PSAM = strings.TrimSpace(in.I2PSAM)
	in.I2PDestinationPublic = strings.TrimSpace(in.I2PDestinationPublic)
	in.I2PDestinationPrivate = strings.TrimSpace(in.I2PDestinationPrivate)
	in.I2PDaemonMode = strings.ToLower(strings.TrimSpace(in.I2PDaemonMode))
	in.I2PDaemonPath = strings.TrimSpace(in.I2PDaemonPath)
	in.I2PDataDir = strings.TrimSpace(in.I2PDataDir)
	in.RequireAnonymous = true

	switch in.Type {
	case TransportTorRelayOnly:
		if in.TorSOCKS5 == "" {
			in.TorSOCKS5 = DefaultTorSOCKS5
		}
	case TransportI2PDatagram:
		if in.I2PSAM == "" {
			in.I2PSAM = DefaultI2PSAM
		}
		if in.I2PTunnelLength == 0 {
			in.I2PTunnelLength = DefaultI2PTunnelLength
		}
		if in.I2PTunnelQuantity == 0 {
			in.I2PTunnelQuantity = DefaultI2PTunnelQuantity
		}
		if in.I2PDaemonMode == "" {
			in.I2PDaemonMode = DefaultI2PDaemonMode
		}
		if in.I2PDaemonPath == "" {
			in.I2PDaemonPath = DefaultI2PDaemonPath
		}
	}

	return in
}

func ValidateTransport(c TransportConfig) error {
	c = NormalizeTransport(c)
	if !c.RequireAnonymous {
		return Violation("anonymous transport is not required")
	}

	switch c.Type {
	case TransportTorRelayOnly:
		if err := validateHostPort("tor socks5", c.TorSOCKS5); err != nil {
			return err
		}
		if err := validateLoopbackHostPort("tor socks5", c.TorSOCKS5); err != nil {
			return err
		}
	case TransportI2PDatagram:
		if err := validateHostPort("i2p sam", c.I2PSAM); err != nil {
			return err
		}
		if (c.I2PDestinationPublic == "") != (c.I2PDestinationPrivate == "") {
			return Violation("i2p destination is incomplete")
		}
		switch c.I2PDaemonMode {
		case I2PDaemonExternal, I2PDaemonAuto, I2PDaemonManaged:
		default:
			return Violation(fmt.Sprintf("unsupported i2p daemon mode %q", c.I2PDaemonMode))
		}
		if c.I2PTunnelLength > 7 {
			return Violation(fmt.Sprintf("i2p tunnel length %d is out of range 0..7", c.I2PTunnelLength))
		}
		if c.I2PTunnelQuantity > 16 {
			return Violation(fmt.Sprintf("i2p tunnel quantity %d is out of range 1..16", c.I2PTunnelQuantity))
		}
	default:
		return Violation(fmt.Sprintf("unsupported anonymous transport %q", c.Type))
	}
	return nil
}

func EnsureI2PDestination(ctx context.Context, c TransportConfig) (TransportConfig, bool, error) {
	c = NormalizeTransport(c)
	if c.Type != TransportI2PDatagram {
		return c, false, nil
	}
	if err := ValidateTransport(c); err != nil {
		return c, false, err
	}
	if c.I2PDestinationPublic != "" && c.I2PDestinationPrivate != "" {
		return c, false, nil
	}

	destination, err := generateI2PDestination(ctx, c.I2PSAM)
	if err != nil {
		return c, false, fmt.Errorf("generate i2p SAM destination: %w", err)
	}
	c.I2PDestinationPublic = strings.TrimSpace(destination.Public)
	c.I2PDestinationPrivate = strings.TrimSpace(destination.Private)
	if c.I2PDestinationPublic == "" || c.I2PDestinationPrivate == "" {
		return c, false, Violation("generated i2p destination is incomplete")
	}
	return c, true, nil
}

func Violation(reason string) error {
	return fmt.Errorf("anonymous mode violation: %s. Refusing to start because this may leak the real IP address", reason)
}

func HostIsAnonymous(host string) bool {
	return HostTransport(host) != ""
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

func URLIsAnonymous(u *url.URL) bool {
	if u == nil {
		return false
	}
	return HostIsAnonymous(u.Hostname())
}

func EndpointIsAnonymous(endpoint string) bool {
	return EndpointTransport(endpoint) != ""
}

func EndpointTransport(endpoint string) string {
	if parsed, err := url.Parse(endpoint); err == nil && parsed.Hostname() != "" {
		return HostTransport(parsed.Hostname())
	}
	return HostTransport(endpoint)
}

func ValidateServiceURL(serviceName string, u *url.URL) error {
	if u == nil {
		return Violation(fmt.Sprintf("%s URL is empty", serviceName))
	}
	if !URLIsAnonymous(u) {
		return Violation(fmt.Sprintf("%s URL %q is not an anonymous .onion or .b32.i2p endpoint", serviceName, u.String()))
	}
	return nil
}

func ValidateServiceURLForTransport(serviceName string, u *url.URL, transport TransportConfig) error {
	if err := ValidateServiceURL(serviceName, u); err != nil {
		return err
	}
	return validateEndpointTransport(serviceName+" URL", URLTransport(u), transport)
}

func ValidateEndpointForTransport(serviceName string, endpoint string, transport TransportConfig) error {
	if !EndpointIsAnonymous(endpoint) {
		return Violation(fmt.Sprintf("%s endpoint %q is not an anonymous .onion or .b32.i2p endpoint", serviceName, endpoint))
	}
	return validateEndpointTransport(serviceName+" endpoint", EndpointTransport(endpoint), transport)
}

func URLTransport(u *url.URL) string {
	if u == nil {
		return ""
	}
	return HostTransport(u.Hostname())
}

func ValidateHostConfig(serviceName string, cfg *mgmProto.HostConfig) error {
	if cfg == nil {
		return Violation(fmt.Sprintf("%s endpoint is empty", serviceName))
	}
	if !EndpointIsAnonymous(cfg.GetUri()) {
		return Violation(fmt.Sprintf("%s endpoint %q is not an anonymous .onion or .b32.i2p endpoint", serviceName, cfg.GetUri()))
	}
	return nil
}

func ValidateHostConfigForTransport(serviceName string, cfg *mgmProto.HostConfig, transport TransportConfig) error {
	if err := ValidateHostConfig(serviceName, cfg); err != nil {
		return err
	}
	return validateEndpointTransport(serviceName+" endpoint", EndpointTransport(cfg.GetUri()), transport)
}

func ValidateNetbirdConfig(cfg *mgmProto.NetbirdConfig) error {
	if cfg == nil {
		return Violation("management returned empty network config")
	}
	if err := ValidateHostConfig("signal", cfg.GetSignal()); err != nil {
		return err
	}
	if cfg.GetStuns() != nil && len(cfg.GetStuns()) > 0 {
		return Violation("STUN servers are enabled")
	}
	if cfg.GetTurns() != nil && len(cfg.GetTurns()) > 0 {
		return Violation("TURN servers are enabled")
	}
	for _, relay := range cfg.GetRelay().GetUrls() {
		if !EndpointIsAnonymous(relay) {
			return Violation(fmt.Sprintf("relay endpoint %q is not an anonymous .onion or .b32.i2p endpoint", relay))
		}
	}
	return nil
}

func ValidateNetbirdConfigForTransport(cfg *mgmProto.NetbirdConfig, transport TransportConfig) error {
	if cfg == nil {
		return Violation("management returned empty network config")
	}
	if err := ValidateHostConfigForTransport("signal", cfg.GetSignal(), transport); err != nil {
		return err
	}
	if cfg.GetStuns() != nil && len(cfg.GetStuns()) > 0 {
		return Violation("STUN servers are enabled")
	}
	if cfg.GetTurns() != nil && len(cfg.GetTurns()) > 0 {
		return Violation("TURN servers are enabled")
	}
	for _, relay := range cfg.GetRelay().GetUrls() {
		relayTransport := EndpointTransport(relay)
		if relayTransport == "" {
			return Violation(fmt.Sprintf("relay endpoint %q is not an anonymous .onion or .b32.i2p endpoint", relay))
		}
		if err := validateEndpointTransport("relay endpoint "+relay, relayTransport, transport); err != nil {
			return err
		}
	}
	return nil
}

func validateEndpointTransport(serviceName string, endpointTransport string, transport TransportConfig) error {
	transport = NormalizeTransport(transport)
	var expected string
	switch transport.Type {
	case TransportTorRelayOnly:
		expected = EndpointTransportTor
	case TransportI2PDatagram:
		expected = EndpointTransportI2P
	default:
		return Violation(fmt.Sprintf("unsupported anonymous transport %q", transport.Type))
	}
	if endpointTransport != expected {
		return Violation(fmt.Sprintf("%s uses %s endpoint but anonymous transport is %s", serviceName, transportName(endpointTransport), transport.Type))
	}
	return nil
}

func transportName(endpointTransport string) string {
	switch endpointTransport {
	case EndpointTransportTor:
		return ".onion"
	case EndpointTransportI2P:
		return ".b32.i2p"
	default:
		return "clearnet"
	}
}

func SanitizeSystemInfo(info *system.Info, publicKey string) {
	if info == nil {
		return
	}

	info.Hostname = StableHostname(publicKey)
	info.NetworkAddresses = nil
	info.SystemSerialNumber = ""
	info.SystemProductName = ""
	info.SystemManufacturer = ""
	info.Environment = system.Environment{}
	info.AnonymousMode = true
}

func SetSystemInfoTransport(info *system.Info, transport TransportConfig) {
	if info == nil {
		return
	}

	transport = NormalizeTransport(transport)
	info.AnonymousTransport = transport.Type
	if transport.Type == TransportI2PDatagram {
		info.I2PDestination = transport.I2PDestinationPublic
		return
	}
	info.I2PDestination = ""
}

func StableHostname(publicKey string) string {
	hash := sha256.Sum256([]byte(publicKey))
	return "anonbird-" + hex.EncodeToString(hash[:])[:12]
}

func validateHostPort(name, value string) error {
	if value == "" {
		return Violation(fmt.Sprintf("%s address is empty", name))
	}
	host, port, err := net.SplitHostPort(value)
	if err != nil {
		return Violation(fmt.Sprintf("%s address %q must be host:port", name, value))
	}
	if strings.TrimSpace(host) == "" || strings.TrimSpace(port) == "" {
		return Violation(fmt.Sprintf("%s address %q must include host and port", name, value))
	}
	return nil
}

func validateLoopbackHostPort(name, value string) error {
	host, _, err := net.SplitHostPort(value)
	if err != nil {
		return Violation(fmt.Sprintf("%s address %q must be host:port", name, value))
	}
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if host == "" || strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return Violation(fmt.Sprintf("%s address %q must be local loopback to avoid clearnet proxy leaks", name, value))
	}
	return nil
}
