package strategy

import "snow-white/internal/candle"

type Action int

const (
	Hold Action = iota
	Buy
	Sell
)

func (a Action) String() string {
	switch a {
	case Buy:
		return "BUY"
	case Sell:
		return "SELL"
	default:
		return "HOLD"
	}
}

type Signal struct {
	Action Action
	Reason string
}

// Strategy evaluates a candle history and emits a signal for the most recent bar.
type Strategy interface {
	Name() string
	WarmupBars() int
	Evaluate(candles []candle.Candle) Signal
}
