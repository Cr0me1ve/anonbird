package grpc

import (
	"testing"
	"time"
)

func TestAnonymousKeepaliveParamsTolerateHighLatencyTransports(t *testing.T) {
	params := anonymousKeepaliveParams()
	if params.Time != 2*time.Minute {
		t.Fatalf("unexpected keepalive time: %s", params.Time)
	}
	if params.Timeout != 2*time.Minute {
		t.Fatalf("unexpected keepalive timeout: %s", params.Timeout)
	}
	if anonymousDialTimeout < 2*time.Minute {
		t.Fatalf("anonymous dial timeout too short: %s", anonymousDialTimeout)
	}
}
