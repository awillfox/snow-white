package config

import (
	"testing"
	"time"
)

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("INVX_APIKEY", "pub")
	t.Setenv("INVX_SECRET", "sec")
	t.Setenv("INVX_HOST", "api-dev.innovestxonline.com")
	t.Setenv("PSQL_URL", "postgres://localhost/x")
	t.Setenv("INVX_SYMBOLS", "BTCTHB,ETHTHB")
	t.Setenv("INVX_COLLECT_INTERVAL", "30s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIKey != "pub" || cfg.Secret != "sec" {
		t.Fatalf("key/secret not loaded: %+v", cfg)
	}
	if len(cfg.Symbols) != 2 || cfg.Symbols[0] != "BTCTHB" {
		t.Fatalf("symbols = %v", cfg.Symbols)
	}
	if cfg.CollectInterval != 30*time.Second {
		t.Fatalf("interval = %v", cfg.CollectInterval)
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("INVX_APIKEY", "x")
	t.Setenv("INVX_SECRET", "y")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CollectInterval != 60*time.Second {
		t.Fatalf("default interval = %v, want 60s", cfg.CollectInterval)
	}
}

func TestLoadRiskCaps(t *testing.T) {
	t.Setenv("INVX_APIKEY", "k")
	t.Setenv("INVX_SECRET", "s")
	// Env vars are THB; Load() converts to satang (×100) internally.
	t.Setenv("INVX_MAX_ORDER", "5000")   // 5000 THB → 500000 satang
	t.Setenv("INVX_MAX_DAILY", "50000")  // 50000 THB → 5000000 satang
	t.Setenv("INVX_MAX_LOSS", "10000")   // 10000 THB → 1000000 satang
	t.Setenv("INVX_KILL_FILE", "./.halt")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxOrder != 500000 || cfg.MaxDaily != 5000000 || cfg.MaxLoss != 1000000 {
		t.Fatalf("caps not loaded (want satang 500000/5000000/1000000): %+v", cfg)
	}
	if cfg.KillFile != "./.halt" {
		t.Fatalf("kill file = %q", cfg.KillFile)
	}
}
