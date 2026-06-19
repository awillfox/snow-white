package cli

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"snow-white/internal/backtest"
	"snow-white/internal/candle"
	"snow-white/internal/config"
	"snow-white/internal/strategy"
	"snow-white/pkg/scale"
)

func newBacktestCmd() *cobra.Command {
	var symbol string
	var fast, slow, feeBps int
	var cashTHB float64
	var fromStr, toStr string

	cmd := &cobra.Command{
		Use:   "backtest",
		Short: "Replay the MA-cross strategy over stored candles",
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

			cs, err := candle.NewStore(pool).List(ctx, symbol, from, to, 100000)
			if err != nil {
				return err
			}
			if len(cs) == 0 {
				return fmt.Errorf("no candles for %s in range", symbol)
			}
			startCash := int64(cashTHB * 100) // THB -> satang
			res := backtest.Run(cs, strategy.NewMACross(fast, slow), startCash, feeBps)

			fmt.Printf("strategy:   macross(%d,%d)\n", fast, slow)
			fmt.Printf("candles:    %d  (%s .. %s)\n", len(cs), cs[0].OpenTime.Format("2006-01-02 15:04"), cs[len(cs)-1].OpenTime.Format("2006-01-02 15:04"))
			fmt.Printf("start cash: %s THB\n", scale.Format(res.StartCash, 2))
			fmt.Printf("end cash:   %s THB\n", scale.Format(res.EndCash, 2))
			fmt.Printf("P&L:        %s THB\n", scale.Format(res.PnL, 2))
			fmt.Printf("trades:     %d  win rate: %.0f%%\n", res.NumTrades, res.WinRate*100)
			fmt.Printf("max draw:   %.1f%%\n", res.MaxDrawdownPct*100)
			return nil
		},
	}
	cmd.Flags().StringVar(&symbol, "symbol", "", "symbol, e.g. BTCTHB")
	cmd.Flags().IntVar(&fast, "fast", 20, "fast SMA period")
	cmd.Flags().IntVar(&slow, "slow", 50, "slow SMA period")
	cmd.Flags().IntVar(&feeBps, "fee-bps", 25, "fee in basis points per trade")
	cmd.Flags().Float64Var(&cashTHB, "cash", 100000, "starting cash in THB")
	cmd.Flags().StringVar(&fromStr, "from", "", "start date YYYY-MM-DD (default: 30d ago)")
	cmd.Flags().StringVar(&toStr, "to", "", "end date YYYY-MM-DD (default: now)")
	return cmd
}
