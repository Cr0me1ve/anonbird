package cmd

import (
	"strings"
	"testing"

	"github.com/netbirdio/netbird/client/internal/anonymous"
)

func TestParseJoinToken(t *testing.T) {
	token, err := parseJoinToken("anonbird://join?server=http%3A%2F%2Fmanagementexample.onion&setup_key=NB-SETUP-xxxx&transport=tor-relay-only&tor_socks5=127.0.0.1%3A9051&hostname=server-1&profile=prod&service=ssh&dns_labels=db,api")
	if err != nil {
		t.Fatalf("parse join token: %v", err)
	}

	if token.ManagementURL != "http://managementexample.onion" {
		t.Fatalf("unexpected management URL: %s", token.ManagementURL)
	}
	if token.SetupKey != "NB-SETUP-xxxx" {
		t.Fatalf("unexpected setup key: %s", token.SetupKey)
	}
	if token.Transport.Type != anonymous.TransportTorRelayOnly {
		t.Fatalf("unexpected transport: %s", token.Transport.Type)
	}
	if token.Transport.TorSOCKS5 != "127.0.0.1:9051" {
		t.Fatalf("unexpected Tor SOCKS5 address: %s", token.Transport.TorSOCKS5)
	}
	if token.Hostname != "server-1" {
		t.Fatalf("unexpected hostname: %s", token.Hostname)
	}
	if token.Profile != "prod" {
		t.Fatalf("unexpected profile: %s", token.Profile)
	}
	wantLabels := []string{"db", "api", "ssh"}
	if strings.Join(token.DNSLabels, ",") != strings.Join(wantLabels, ",") {
		t.Fatalf("unexpected DNS labels: %v", token.DNSLabels)
	}
}

func TestParseJoinTokenRejectsClearnetManagement(t *testing.T) {
	_, err := parseJoinToken("anonbird://join?server=https%3A%2F%2Fmanagement.netbird.io&setup_key=NB-SETUP-xxxx")
	if err == nil || !strings.Contains(err.Error(), "anonymous mode violation") {
		t.Fatalf("expected anonymous management violation, got %v", err)
	}
}

func TestParseJoinTokenRejectsMissingSetupKey(t *testing.T) {
	_, err := parseJoinToken("anonbird://join?server=http%3A%2F%2Fmanagementexample.onion")
	if err == nil || !strings.Contains(err.Error(), "setup key") {
		t.Fatalf("expected setup key error, got %v", err)
	}
}

func TestParseJoinTokenAcceptsI2P(t *testing.T) {
	token, err := parseJoinToken("anonbird://join?server=http%3A%2F%2Fmanagementexample.b32.i2p&setup_key=NB-SETUP-xxxx&transport=i2p-datagram&i2p_sam=127.0.0.1%3A7657&i2p_tunnel_length=2&i2p_tunnel_quantity=4&i2p_daemon_mode=managed&i2pd_path=%2Fusr%2Flocal%2Fbin%2Fi2pd&i2p_data_dir=%2Ftmp%2Fanonbird-i2pd")
	if err != nil {
		t.Fatalf("parse i2p join token: %v", err)
	}
	if token.Transport.Type != anonymous.TransportI2PDatagram {
		t.Fatalf("unexpected transport: %s", token.Transport.Type)
	}
	if token.Transport.I2PSAM != "127.0.0.1:7657" {
		t.Fatalf("unexpected I2P SAM address: %s", token.Transport.I2PSAM)
	}
	if token.Transport.I2PTunnelLength != 2 {
		t.Fatalf("unexpected I2P tunnel length: %d", token.Transport.I2PTunnelLength)
	}
	if token.Transport.I2PTunnelQuantity != 4 {
		t.Fatalf("unexpected I2P tunnel quantity: %d", token.Transport.I2PTunnelQuantity)
	}
	if token.Transport.I2PDaemonMode != anonymous.I2PDaemonManaged {
		t.Fatalf("unexpected I2P daemon mode: %s", token.Transport.I2PDaemonMode)
	}
	if token.Transport.I2PDaemonPath != "/usr/local/bin/i2pd" {
		t.Fatalf("unexpected i2pd path: %s", token.Transport.I2PDaemonPath)
	}
	if token.Transport.I2PDataDir != "/tmp/anonbird-i2pd" {
		t.Fatalf("unexpected i2pd data dir: %s", token.Transport.I2PDataDir)
	}
}

func TestParseJoinTokenRejectsTransportMismatch(t *testing.T) {
	_, err := parseJoinToken("anonbird://join?server=http%3A%2F%2Fmanagementexample.onion&setup_key=NB-SETUP-xxxx&transport=i2p-datagram")
	if err == nil || !strings.Contains(err.Error(), "uses .onion endpoint") {
		t.Fatalf("expected transport mismatch error, got %v", err)
	}
}

func TestParseJoinTokenRejectsInvalidDNSLabel(t *testing.T) {
	_, err := parseJoinToken("anonbird://join?server=http%3A%2F%2Fmanagementexample.onion&setup_key=NB-SETUP-xxxx&service=bad_label!")
	if err == nil || !strings.Contains(err.Error(), "dns labels") {
		t.Fatalf("expected DNS label error, got %v", err)
	}
}

func TestParseJoinTokenRejectsInvalidI2PTunnel(t *testing.T) {
	_, err := parseJoinToken("anonbird://join?server=http%3A%2F%2Fmanagementexample.b32.i2p&setup_key=NB-SETUP-xxxx&transport=i2p-datagram&i2p_tunnel_length=8")
	if err == nil || !strings.Contains(err.Error(), "i2p tunnel length") {
		t.Fatalf("expected I2P tunnel length error, got %v", err)
	}
}
