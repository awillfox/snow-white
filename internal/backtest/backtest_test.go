package backtest

import (
	"testing"
	"time"

	"snow-white/internal/candle"
	"snow-white/internal/strategy"
)

// scriptStrategy emits a fixed action per bar index, for deterministic tests.
type scriptStrategy struct{ acts []strategy.Action }

func (s scriptStrategy) Name() string     { return "script" }
func (s scriptStrategy) WarmupBars() int   { return 0 }
func (s scriptStrategy) Evaluate(cs []candle.Candle) strategy.Signal {
	i := len(cs) - 1
	if i < len(s.acts) {
		return strategy.Signal{Action: s.acts[i]}
	}
	return strategy.Signal{Action: strategy.Hold}
}

func mk(closes ...int64) []candle.Candle {
	base := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	out := make([]candle.Candle, len(closes))
	for i, c := range closes {
		out[i] = candle.Candle{OpenTime: base.Add(time.Duration(i) * time.Minute), Close: c}
	}
	return out
}

func TestBuyLowSellHighProfit(t *testing.T) {
	// Buy at 100 satang, sell at 200 satang, no fee -> cash doubles.
	cs := mk(100, 200)
	acts := []strategy.Action{strategy.Buy, strategy.Sell}
	res := Run(cs, scriptStrategy{acts: acts}, 1000, 0)

	if res.NumTrades != 2 {
		t.Fatalf("NumTrades = %d, want 2", res.NumTrades)
	}
	if res.EndCash != 2000 {
		t.Fatalf("EndCash = %d, want 2000", res.EndCash)
	}
	if res.PnL != 1000 {
		t.Fatalf("PnL = %d, want 1000", res.PnL)
	}
}

func TestNoTradesKeepsCash(t *testing.T) {
	res := Run(mk(100, 110, 120), scriptStrategy{acts: nil}, 5000, 0)
	if res.EndCash != 5000 || res.NumTrades != 0 {
		t.Fatalf("EndCash=%d NumTrades=%d", res.EndCash, res.NumTrades)
	}
}
