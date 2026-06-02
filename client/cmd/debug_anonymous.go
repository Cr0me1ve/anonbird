package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	gstatus "google.golang.org/grpc/status"

	"github.com/netbirdio/netbird/client/internal"
	"github.com/netbirdio/netbird/client/internal/anonymous"
	"github.com/netbirdio/netbird/client/internal/profilemanager"
	"github.com/netbirdio/netbird/client/proto"
	"github.com/netbirdio/netbird/shared/anonymous/i2psam"
)

var anonymousCheckCmd = &cobra.Command{
	Use:   "anonymous-check",
	Short: "Check AnonBird anonymous mode leak protections",
	Long:  "Checks the daemon configuration and live status for anonymous-mode transport, ICE, direct UDP, and endpoint publication leaks.",
	RunE:  anonymousCheck,
}

type anonymousCheckReport struct {
	lines      []string
	violations []string
}

func (r anonymousCheckReport) OK() bool {
	return len(r.violations) == 0
}

func (r anonymousCheckReport) String() string {
	var b strings.Builder
	for _, line := range r.lines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if len(r.violations) > 0 {
		b.WriteString("\nViolations:\n")
		for _, violation := range r.violations {
			b.WriteString(" - ")
			b.WriteString(violation)
			b.WriteByte('\n')
		}
	}
	if r.OK() {
		b.WriteString("Result: OK\n")
	} else {
		b.WriteString("Result: FAIL\n")
	}
	return b.String()
}

func anonymousCheck(cmd *cobra.Command, _ []string) error {
	conn, err := getClient(cmd)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := proto.NewDaemonServiceClient(conn)
	cfg, err := client.GetConfig(cmd.Context(), &proto.GetConfigRequest{})
	if err != nil {
		if strings.Contains(gstatus.Convert(err).Message(), "active profile name is empty") {
			cfg, err = client.GetConfig(cmd.Context(), &proto.GetConfigRequest{ProfileName: "default"})
		}
		if err != nil {
			return fmt.Errorf("failed to get daemon config: %v", gstatus.Convert(err).Message())
		}
	}

	status, err := client.Status(cmd.Context(), &proto.StatusRequest{
		GetFullPeerStatus: true,
		ShouldRunProbes:   false,
	})
	if err != nil {
		return fmt.Errorf("failed to get daemon status: %v", gstatus.Convert(err).Message())
	}

	report := buildAnonymousCheckReport(cfg, status)
	appendI2PSAMCheck(cmd.Context(), &report, cfg)
	cmd.Print(report.String())
	if !report.OK() {
		return fmt.Errorf("anonymous check failed: %d violation(s)", len(report.violations))
	}
	return nil
}

func appendI2PSAMCheck(parent context.Context, report *anonymousCheckReport, cfg *proto.GetConfigResponse) {
	if report == nil || cfg == nil || !cfg.GetAnonymousMode() {
		return
	}
	transport := anonymous.NormalizeTransport(anonymous.TransportConfig{
		Type:              cfg.GetAnonymousTransport(),
		I2PSAM:            cfg.GetI2PSam(),
		I2PTunnelLength:   uint8(cfg.GetI2PTunnelLength()),
		I2PTunnelQuantity: uint8(cfg.GetI2PTunnelQuantity()),
		I2PDaemonMode:     cfg.GetI2PDaemonMode(),
		I2PDaemonPath:     cfg.GetI2PdPath(),
		I2PDataDir:        cfg.GetI2PDataDir(),
	})
	if transport.Type != anonymous.TransportI2PDatagram {
		return
	}

	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	if err := i2psam.Check(ctx, transport.I2PSAM); err != nil {
		report.lines = append(report.lines, "I2P SAM reachable: no")
		report.violations = append(report.violations, fmt.Sprintf("I2P SAM bridge %s is not reachable: %v", transport.I2PSAM, err))
		return
	}
	report.lines = append(report.lines, "I2P SAM reachable: yes")
}

