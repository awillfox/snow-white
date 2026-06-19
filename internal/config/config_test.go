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
