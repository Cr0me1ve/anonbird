package server

import (
	"testing"

	"github.com/netbirdio/netbird/shared/relay/auth/allow"
)

func TestConfigValidateEnablesAnonymousHealthChecksForOnionRelay(t *testing.T) {
	cfg := Config{
		ExposedAddress: "rel://o2n24n6pjl4dkz2i3tlyfov3ozpnwcu4bhy26rtd65stqctn6rg3vpad.onion:80",
		AuthValidator:  &allow.Auth{},
	}
	if err := cfg.validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !cfg.AnonymousHealthChecks {
		t.Fatal("expected onion relay to enable anonymous health checks")
	}
}

func TestConfigValidateKeepsDefaultHealthChecksForDirectRelay(t *testing.T) {
	cfg := Config{
		ExposedAddress: "rel://relay.example.com:80",
		AuthValidator:  &allow.Auth{},
	}
	if err := cfg.validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if cfg.AnonymousHealthChecks {
		t.Fatal("expected direct relay to keep default health checks")
	}
}
