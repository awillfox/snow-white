package trader

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKillFileTripped(t *testing.T) {
	if KillFileTripped("") {
		t.Fatal("empty path must be false")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, ".halt")
	if KillFileTripped(p) {
		t.Fatal("absent file must be false")
	}
	if err := os.WriteFile(p, []byte("stop"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !KillFileTripped(p) {
		t.Fatal("present file must be true")
	}
}
