package engine_test

import (
	"testing"

	"snow_white/internal/engine"
)

func TestNewTablesDiff(t *testing.T) {
	before := map[string]struct{}{
		"users": {},
		"posts": {},
	}
	after := map[string]struct{}{
		"users":    {},
		"posts":    {},
		"comments": {},
	}

	toDelete := engine.DiffTables(before, after)
	if len(toDelete) != 1 {
		t.Fatalf("expected 1 new table, got %v", toDelete)
	}
	if toDelete[0] != "comments" {
		t.Errorf("expected 'comments', got %q", toDelete[0])
	}
}
