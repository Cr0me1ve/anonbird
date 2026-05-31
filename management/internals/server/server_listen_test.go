package server

import "testing"

func TestListenAddressUsesConfiguredHost(t *testing.T) {
	s := NewServer(&Config{
		MgmtPort:          8080,
		MgmtListenAddress: "127.0.0.1:8080",
	})

	if got := s.managementListenAddress(); got != "127.0.0.1:8080" {
		t.Fatalf("managementListenAddress() = %q, want %q", got, "127.0.0.1:8080")
	}
	if got := s.listenAddressForPort(ManagementLegacyPort); got != "127.0.0.1:33073" {
		t.Fatalf("listenAddressForPort() = %q, want %q", got, "127.0.0.1:33073")
	}
}

func TestListenAddressDefaultsToAllInterfaces(t *testing.T) {
	s := NewServer(&Config{MgmtPort: 8080})

	if got := s.managementListenAddress(); got != ":8080" {
		t.Fatalf("managementListenAddress() = %q, want %q", got, ":8080")
	}
	if got := s.listenAddressForPort(ManagementLegacyPort); got != ":33073" {
		t.Fatalf("listenAddressForPort() = %q, want %q", got, ":33073")
	}
}
