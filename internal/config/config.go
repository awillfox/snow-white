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
	} {
		_ = v.BindEnv(k)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &cfg, nil
}
