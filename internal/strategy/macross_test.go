package strategy

import (
	"testing"
	"time"

	"snow-white/internal/candle"
)

func candles(closes ...int64) []candle.Candle {
	base := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	out := make([]candle.Candle, len(closes))
	for i, c := range closes {
		out[i] = candle.Candle{Symbol: "BTCTHB", OpenTime: base.Add(time.Duration(i) * time.Minute), Close: c}
	}
	return out
}

func TestMACrossBuyOnGoldenCross(t *testing.T) {
	// Fast(2) crosses above Slow(3) on the last bar.
	// closes: prior bar fast<=slow, last bar fast>slow.
	cs := candles(50, 40, 30, 20, 60) // fast < slow at bar 3, fast > slow at bar 4
	sig := NewMACross(2, 3).Evaluate(cs)
	if sig.Action != Buy {
		t.Fatalf("Action = %v, want Buy (%s)", sig.Action, sig.Reason)
	}
}

func TestMACrossSellOnDeathCross(t *testing.T) {
	cs := candles(20, 30, 40, 50, 5) // fast > slow at bar 3, fast < slow at bar 4
	sig := NewMACross(2, 3).Evaluate(cs)
	if sig.Action != Sell {
		t.Fatalf("Action = %v, want Sell (%s)", sig.Action, sig.Reason)
	}
}

func TestMACrossHoldWhenNotWarm(t *testing.T) {
	cs := candles(10, 20) // fewer than Slow bars
	sig := NewMACross(2, 3).Evaluate(cs)
	if sig.Action != Hold {
		t.Fatalf("Action = %v, want Hold", sig.Action)
	}
}

func TestMACrossHoldWhenNoCross(t *testing.T) {
	cs := candles(10, 11, 12, 13, 14) // steady trend, fast stays above slow, no fresh cross
	sig := NewMACross(2, 3).Evaluate(cs)
	if sig.Action != Hold {
		t.Fatalf("Action = %v, want Hold (no fresh cross)", sig.Action)
	}
}
