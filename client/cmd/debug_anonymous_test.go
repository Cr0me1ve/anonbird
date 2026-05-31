package cmd

import (
	"strings"
	"testing"

	"github.com/netbirdio/netbird/client/internal/anonymous"
	"github.com/netbirdio/netbird/client/proto"
)

func TestBuildAnonymousCheckReportOK(t *testing.T) {
	report := buildAnonymousCheckReport(
		&proto.GetConfigResponse{
			AnonymousMode:      true,
			AnonymousTransport: anonymous.TransportTorRelayOnly,
			TorSocks5:          anonymous.DefaultTorSOCKS5,
			ManagementUrl:      "http://managementexample.onion",
		},
		&proto.StatusResponse{
			FullStatus: &proto.FullStatus{
				SignalState:    &proto.SignalState{URL: "grpc://signalexample.onion:443"},
				LocalPeerState: &proto.LocalPeerState{KernelInterface: false},
				Relays: []*proto.RelayState{
					{URI: "rels://relayexample.onion:443"},
				},
				Peers: []*proto.PeerState{
					{ConnStatus: "connected", Relayed: true},
				},
			},
		},
	)

	if !report.OK() {
		t.Fatalf("expected anonymous check to pass, got violations: %v", report.violations)
	}
	output := report.String()
	for _, want := range []string{
		"Anonymous mode: enabled",
		"Management transport: tor",
		"Signal transport: tor",
		"Relay transport: tor",
		"STUN: disabled",
		"ICE: disabled",
		"Direct UDP: disabled",
		"Published endpoints: none",
		"WireGuard mode: userspace",
		"Result: OK",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, output)
		}
	}
}

func TestBuildAnonymousCheckReportI2POK(t *testing.T) {
	report := buildAnonymousCheckReport(
		&proto.GetConfigResponse{
			AnonymousMode:      true,
			AnonymousTransport: anonymous.TransportI2PDatagram,
			I2PSam:             anonymous.DefaultI2PSAM,
			I2PTunnelLength:    anonymous.DefaultI2PTunnelLength,
			I2PTunnelQuantity:  anonymous.DefaultI2PTunnelQuantity,
			I2PDestination:     "public-destination",
			ManagementUrl:      "http://managementexample.b32.i2p",
		},
		&proto.StatusResponse{
			FullStatus: &proto.FullStatus{
				SignalState:    &proto.SignalState{URL: "grpc://signalexample.b32.i2p:443"},
				LocalPeerState: &proto.LocalPeerState{KernelInterface: false},
				Relays: []*proto.RelayState{
					{URI: "rels://relayexample.b32.i2p:443"},
				},
				Peers: []*proto.PeerState{
					{ConnStatus: "connected", Relayed: true},
				},
			},
		},
	)

	if !report.OK() {
		t.Fatalf("expected anonymous i2p check to pass, got violations: %v", report.violations)
	}
	output := report.String()
	for _, want := range []string{
		"Anonymous transport: i2p-datagram",
		"I2P SAM: 127.0.0.1:7656",
		"I2P daemon mode: auto",
		"i2pd path: i2pd",
		"I2P tunnel length: 1",
		"I2P tunnel quantity: 3",
		"I2P destination: registered",
		"Management transport: i2p",
		"Signal transport: i2p",
		"Relay transport: i2p",
		"Result: OK",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, output)
		}
	}
}