func buildAnonymousCheckReport(cfg *proto.GetConfigResponse, status *proto.StatusResponse) anonymousCheckReport {
	report := anonymousCheckReport{}
	addLine := func(label, value string) {
		report.lines = append(report.lines, fmt.Sprintf("%s: %s", label, value))
	}
	addViolation := func(reason string) {
		report.violations = append(report.violations, reason)
	}

	if cfg == nil {
		addViolation("daemon returned empty config")
		addLine("Anonymous mode", "unknown")
		addLine("Result", "FAIL")
		return report
	}

	preEnrollmentDefault := isPreEnrollmentDefaultConfig(cfg, status)
	switch {
	case cfg.GetAnonymousMode():
		addLine("Anonymous mode", "enabled")
	case preEnrollmentDefault:
		addLine("Anonymous mode", "pending enrollment")
	default:
		addLine("Anonymous mode", "disabled")
		addViolation("anonymous_mode is disabled")
	}

	transport := anonymous.NormalizeTransport(anonymous.TransportConfig{
		Type:              cfg.GetAnonymousTransport(),
		TorSOCKS5:         cfg.GetTorSocks5(),
		I2PSAM:            cfg.GetI2PSam(),
		I2PTunnelLength:   uint8(cfg.GetI2PTunnelLength()),
		I2PTunnelQuantity: uint8(cfg.GetI2PTunnelQuantity()),
		I2PDaemonMode:     cfg.GetI2PDaemonMode(),
		I2PDaemonPath:     cfg.GetI2PdPath(),
		I2PDataDir:        cfg.GetI2PDataDir(),
	})
	addLine("Anonymous transport", transport.Type)
	if transport.Type == anonymous.TransportTorRelayOnly {
		addLine("Tor SOCKS5", transport.TorSOCKS5)
	}
	if transport.Type == anonymous.TransportI2PDatagram {
		addLine("I2P SAM", transport.I2PSAM)
		addLine("I2P daemon mode", transport.I2PDaemonMode)
		addLine("i2pd path", transport.I2PDaemonPath)
		if transport.I2PDataDir != "" {
			addLine("i2pd data dir", transport.I2PDataDir)
		}
		addLine("I2P tunnel length", fmt.Sprintf("%d", transport.I2PTunnelLength))
		addLine("I2P tunnel quantity", fmt.Sprintf("%d", transport.I2PTunnelQuantity))
		if cfg.GetI2PDestination() == "" {
			addLine("I2P destination", "missing")
			addViolation("I2P public destination is not registered")
		} else {
			addLine("I2P destination", "registered")
		}
	}
	if err := anonymous.ValidateTransport(transport); err != nil {
		addViolation(err.Error())
	}
	expectedTransport := expectedEndpointTransport(transport)

	managementTransport := classifyAnonymousEndpoint(cfg.GetManagementUrl())
	addLine("Management transport", managementTransport)
	switch {
	case preEnrollmentDefault && managementTransport == "clearnet":
		addLine("Enrollment", "required")
		addLine("Default connection policy", "anonymous tor-relay-only")
	case managementTransport != "tor" && managementTransport != "i2p":
		addViolation("management URL is not an anonymous .onion or .b32.i2p endpoint")
	case expectedTransport != "" && managementTransport != expectedTransport:
		addViolation(fmt.Sprintf("management URL uses %s but anonymous transport is %s", managementTransport, transport.Type))
	}

	fullStatus := status.GetFullStatus()
	signalTransport := "unknown"
	relayTransport := "unknown"
	stunStatus := "disabled"
	iceStatus := "disabled"
	directUDPStatus := "disabled"
	publishedEndpoints := 0
	anonymousPeerEndpoints := 0
	wireGuardMode := "unknown"
	clearnetFallback := "disabled"

	if fullStatus != nil {
		if signalURL := fullStatus.GetSignalState().GetURL(); signalURL != "" {
			signalTransport = classifyAnonymousEndpoint(signalURL)
			if signalTransport != "tor" && signalTransport != "i2p" {
				addViolation("signal URL is not an anonymous .onion or .b32.i2p endpoint")
			} else if expectedTransport != "" && signalTransport != expectedTransport {
				addViolation(fmt.Sprintf("signal URL uses %s but anonymous transport is %s", signalTransport, transport.Type))
			}
		}

		relayTransport = classifyRelayTransports(fullStatus.GetRelays())
		for _, relay := range fullStatus.GetRelays() {
			uri := strings.ToLower(relay.GetURI())
			if strings.HasPrefix(uri, "stun:") || strings.HasPrefix(uri, "stuns:") {
				stunStatus = "enabled"
				addViolation("STUN relay probe is present")
			}
			if relay.GetURI() != "" && classifyAnonymousEndpoint(relay.GetURI()) == "clearnet" {
				addViolation("relay URI is not an anonymous .onion or .b32.i2p endpoint")
			} else if expectedTransport != "" {
				relayTransport := classifyAnonymousEndpoint(relay.GetURI())
				if relayTransport == "tor" || relayTransport == "i2p" {
					if relayTransport != expectedTransport {
						addViolation(fmt.Sprintf("relay URI uses %s but anonymous transport is %s", relayTransport, transport.Type))
					}
				}
			}
		}

		if fullStatus.GetLocalPeerState().GetKernelInterface() {
			wireGuardMode = "kernel"
			directUDPStatus = "enabled"
			addViolation("kernel WireGuard interface is active")
		} else {
			wireGuardMode = "userspace"
		}

		for _, peer := range fullStatus.GetPeers() {
			anonymousI2PDatagram := peerUsesAnonymousI2PDatagramStatus(peer, transport)
			if peer.GetLocalIceCandidateType() != "" || peer.GetRemoteIceCandidateType() != "" {
				if anonymousI2PDatagram {
					anonymousPeerEndpoints++
					continue
				}
				iceStatus = "enabled"
				addViolation("ICE candidate type is present in peer status")
			}
			if peer.GetLocalIceCandidateEndpoint() != "" {
				if anonymousI2PDatagram && isAnonymousI2PDatagramStatusValue(peer.GetLocalIceCandidateEndpoint()) {
					continue
				}
				publishedEndpoints++
				iceStatus = "enabled"
				directUDPStatus = "enabled"
				addViolation("local ICE candidate endpoint is published")
			}
			if peer.GetRemoteIceCandidateEndpoint() != "" {
				if anonymousI2PDatagram && isAnonymousI2PDatagramStatusValue(peer.GetRemoteIceCandidateEndpoint()) {
					continue
				}
				publishedEndpoints++
				iceStatus = "enabled"
				directUDPStatus = "enabled"
				addViolation("remote ICE candidate endpoint is present")
			}
			if peer.GetConnStatus() == "connected" && !peer.GetRelayed() {
				if anonymousI2PDatagram {
					continue
				}
				directUDPStatus = "enabled"
				addViolation("connected peer is not using relay transport")
			}
		}
	}

	if (!preEnrollmentDefault && managementTransport == "clearnet") || signalTransport == "clearnet" || relayTransport == "clearnet" || directUDPStatus == "enabled" {
		clearnetFallback = "enabled"
	}

	addLine("Signal transport", signalTransport)
	addLine("Relay transport", relayTransport)
	addLine("STUN", stunStatus)
	addLine("ICE", iceStatus)
	addLine("Direct UDP", directUDPStatus)
	addLine("Clearnet fallback", clearnetFallback)
	if publishedEndpoints == 0 {
		addLine("Published endpoints", "none")
	} else {
		addLine("Published endpoints", fmt.Sprintf("%d", publishedEndpoints))
	}
	if anonymousPeerEndpoints > 0 {
		addLine("Anonymous peer endpoints", anonymous.TransportI2PDatagram)
	}
	addLine("WireGuard mode", wireGuardMode)

	return report
}

