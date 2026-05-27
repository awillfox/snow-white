package engine_test

import (
	"testing"

	"snow_white/internal/engine"
)

func TestProbeInvalidHost(t *testing.T) {
	err := engine.Probe("postgres://bad:bad@127.0.0.1:9999/nodb?sslmode=disable")
	if err == nil {
		t.Fatal("expected error for unreachable host, got nil")
	}
}