func TestBuildAnonymousCheckReportI2PDirectPeerOK(t *testing.T) {
	report := buildAnonymousCheckReport(
		&proto.GetConfigResponse{
			AnonymousMode:      true,
			AnonymousTransport: anonymous.TransportI2PDatagram,
			I2PSam:             anonymous.DefaultI2PSAM,
			I2PTunnelLength:    anonymous.DefaultI2PTunnelLength,
			I2PTunnelQuantity:  anonymous.DefaultI2PTunnelQuantity,
			I2PDestination:     "public-destination",
			ManagementUrl:      "http://managementexample.b32.i2p",
		},
		&proto.StatusResponse{
			FullStatus: &proto.FullStatus{
				SignalState:    &proto.SignalState{URL: "grpc://signalexample.b32.i2p:443"},
				LocalPeerState: &proto.LocalPeerState{KernelInterface: false},
				Relays: []*proto.RelayState{
					{URI: "rels://relayexample.b32.i2p:443"},
				},
				Peers: []*proto.PeerState{
					{
						ConnStatus:                 "connected",
						Relayed:                    false,
						LocalIceCandidateType:      anonymous.TransportI2PDatagram,
						RemoteIceCandidateType:     anonymous.TransportI2PDatagram,
						LocalIceCandidateEndpoint:  anonymous.TransportI2PDatagram,
						RemoteIceCandidateEndpoint: anonymous.TransportI2PDatagram + ":abcdef1234567890",
					},
				},
			},
		},
	)

	if !report.OK() {
		t.Fatalf("expected anonymous i2p direct peer check to pass, got violations: %v", report.violations)
	}
	output := report.String()
	for _, want := range []string{
		"ICE: disabled",
		"Direct UDP: disabled",
		"Clearnet fallback: disabled",
		"Published endpoints: none",
		"Anonymous peer endpoints: i2p-datagram",
		"Result: OK",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, output)
		}
	}
}

func TestBuildAnonymousCheckReportDetectsLeaks(t *testing.T) {
	report := buildAnonymousCheckReport(
		&proto.GetConfigResponse{
			AnonymousMode:      true,
			AnonymousTransport: anonymous.TransportTorRelayOnly,
			TorSocks5:          anonymous.DefaultTorSOCKS5,
			ManagementUrl:      "https://management.netbird.io",
		},
		&proto.StatusResponse{
			FullStatus: &proto.FullStatus{
				SignalState:    &proto.SignalState{URL: "grpc://signal.netbird.io:443"},
				LocalPeerState: &proto.LocalPeerState{KernelInterface: true},
				Relays: []*proto.RelayState{
					{URI: "stun:stun.netbird.io:3478"},
					{URI: "rels://relay.netbird.io:443"},
				},
				Peers: []*proto.PeerState{
					{
						ConnStatus:                 "connected",
						Relayed:                    false,
						LocalIceCandidateType:      "host",
						LocalIceCandidateEndpoint:  "203.0.113.10:51820",
						RemoteIceCandidateEndpoint: "198.51.100.20:51820",
						RemoteIceCandidateType:     "srflx",
					},
				},
			},
		},
	)

	if report.OK() {
		t.Fatal("expected anonymous check to fail")
	}
	output := report.String()
	for _, want := range []string{
		"Management transport: clearnet",
		"Signal transport: clearnet",
		"Relay transport: clearnet",
		"STUN: enabled",
		"ICE: enabled",
		"Direct UDP: enabled",
		"Clearnet fallback: enabled",
		"Published endpoints: 2",
		"WireGuard mode: kernel",
		"Result: FAIL",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, output)
		}
	}
}

func TestBuildAnonymousCheckReportDetectsTransportMismatch(t *testing.T) {
	report := buildAnonymousCheckReport(
		&proto.GetConfigResponse{
			AnonymousMode:      true,
			AnonymousTransport: anonymous.TransportI2PDatagram,
			I2PSam:             anonymous.DefaultI2PSAM,
			ManagementUrl:      "http://managementexample.onion",
		},
		&proto.StatusResponse{
			FullStatus: &proto.FullStatus{
				SignalState:    &proto.SignalState{URL: "grpc://signalexample.onion:443"},
				LocalPeerState: &proto.LocalPeerState{KernelInterface: false},
				Relays: []*proto.RelayState{
					{URI: "rels://relayexample.onion:443"},
				},
				Peers: []*proto.PeerState{
					{ConnStatus: "connected", Relayed: true},
				},
			},
		},
	)

	if report.OK() {
		t.Fatal("expected anonymous check to fail")
	}
	output := report.String()
	for _, want := range []string{
		"management URL uses tor but anonymous transport is i2p-datagram",
		"signal URL uses tor but anonymous transport is i2p-datagram",
		"relay URI uses tor but anonymous transport is i2p-datagram",
		"Result: FAIL",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, output)
		}
	}
}
