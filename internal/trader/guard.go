package trader

import (
	"fmt"

	"snow-white/internal/order"
)

type Caps struct {
	MaxOrder int64 // satang per order
	MaxDaily int64 // satang deployed per day
	MaxLoss  int64 // satang daily realized loss before halt
}

type Decision struct {
	Allowed  bool
	Reason   string
	TripHalt bool // caller should persist halted=true
}

// Check evaluates an order of orderValueTHB satang against the day's risk state
// and caps. Order matters: kill switch, loss stop, per-order, daily.
func Check(state order.RiskState, caps Caps, orderValueTHB int64) Decision {
	if state.Halted {
		return Decision{Allowed: false, Reason: "kill switch active"}
	}
	if caps.MaxLoss > 0 && state.LossToday >= caps.MaxLoss {
		return Decision{Allowed: false, Reason: "daily loss stop", TripHalt: true}
	}
	if caps.MaxOrder > 0 && orderValueTHB > caps.MaxOrder {
		return Decision{Allowed: false, Reason: fmt.Sprintf("exceeds per-order cap (%d > %d)", orderValueTHB, caps.MaxOrder)}
	}
	if caps.MaxDaily > 0 && state.SpentToday+orderValueTHB > caps.MaxDaily {
		return Decision{Allowed: false, Reason: "exceeds daily cap"}
	}
	return Decision{Allowed: true}
}
