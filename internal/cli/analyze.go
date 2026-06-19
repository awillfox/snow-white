package cli

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"snow-white/internal/analyze"
	"snow-white/internal/candle"
	"snow-white/internal/config"
)

func newAnalyzeCmd() *cobra.Command {
	var symbol, out string
	var smaP, emaP, rsiP int
	var fromStr, toStr string

	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Compute indicators over stored candles (read-only)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if symbol == "" {
				return fmt.Errorf("--symbol required")
			}
			from, to, err := parseRange(fromStr, toStr)
			if err != nil {
				return err
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			pool, err := pgxpool.New(ctx, cfg.PSQLURL)
			if err != nil {
				return fmt.Errorf("connect postgres: %w", err)
			}
			defer pool.Close()

			candles, err := candle.NewStore(pool).List(ctx, symbol, from, to, 100000)
			if err != nil {
				return err
			}
			rows := analyze.Compute(candles, smaP, emaP, rsiP)
			// CSV is the implemented output; table/json formats are deferred (YAGNI).
			fmt.Print(analyze.FormatCSV(rows))
			return nil
		},
	}
	cmd.Flags().StringVar(&symbol, "symbol", "", "symbol, e.g. BTCTHB")
	cmd.Flags().IntVar(&smaP, "sma", 20, "SMA period (0 disables)")
	cmd.Flags().IntVar(&emaP, "ema", 0, "EMA period (0 disables)")
	cmd.Flags().IntVar(&rsiP, "rsi", 0, "RSI period (0 disables)")
	cmd.Flags().StringVar(&fromStr, "from", "", "start date YYYY-MM-DD (default: 30d ago)")
	cmd.Flags().StringVar(&toStr, "to", "", "end date YYYY-MM-DD (default: now)")
	cmd.Flags().StringVar(&out, "out", "csv", "output format: csv (table/json deferred)")
	return cmd
}

func parseRange(fromStr, toStr string) (time.Time, time.Time, error) {
	to := time.Now().UTC()
	from := to.AddDate(0, 0, -30)
	if fromStr != "" {
		t, err := time.Parse("2006-01-02", fromStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parse --from: %w", err)
		}
		from = t
	}
	if toStr != "" {
		t, err := time.Parse("2006-01-02", toStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parse --to: %w", err)
		}
		to = t.Add(24 * time.Hour)
	}
	return from, to, nil
}
