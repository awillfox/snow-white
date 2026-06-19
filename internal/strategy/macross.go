package strategy

import (
	"fmt"

	"snow-white/internal/candle"
	"snow-white/internal/indicator"
)

// MACross emits Buy when the fast SMA crosses above the slow SMA on the latest
// bar, Sell when it crosses below, Hold otherwise.
type MACross struct {
	Fast int
	Slow int
}

func NewMACross(fast, slow int) MACross { return MACross{Fast: fast, Slow: slow} }

func (m MACross) Name() string { return fmt.Sprintf("macross(%d,%d)", m.Fast, m.Slow) }

func (m MACross) WarmupBars() int { return m.Slow + 1 }

func (m MACross) Evaluate(cs []candle.Candle) Signal {
	if len(cs) < m.WarmupBars() {
		return Signal{Action: Hold, Reason: "warming up"}
	}
	closes := make([]int64, len(cs))
	for i, c := range cs {
		closes[i] = c.Close
	}
	fast := indicator.SMA(closes, m.Fast)
	slow := indicator.SMA(closes, m.Slow)

	n := len(cs) - 1
	curFast, curSlow := fast[n], slow[n]
	prevFast, prevSlow := fast[n-1], slow[n-1]
	if !indicator.IsWarm(prevFast) || !indicator.IsWarm(prevSlow) {
		return Signal{Action: Hold, Reason: "warming up"}
	}

	switch {
	case prevFast <= prevSlow && curFast > curSlow:
		return Signal{Action: Buy, Reason: fmt.Sprintf("fast %.2f crossed above slow %.2f", curFast, curSlow)}
	case prevFast >= prevSlow && curFast < curSlow:
		return Signal{Action: Sell, Reason: fmt.Sprintf("fast %.2f crossed below slow %.2f", curFast, curSlow)}
	default:
		return Signal{Action: Hold, Reason: "no fresh cross"}
	}
}
