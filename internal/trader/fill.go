package trader

import (
	"fmt"
	"math"

	"snow-white/internal/order"
)

// computeFill returns the updated position fields and risk delta after a real fill.
//
// # Units
//
// qtyExecuted and pos.Qty are in ×1e8 scaled coin units (e.g. 0.001 BTC = 100_000).
// avgPrice and pos.AvgCost are in satang per WHOLE coin (not per scaled unit).
// The /1e8 division in each multiplication converts (scaled-units × satang-per-coin)
// → satang, e.g.:
//
//	100_000 units × 100_000 satang/coin / 1e8 = 100 satang
//
// All arithmetic is integer satang — no float.
//
// side is the ORDER side ("BUY"/"SELL").
//
// BUY:
//
//	Adds qty to position; recalculates weighted average cost.
//	No PnL realised; lossDelta = 0.
//
// SELL:
//
//	Subtracts qty from position; realises PnL = proceeds − cost.
//	If realised PnL is negative, lossDelta = |realised| (magnitude of the loss).
//	AvgCost is unchanged on a SELL (only realized PnL and qty change).
//
// Returns an error if any intermediate multiplication would overflow int64.
// Callers must treat an error as a signal that the fill cannot be applied safely;
// they should leave the order pending and alert an operator.
func computeFill(pos order.Position, side string, qtyExecuted, avgPrice int64) (newQty, newAvgCost, newRealizedPnl, lossDelta int64, err error) {
	if side == "BUY" {
		// costBefore = pos.Qty * pos.AvgCost / 1e8
		if pos.AvgCost != 0 && pos.Qty > math.MaxInt64/pos.AvgCost {
			return 0, 0, 0, 0, fmt.Errorf("computeFill overflow: pos.Qty(%d) * pos.AvgCost(%d)", pos.Qty, pos.AvgCost)
		}
		costBefore := pos.Qty * pos.AvgCost / 1e8

		// addCost = qtyExecuted * avgPrice / 1e8
		if avgPrice != 0 && qtyExecuted > math.MaxInt64/avgPrice {
			return 0, 0, 0, 0, fmt.Errorf("computeFill overflow: qtyExecuted(%d) * avgPrice(%d)", qtyExecuted, avgPrice)
		}
		addCost := qtyExecuted * avgPrice / 1e8

		newQty = pos.Qty + qtyExecuted
		newAvgCost = pos.AvgCost
		if newQty > 0 {
			totalCost := costBefore + addCost
			// newAvgCost = totalCost * 1e8 / newQty
			const scale = int64(1e8)
			if totalCost > math.MaxInt64/scale {
				return 0, 0, 0, 0, fmt.Errorf("computeFill overflow: totalCost(%d) * 1e8", totalCost)
			}
			newAvgCost = totalCost * scale / newQty
		}
		return newQty, newAvgCost, pos.RealizedPnl, 0, nil
	}

	// SELL: realise PnL on the sold qty against avg cost.

	// proceeds = qtyExecuted * avgPrice / 1e8
	if avgPrice != 0 && qtyExecuted > math.MaxInt64/avgPrice {
		return 0, 0, 0, 0, fmt.Errorf("computeFill overflow: qtyExecuted(%d) * avgPrice(%d)", qtyExecuted, avgPrice)
	}
	proceeds := qtyExecuted * avgPrice / 1e8

	// cost = qtyExecuted * pos.AvgCost / 1e8
	if pos.AvgCost != 0 && qtyExecuted > math.MaxInt64/pos.AvgCost {
		return 0, 0, 0, 0, fmt.Errorf("computeFill overflow: qtyExecuted(%d) * pos.AvgCost(%d)", qtyExecuted, pos.AvgCost)
	}
	cost := qtyExecuted * pos.AvgCost / 1e8

	realized := proceeds - cost
	newQty = pos.Qty - qtyExecuted
	lossDelta = 0
	if realized < 0 {
		lossDelta = -realized
	}
	return newQty, pos.AvgCost, pos.RealizedPnl + realized, lossDelta, nil
}
