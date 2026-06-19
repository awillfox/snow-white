// Package indicator computes technical indicators over satang int64 close prices.
// Output slices align 1:1 with input; warm-up positions are math.NaN().
package indicator

import "math"

func nanSlice(n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	return out
}

// IsWarm reports whether an indicator value is defined (past warm-up).
func IsWarm(v float64) bool { return !math.IsNaN(v) }

// SMA is the simple moving average over `period` values.
func SMA(values []int64, period int) []float64 {
	out := nanSlice(len(values))
	if period <= 0 || len(values) < period {
		return out
	}
	var sum int64
	for i := 0; i < len(values); i++ {
		sum += values[i]
		if i >= period {
			sum -= values[i-period]
		}
		if i >= period-1 {
			out[i] = float64(sum) / float64(period)
		}
	}
	return out
}

// EMA seeds with the SMA of the first `period` values, then applies the standard
// multiplier 2/(period+1).
func EMA(values []int64, period int) []float64 {
	out := nanSlice(len(values))
	if period <= 0 || len(values) < period {
		return out
	}
	k := 2.0 / float64(period+1)
	var seed int64
	for i := 0; i < period; i++ {
		seed += values[i]
	}
	prev := float64(seed) / float64(period)
	out[period-1] = prev
	for i := period; i < len(values); i++ {
		prev = (float64(values[i])-prev)*k + prev
		out[i] = prev
	}
	return out
}

// RSI is Wilder's Relative Strength Index over `period`.
func RSI(values []int64, period int) []float64 {
	out := nanSlice(len(values))
	if period <= 0 || len(values) <= period {
		return out
	}
	var gain, loss float64
	for i := 1; i <= period; i++ {
		ch := float64(values[i] - values[i-1])
		if ch >= 0 {
			gain += ch
		} else {
			loss -= ch
		}
	}
	avgGain := gain / float64(period)
	avgLoss := loss / float64(period)
	out[period] = rsiFrom(avgGain, avgLoss)
	for i := period + 1; i < len(values); i++ {
		ch := float64(values[i] - values[i-1])
		g, l := 0.0, 0.0
		if ch >= 0 {
			g = ch
		} else {
			l = -ch
		}
		avgGain = (avgGain*float64(period-1) + g) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + l) / float64(period)
		out[i] = rsiFrom(avgGain, avgLoss)
	}
	return out
}

func rsiFrom(avgGain, avgLoss float64) float64 {
	if avgLoss == 0 {
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - (100 / (1 + rs))
}
