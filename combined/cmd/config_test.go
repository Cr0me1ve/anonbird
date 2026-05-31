package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigExplicitEmptyStunPortsDisablesSTUN(t *testing.T) {
	configPath := writeCombinedConfig(t, `
server:
  listenAddress: "127.0.0.1:8080"
  exposedAddress: "http://exampleexampleexampleexampleexampleexampleexampleexampleexampleexampleexampleexampleexampleexampld.onion:80"
  stunPorts: []
  authSecret: "test-secret"
  dataDir: "/tmp/anonbird-test"
  auth:
    issuer: "http://exampleexampleexampleexampleexampleexampleexampleexampleexampleexampleexampleexampleexampleexampld.onion:80/oauth2"
  store:
    engine: "sqlite"
`)

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	if len(cfg.Server.StunPorts) != 0 {
		t.Fatalf("expected explicit empty stunPorts to remain empty, got %v", cfg.Server.StunPorts)
	}
	if cfg.Relay.Stun.Enabled {
		t.Fatalf("expected relay STUN to be disabled")
	}
	if len(cfg.Management.Stuns) != 0 {
		t.Fatalf("expected no management STUN servers to be advertised, got %v", cfg.Management.Stuns)
	}
}

func TestLoadConfigOmittedStunPortsUsesDefault(t *testing.T) {
	configPath := writeCombinedConfig(t, `
server:
  listenAddress: "127.0.0.1:8080"
  exposedAddress: "http://exampleexampleexampleexampleexampleexampleexampleexampleexampleexampleexampleexampleexampleexampld.onion:80"
  authSecret: "test-secret"
  dataDir: "/tmp/anonbird-test"
  auth:
    issuer: "http://exampleexampleexampleexampleexampleexampleexampleexampleexampleexampleexampleexampleexampleexampld.onion:80/oauth2"
  store:
    engine: "sqlite"
`)

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	if len(cfg.Server.StunPorts) != 1 || cfg.Server.StunPorts[0] != 3478 {
		t.Fatalf("expected omitted stunPorts to default to 3478, got %v", cfg.Server.StunPorts)
	}
	if !cfg.Relay.Stun.Enabled {
		t.Fatalf("expected relay STUN to be enabled by default")
	}
	if len(cfg.Management.Stuns) != 1 {
		t.Fatalf("expected one management STUN server to be advertised, got %v", cfg.Management.Stuns)
	}
}

func TestLoadConfigRelayRateLimitPropagatesToRelay(t *testing.T) {
	configPath := writeCombinedConfig(t, `
server:
  listenAddress: "127.0.0.1:8080"
  exposedAddress: "http://exampleexampleexampleexampleexampleexampleexampleexampleexampleexampleexampleexampleexampleexampld.onion:80"
  stunPorts: []
  authSecret: "test-secret"
  dataDir: "/tmp/anonbird-test"
  relayRateLimit:
    enabled: true
    bytesPerSecond: 1048576
    burstBytes: 2097152
  auth:
    issuer: "http://exampleexampleexampleexampleexampleexampleexampleexampleexampleexampleexampleexampleexampleexampld.onion:80/oauth2"
  store:
    engine: "sqlite"
`)

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if !cfg.Relay.RateLimit.Enabled {
		t.Fatal("expected relay rate limit to be enabled")
	}
	if cfg.Relay.RateLimit.BytesPerSecond != 1048576 {
		t.Fatalf("unexpected bytesPerSecond: %d", cfg.Relay.RateLimit.BytesPerSecond)
	}
	if cfg.Relay.RateLimit.BurstBytes != 2097152 {
		t.Fatalf("unexpected burstBytes: %d", cfg.Relay.RateLimit.BurstBytes)
	}
}

