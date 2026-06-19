package cli

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"snow-white/internal/candle"
	"snow-white/internal/collector"
	"snow-white/internal/config"
	"snow-white/internal/invx"
)

func newCollectCmd() *cobra.Command {
	var symbolsFlag []string
	var intervalFlag time.Duration

	cmd := &cobra.Command{
		Use:   "collect",
		Short: "Poll ticker candles into Postgres (daemon)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			symbols := cfg.Symbols
			if len(symbolsFlag) > 0 {
				symbols = symbolsFlag
			}
			if len(symbols) == 0 {
				return fmt.Errorf("no symbols: set INVX_SYMBOLS or --symbols")
			}
			interval := cfg.CollectInterval
			if intervalFlag > 0 {
				interval = intervalFlag
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			pool, err := pgxpool.New(ctx, cfg.PSQLURL)
			if err != nil {
				return fmt.Errorf("connect postgres: %w", err)
			}
			defer pool.Close()

			client := invx.New(cfg.APIKey, cfg.Secret, cfg.Host, nil)
			store := candle.NewStore(pool)
			col := collector.New(client, store, symbols, interval)

			if err := col.Run(ctx); err != nil && err != context.Canceled {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&symbolsFlag, "symbols", nil, "symbols to collect (overrides INVX_SYMBOLS)")
	cmd.Flags().DurationVar(&intervalFlag, "interval", 0, "poll interval (overrides INVX_COLLECT_INTERVAL)")
	return cmd
}
