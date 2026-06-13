package cloudquota

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testInternalToken = "12345678901234567890123456789012"

func TestClientCheckAllowed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-AnonBird-Cloud-Token") != testInternalToken {
			t.Fatal("missing internal token")
		}
		_ = json.NewEncoder(w).Encode(Decision{
			Allowed:  true,
			Resource: ResourceUsers,
			PlanID:   "free",
			Limit:    2,
			Current:  1,
			Delta:    1,
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, testInternalToken, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	decision, err := client.Check(context.Background(), "account-1", ResourceUsers, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed {
		t.Fatal("expected allowed decision")
	}
}

func TestClientCheckLimitExceeded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(Decision{
			Allowed:  false,
			Resource: ResourcePeers,
			PlanID:   "free",
			Limit:    10,
			Current:  10,
			Delta:    1,
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, testInternalToken, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Check(context.Background(), "account-1", ResourcePeers, 10, 1)
	var limitErr ErrLimitExceeded
	if !errors.As(err, &limitErr) {
		t.Fatalf("expected ErrLimitExceeded, got %v", err)
	}
}

func TestClientRejectsPlainHTTPOutsidePrivateHosts(t *testing.T) {
	if _, err := NewClient("http://example.com", testInternalToken, nil); err == nil {
		t.Fatal("expected public HTTP URL to be rejected")
	}
}

func TestClientAllowsPlainHTTPServiceName(t *testing.T) {
	if _, err := NewClient("http://cloud-api:8080", testInternalToken, nil); err != nil {
		t.Fatalf("expected single-label service name to be allowed: %v", err)
	}
}

func TestNewFromEnvRequiresCompleteConfig(t *testing.T) {
	t.Setenv(envAPIURL, "https://anonbird.example")
	t.Setenv(envInternalToken, "")

	if _, err := NewFromEnv(); err == nil {
		t.Fatal("expected incomplete env config to fail")
	}
}
