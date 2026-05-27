package profile_test

import (
	"testing"

	"snow_white/internal/profile"
)

func TestProfileSSLMode(t *testing.T) {
	p := profile.Profile{SSLMode: "require"}
	if p.SSLMode != "require" {
		t.Errorf("expected 'require', got %q", p.SSLMode)
	}
}
