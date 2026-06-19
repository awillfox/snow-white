package analyze

import (
	"strings"
	"testing"
	"time"

	"snow-white/internal/candle"
)

func mkCandles(closes ...int64) []candle.Candle {
	base := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	out := make([]candle.Candle, len(closes))
	for i, c := range closes {
		out[i] = candle.Candle{Symbol: "BTCTHB", OpenTime: base.Add(time.Duration(i) * time.Minute), Close: c}
	}
	return out
}

func TestComputeAlignsAndWarmsUp(t *testing.T) {
	rows := Compute(mkCandles(10, 20, 30, 40), 2, 0, 0)
	if len(rows) != 4 {
		t.Fatalf("len = %d", len(rows))
	}
	if rows[1].SMA != 15 {
		t.Errorf("SMA[1] = %v, want 15", rows[1].SMA)
	}
}

func TestFormatCSVHeader(t *testing.T) {
	out := FormatCSV(Compute(mkCandles(10, 20), 2, 0, 0))
	if !strings.HasPrefix(out, "open_time,close,sma,ema,rsi\n") {
		t.Fatalf("missing header: %q", out)
	}
}

func TestFormatCSVRSINotDividedBy100(t *testing.T) {
	// Strictly increasing closes yield RSI = 100 (all gains, no losses).
	// The RSI column should be "100.00", NOT "1.00" (which is the bug: 100/100=1).
	rows := Compute(mkCandles(10, 20, 30, 40, 50), 0, 0, 2)
	out := FormatCSV(rows)

	// Split output and extract RSI column (index 4) from the last data row.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	lastLine := lines[len(lines)-1]
	fields := strings.Split(lastLine, ",")
	rsiField := fields[4]

	if rsiField != "100.00" {
		t.Errorf("RSI field = %q, want 100.00 (not divided by 100)", rsiField)
	}
}
