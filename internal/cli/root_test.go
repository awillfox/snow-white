package cli

import "testing"

func TestNewRootCmd(t *testing.T) {
	cmd := NewRootCmd()
	if cmd.Use != "snow-white" {
		t.Fatalf("Use = %q, want snow-white", cmd.Use)
	}
	if !cmd.HasSubCommands() {
		// no subcommands yet is OK for now; just assert it builds
	}
}
