package setupkey

import (
	"context"
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want time.Duration
	}{
		{name: "empty", raw: "", want: 0},
		{name: "hours", raw: "48h", want: 48 * time.Hour},
		{name: "days", raw: "30d", want: 30 * 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDuration(tt.raw)
			if err != nil {
				t.Fatalf("parse duration: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %s, got %s", tt.want, got)
			}
		})
	}
}

func TestParseDurationRejectsNonPositive(t *testing.T) {
	for _, raw := range []string{"0s", "-1h", "0d"} {
		if _, err := parseDuration(raw); err == nil {
			t.Fatalf("expected %q to fail", raw)
		}
	}
}

func TestNewBootstrapAccount(t *testing.T) {
	account := newBootstrapAccount(context.Background(), bootstrapOptions{
		accountID:   "anonbird",
		ownerUserID: "owner",
		ownerEmail:  "owner@anonbird.local",
		ownerName:   "Owner",
		domain:      "anonbird.local",
	})

	if account.Id != "anonbird" {
		t.Fatalf("unexpected account ID: %s", account.Id)
	}
	if account.Users["owner"] == nil {
		t.Fatal("expected owner user")
	}
	if _, err := account.GetGroupAll(); err != nil {
		t.Fatalf("expected All group: %v", err)
	}
	if len(account.Policies) != 1 {
		t.Fatalf("expected default policy, got %d", len(account.Policies))
	}
	if account.Settings == nil || account.Settings.Extra == nil || account.Settings.Extra.UserApprovalRequired {
		t.Fatalf("expected user approval to be disabled for bootstrap account")
	}
}

func TestDefaultSetupKeyAutoGroupsUsesAllGroup(t *testing.T) {
	account := newBootstrapAccount(context.Background(), bootstrapOptions{
		accountID:   "anonbird",
		ownerUserID: "owner",
		ownerEmail:  "owner@anonbird.local",
		ownerName:   "Owner",
		domain:      "anonbird.local",
	})
	allGroup, err := account.GetGroupAll()
	if err != nil {
		t.Fatalf("expected All group: %v", err)
	}

	groups := defaultSetupKeyAutoGroups(account)
	if len(groups) != 1 || groups[0] != allGroup.ID {
		t.Fatalf("expected setup key auto group %q, got %v", allGroup.ID, groups)
	}
}
