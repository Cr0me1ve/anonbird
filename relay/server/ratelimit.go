package server

import (
	"fmt"

	"golang.org/x/time/rate"

	"github.com/netbirdio/netbird/shared/relay/messages"
)

// RateLimitConfig configures a per-peer token bucket for relayed transport data.
type RateLimitConfig struct {
	Enabled        bool `yaml:"enabled"`
	BytesPerSecond int  `yaml:"bytesPerSecond"`
	BurstBytes     int  `yaml:"burstBytes"`
}

func (c RateLimitConfig) Normalize() (RateLimitConfig, error) {
	if !c.Enabled {
		return RateLimitConfig{}, nil
	}

	if c.BytesPerSecond <= 0 {
		return RateLimitConfig{}, fmt.Errorf("rate limit bytesPerSecond must be positive when enabled")
	}

	if c.BurstBytes <= 0 {
		c.BurstBytes = c.BytesPerSecond
	}
	if c.BurstBytes < messages.MaxMessageSize {
		c.BurstBytes = messages.MaxMessageSize
	}

	return c, nil
}

func (c RateLimitConfig) NewLimiter() *rate.Limiter {
	normalized, err := c.Normalize()
	if err != nil || !normalized.Enabled {
		return nil
	}
	return rate.NewLimiter(rate.Limit(normalized.BytesPerSecond), normalized.BurstBytes)
}
