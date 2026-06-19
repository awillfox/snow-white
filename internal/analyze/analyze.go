package analyze

import (
	"fmt"
	"math"
	"strings"
	"time"

	"snow-white/internal/candle"
	"snow-white/internal/indicator"
	"snow-white/pkg/scale"
)

type Row struct {
	OpenTime time.Time
	Close    int64
	SMA      float64
	EMA      float64
	RSI      float64
}

// Compute returns one Row per candle. A period of 0 disables that indicator
// (its column stays NaN).
func Compute(candles []candle.Candle, smaP, emaP, rsiP int) []Row {
	closes := make([]int64, len(candles))
	for i, c := range candles {
		closes[i] = c.Close
	}
	var sma, ema, rsi []float64
	if smaP > 0 {
		sma = indicator.SMA(closes, smaP)
	}
	if emaP > 0 {
		ema = indicator.EMA(closes, emaP)
	}
	if rsiP > 0 {
		rsi = indicator.RSI(closes, rsiP)
	}
	rows := make([]Row, len(candles))
	for i, c := range candles {
		r := Row{OpenTime: c.OpenTime, Close: c.Close}
		r.SMA = at(sma, i)
		r.EMA = at(ema, i)
		r.RSI = at(rsi, i)
		rows[i] = r
	}
	return rows
}

func at(s []float64, i int) float64 {
	if s == nil {
		return math.NaN()
	}
	return s[i]
}

func FormatCSV(rows []Row) string {
	var b strings.Builder
	b.WriteString("open_time,close,sma,ema,rsi\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "%s,%s,%s,%s,%s\n",
			r.OpenTime.UTC().Format(time.RFC3339),
			scale.Format(r.Close, 2),
			fmtFloat(r.SMA), fmtFloat(r.EMA), fmtFloat(r.RSI),
		)
	}
	return b.String()
}

func fmtFloat(v float64) string {
	if v != v { // NaN
		return ""
	}
	return fmt.Sprintf("%.2f", v/100) // satang-scale indicator -> THB display
}
