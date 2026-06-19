package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	APIKey          string        `mapstructure:"INVX_APIKEY"`
	Secret          string        `mapstructure:"INVX_SECRET"`
	Host            string        `mapstructure:"INVX_HOST"`
	PSQLURL         string        `mapstructure:"PSQL_URL"`
	Symbols         []string      `mapstructure:"INVX_SYMBOLS"`
	CollectInterval time.Duration `mapstructure:"INVX_COLLECT_INTERVAL"`
	MaxOrder        int64         `mapstructure:"INVX_MAX_ORDER"`
	MaxDaily        int64         `mapstructure:"INVX_MAX_DAILY"`
	MaxLoss         int64         `mapstructure:"INVX_MAX_LOSS"`
	KillFile        string        `mapstructure:"INVX_KILL_FILE"`
	DiscordWebhookURL string        `mapstructure:"DISCORD_BOT_URL"`
}

// Load reads configuration from environment variables, falling back to an
// optional .env file. Environment variables always win.
func Load() (*Config, error) {
	v := viper.New()
	v.SetConfigName(".env")
	v.SetConfigType("env")
	v.AddConfigPath(".")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetDefault("INVX_HOST", "api-dev.innovestxonline.com")
	v.SetDefault("INVX_COLLECT_INTERVAL", "60s")

	_ = v.ReadInConfig() // optional file; env still applies

	// AutomaticEnv does not populate Unmarshal targets unless keys are known.
	for _, k := range []string{
		"INVX_APIKEY", "INVX_SECRET", "INVX_HOST", "PSQL_URL",
		"INVX_SYMBOLS", "INVX_COLLECT_INTERVAL",
		"INVX_MAX_ORDER", "INVX_MAX_DAILY", "INVX_MAX_LOSS", "INVX_KILL_FILE",
		"DISCORD_BOT_URL",
	} {
		_ = v.BindEnv(k)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// INVX_MAX_ORDER / INVX_MAX_DAILY / INVX_MAX_LOSS are entered in THB
	// (human-friendly, matches .env labels). Convert to satang (×100) for all
	// internal comparisons, which use int64 satang throughout.
	cfg.MaxOrder *= 100
	cfg.MaxDaily *= 100
	cfg.MaxLoss *= 100

	return &cfg, nil
}