func isPreEnrollmentDefaultConfig(cfg *proto.GetConfigResponse, status *proto.StatusResponse) bool {
	if cfg == nil || status == nil {
		return false
	}
	if cfg.GetAnonymousMode() {
		return false
	}
	if status.GetStatus() != string(internal.StatusNeedsLogin) {
		return false
	}
	if strings.TrimSpace(cfg.GetManagementUrl()) != profilemanager.DefaultManagementURL {
		return false
	}
	fullStatus := status.GetFullStatus()
	if fullStatus == nil {
		return true
	}
	return fullStatus.GetSignalState().GetURL() == "" &&
		len(fullStatus.GetRelays()) == 0 &&
		len(fullStatus.GetPeers()) == 0
}

func peerUsesAnonymousI2PDatagramStatus(peer *proto.PeerState, transport anonymous.TransportConfig) bool {
	if peer == nil || transport.Type != anonymous.TransportI2PDatagram {
		return false
	}

	values := []string{
		peer.GetLocalIceCandidateType(),
		peer.GetRemoteIceCandidateType(),
		peer.GetLocalIceCandidateEndpoint(),
		peer.GetRemoteIceCandidateEndpoint(),
	}
	hasI2PStatus := false
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if !isAnonymousI2PDatagramStatusValue(value) {
			return false
		}
		hasI2PStatus = true
	}
	return hasI2PStatus
}

func isAnonymousI2PDatagramStatusValue(value string) bool {
	value = strings.TrimSpace(value)
	return value == anonymous.TransportI2PDatagram || strings.HasPrefix(value, anonymous.TransportI2PDatagram+":")
}

func classifyRelayTransports(relays []*proto.RelayState) string {
	if len(relays) == 0 {
		return "unknown"
	}
	seen := map[string]struct{}{}
	for _, relay := range relays {
		if relay.GetURI() == "" {
			continue
		}
		seen[classifyAnonymousEndpoint(relay.GetURI())] = struct{}{}
	}
	if len(seen) == 0 {
		return "unknown"
	}
	if len(seen) > 1 {
		return "mixed"
	}
	for transport := range seen {
		return transport
	}
	return "unknown"
}

func classifyAnonymousEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "unknown"
	}
	switch anonymous.EndpointTransport(endpoint) {
	case anonymous.EndpointTransportTor:
		return "tor"
	case anonymous.EndpointTransportI2P:
		return "i2p"
	}
	return "clearnet"
}

func expectedEndpointTransport(transport anonymous.TransportConfig) string {
	switch transport.Type {
	case anonymous.TransportTorRelayOnly:
		return "tor"
	case anonymous.TransportI2PDatagram:
		return "i2p"
	default:
		return ""
	}
}
