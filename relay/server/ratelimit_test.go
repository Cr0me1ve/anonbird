package server

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/netbirdio/netbird/relay/server/store"
	"github.com/netbirdio/netbird/shared/relay/messages"
)

func TestRateLimitConfigNormalizeDisabled(t *testing.T) {
	cfg, err := (RateLimitConfig{}).Normalize()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Enabled {
		t.Fatal("expected disabled rate limit")
	}
}

func TestRateLimitConfigNormalizeRejectsInvalidRate(t *testing.T) {
	_, err := (RateLimitConfig{Enabled: true}).Normalize()
	if err == nil {
		t.Fatal("expected invalid rate error")
	}
	if !strings.Contains(err.Error(), "bytesPerSecond") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRateLimitConfigNormalizeRaisesBurstToRelayMessageSize(t *testing.T) {
	cfg, err := (RateLimitConfig{
		Enabled:        true,
		BytesPerSecond: 512,
		BurstBytes:     256,
	}).Normalize()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BurstBytes != messages.MaxMessageSize {
		t.Fatalf("expected burst %d, got %d", messages.MaxMessageSize, cfg.BurstBytes)
	}
}

func TestPeerRateLimitWaitsWhenBucketIsEmpty(t *testing.T) {
	peer := NewPeer(nil, messages.HashID("peer-a"), 0, nil, store.NewStore(), store.NewPeerNotifier(), RateLimitConfig{
		Enabled:        true,
		BytesPerSecond: 1,
		BurstBytes:     messages.MaxMessageSize,
	}, false)
	if peer.limiter == nil {
		t.Fatal("expected limiter")
	}
	if !peer.limiter.AllowN(time.Now(), messages.MaxMessageSize) {
		t.Fatal("failed to drain initial burst")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := peer.waitTransportRate(ctx, 1)
	if err == nil {
		t.Fatal("expected context deadline while waiting for rate limit")
	}
}

func TestPeerRateLimitDisabledDoesNotWait(t *testing.T) {
	peer := NewPeer(nil, messages.HashID("peer-a"), 0, nil, store.NewStore(), store.NewPeerNotifier(), RateLimitConfig{}, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := peer.waitTransportRate(ctx, messages.MaxMessageSize); err != nil {
		t.Fatalf("disabled rate limit should not wait: %v", err)
	}
}
