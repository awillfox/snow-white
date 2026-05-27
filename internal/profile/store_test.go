package profile_test

import (
	"os"
	"path/filepath"
	"testing"

	"snow_white/internal/profile"
)

func TestLoadMissingFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	profiles, err := profile.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 0 {
		t.Errorf("expected empty slice, got %d profiles", len(profiles))
	}
}

func TestSaveAndLoad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	input := []profile.Profile{
		{Name: "dev", Host: "localhost", Port: "5432", User: "pg", Password: "pw", DBName: "db1", SSLMode: "disable"},
	}
	if err := profile.Save(input); err != nil {
		t.Fatal(err)
	}

	loaded, err := profile.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(loaded))
	}
	if loaded[0].Name != "dev" {
		t.Errorf("expected name 'dev', got %q", loaded[0].Name)
	}
	if loaded[0].Password != "pw" {
		t.Errorf("expected password 'pw', got %q", loaded[0].Password)
	}
}

func TestLoadCorruptFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path := filepath.Join(home, ".snow_white", "profiles.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not: valid: yaml: [[["), 0600); err != nil {
		t.Fatal(err)
	}

	profiles, err := profile.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 0 {
		t.Errorf("expected empty on corrupt file, got %d", len(profiles))
	}
}
