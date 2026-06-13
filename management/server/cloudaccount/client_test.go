package cloudaccount

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testInternalToken = "12345678901234567890123456789012"

func TestClientResolve(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/accounts/resolve" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-AnonBird-Cloud-Token") != testInternalToken {
			t.Fatal("missing internal token")
		}
		var request map[string]string
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request["subject"] != "oidc-sub" || request["email"] != "owner@example.com" {
			t.Fatalf("unexpected request payload: %v", request)
		}
		_ = json.NewEncoder(w).Encode(Principal{
			AccountID: "acc_test",
			UserID:    "usr_test",
			Email:     "owner@example.com",
			Role:      "owner",
			Domain:    "acc-test.accounts.anonbird.cloud",
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, testInternalToken, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	principal, err := client.Resolve(context.Background(), "oidc-sub", "owner@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if principal.AccountID != "acc_test" || principal.Domain != "acc-test.accounts.anonbird.cloud" {
		t.Fatalf("unexpected principal: %+v", principal)
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

func TestClientRequiresCompletePrincipal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(Principal{AccountID: "acc_test"})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, testInternalToken, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Resolve(context.Background(), "oidc-sub", "owner@example.com"); err == nil {
		t.Fatal("expected incomplete principal to fail")
	}
}
