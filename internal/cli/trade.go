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
	"snow-white/internal/config"
	"snow-white/internal/invx"
	"snow-white/internal/order"
	"snow-white/internal/strategy"
	"snow-white/internal/trader"
)

func newTradeCmd() *cobra.Command {
	var symbol string
	var fast, slow int
	var live bool
	var buyTHB float64
	var interval time.Duration

	cmd := &cobra.Command{
		Use:   "trade",
		Short: "Run the MA-cross trader (PAPER by default; --live places real orders)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if symbol == "" {
				return fmt.Errorf("--symbol required")
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if interval == 0 {
				interval = cfg.CollectInterval
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			pool, err := pgxpool.New(ctx, cfg.PSQLURL)
			if err != nil {
				return fmt.Errorf("connect postgres: %w", err)
			}
			defer pool.Close()

			client := invx.New(cfg.APIKey, cfg.Secret, cfg.Host, nil)
			candleStore := candle.NewStore(pool)
			orderStore := order.NewStore(pool)
			caps := trader.Caps{MaxOrder: cfg.MaxOrder, MaxDaily: cfg.MaxDaily, MaxLoss: cfg.MaxLoss}
			pipe := trader.NewPipeline(client, orderStore, caps, live, cfg.KillFile, nil)
			strat := strategy.NewMACross(fast, slow)
			tr := trader.NewTrader(candleStore, strat, pipe, orderStore, symbol, int64(buyTHB*100), interval)

			mode := "PAPER"
			if live {
				mode = "LIVE"
			}
			fmt.Printf("trader starting: %s %s caps[order=%d daily=%d loss=%d] kill=%q\n",
				strat.Name(), mode, caps.MaxOrder, caps.MaxDaily, caps.MaxLoss, cfg.KillFile)

			if err := tr.Run(ctx); err != nil && err != context.Canceled {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&symbol, "symbol", "", "symbol, e.g. BTCTHB")
	cmd.Flags().IntVar(&fast, "fast", 20, "fast SMA period")
	cmd.Flags().IntVar(&slow, "slow", 50, "slow SMA period")
	cmd.Flags().BoolVar(&live, "live", false, "place REAL orders (default: paper/dry-run)")
	cmd.Flags().Float64Var(&buyTHB, "buy-thb", 1000, "THB to deploy per Buy signal")
	cmd.Flags().DurationVar(&interval, "interval", 0, "evaluation interval (default: INVX_COLLECT_INTERVAL)")
	return cmd
}
