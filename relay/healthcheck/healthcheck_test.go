package healthcheck

import (
	"net/url"
	"testing"

	"github.com/netbirdio/netbird/relay/protocol"
)

type fakeServiceChecker struct {
	instanceURL url.URL
	protocols   []protocol.Protocol
}

func (f fakeServiceChecker) ListenerProtocols() []protocol.Protocol {
	return f.protocols
}

func (f fakeServiceChecker) InstanceURL() url.URL {
	return f.instanceURL
}

func TestValidateListenersUsesProbeProtocolsWhenServiceHasNoListeners(t *testing.T) {
	publicURL := url.URL{Scheme: "rel", Host: "public.b32.i2p:80"}
	server, err := NewServer(Config{
		ListenAddress:  "127.0.0.1:0",
		ServiceChecker: fakeServiceChecker{instanceURL: publicURL},
		ProbeProtocols: []protocol.Protocol{protocol.Protocol("ws")},
	})
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	listeners, ok := server.validateListeners()
	if !ok {
		t.Fatal("validateListeners() returned ok=false")
	}
	if len(listeners) != 1 || listeners[0] != protocol.Protocol("ws") {
		t.Fatalf("validateListeners() = %v, want [ws]", listeners)
	}
}

func TestValidateListenersFailsWithoutServiceOrProbeProtocols(t *testing.T) {
	publicURL := url.URL{Scheme: "rel", Host: "public.b32.i2p:80"}
	server, err := NewServer(Config{
		ListenAddress:  "127.0.0.1:0",
		ServiceChecker: fakeServiceChecker{instanceURL: publicURL},
	})
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	if listeners, ok := server.validateListeners(); ok || listeners != nil {
		t.Fatalf("validateListeners() = %v, %t; want nil, false", listeners, ok)
	}
}

func TestProbeURLUsesOverrideWhenConfigured(t *testing.T) {
	publicURL := url.URL{Scheme: "rel", Host: "public.b32.i2p:80"}
	probeURL := url.URL{Scheme: "rel", Host: "127.0.0.1:8080"}

	server, err := NewServer(Config{
		ListenAddress:  "127.0.0.1:0",
		ServiceChecker: fakeServiceChecker{instanceURL: publicURL},
		ProbeURL:       &probeURL,
	})
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	if got := server.probeURL(); got.String() != probeURL.String() {
		t.Fatalf("probeURL() = %q, want %q", got.String(), probeURL.String())
	}
}

func TestProbeURLFallsBackToInstanceURL(t *testing.T) {
	publicURL := url.URL{Scheme: "rel", Host: "public.b32.i2p:80"}

	server, err := NewServer(Config{
		ListenAddress:  "127.0.0.1:0",
		ServiceChecker: fakeServiceChecker{instanceURL: publicURL},
	})
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	if got := server.probeURL(); got.String() != publicURL.String() {
		t.Fatalf("probeURL() = %q, want %q", got.String(), publicURL.String())
	}
}