func TestLoadConfigAnonymousExposedAddressDisablesExternalChecks(t *testing.T) {
	tests := []struct {
		name           string
		exposedAddress string
		wantDisabled   bool
	}{
		{
			name:           "onion",
			exposedAddress: "http://exampleexampleexampleexampleexampleexampleexampleexampleexampleexampleexampleexampleexampleexampld.onion:80",
			wantDisabled:   true,
		},
		{
			name:           "i2p",
			exposedAddress: "http://abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz.b32.i2p:80",
			wantDisabled:   true,
		},
		{
			name:           "clearnet",
			exposedAddress: "https://vpn.example.com:443",
			wantDisabled:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := writeCombinedConfig(t, `
server:
  listenAddress: "127.0.0.1:8080"
  exposedAddress: "`+tt.exposedAddress+`"
  stunPorts: []
  authSecret: "test-secret"
  dataDir: "/tmp/anonbird-test"
  auth:
    issuer: "`+tt.exposedAddress+`/oauth2"
  store:
    engine: "sqlite"
`)

			cfg, err := LoadConfig(configPath)
			if err != nil {
				t.Fatalf("LoadConfig returned error: %v", err)
			}
			if cfg.Management.DisableVersionCheck != tt.wantDisabled {
				t.Fatalf("DisableVersionCheck = %v, want %v", cfg.Management.DisableVersionCheck, tt.wantDisabled)
			}
			if cfg.Management.DisableAnonymousMetrics != tt.wantDisabled {
				t.Fatalf("DisableAnonymousMetrics = %v, want %v", cfg.Management.DisableAnonymousMetrics, tt.wantDisabled)
			}
			if cfg.Management.DisableGeoliteUpdate != tt.wantDisabled {
				t.Fatalf("DisableGeoliteUpdate = %v, want %v", cfg.Management.DisableGeoliteUpdate, tt.wantDisabled)
			}
		})
	}
}

func TestLoadConfigExplicitDisableVersionCheckPropagates(t *testing.T) {
	configPath := writeCombinedConfig(t, `
server:
  listenAddress: "127.0.0.1:8080"
  exposedAddress: "https://vpn.example.com:443"
  stunPorts: []
  authSecret: "test-secret"
  dataDir: "/tmp/anonbird-test"
  disableVersionCheck: true
  auth:
    issuer: "https://vpn.example.com/oauth2"
  store:
    engine: "sqlite"
`)

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if !cfg.Management.DisableVersionCheck {
		t.Fatalf("expected explicit disableVersionCheck to propagate")
	}
}

func TestHostFromListenAddress(t *testing.T) {
	if got := hostFromListenAddress("127.0.0.1:8080"); got != "127.0.0.1" {
		t.Fatalf("hostFromListenAddress() = %q, want %q", got, "127.0.0.1")
	}
	if got := hostFromListenAddress(":443"); got != "" {
		t.Fatalf("hostFromListenAddress() = %q, want empty host", got)
	}
}

func TestCombinedRelayProbeURL(t *testing.T) {
	tests := []struct {
		name          string
		listenAddress string
		tlsSupport    bool
		want          string
	}{
		{
			name:          "localhost cleartext",
			listenAddress: "127.0.0.1:8080",
			want:          "rel://127.0.0.1:8080",
		},
		{
			name:          "wildcard cleartext",
			listenAddress: ":8080",
			want:          "rel://127.0.0.1:8080",
		},
		{
			name:          "wildcard tls",
			listenAddress: "0.0.0.0:443",
			tlsSupport:    true,
			want:          "rels://127.0.0.1:443",
		},
		{
			name:          "invalid",
			listenAddress: "not-a-hostport",
			want:          "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := combinedRelayProbeURL(tt.listenAddress, tt.tlsSupport)
			if tt.want == "" {
				if got != nil {
					t.Fatalf("combinedRelayProbeURL() = %s, want nil", got.String())
				}
				return
			}
			if got == nil {
				t.Fatal("combinedRelayProbeURL() = nil")
			}
			if got.String() != tt.want {
				t.Fatalf("combinedRelayProbeURL() = %q, want %q", got.String(), tt.want)
			}
		})
	}
}

func writeCombinedConfig(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "combined.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	return path
}
