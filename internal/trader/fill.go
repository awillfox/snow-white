package trader

import "snow-white/internal/order"

// computeFill returns the updated position fields and risk delta after a real fill.
//
// side is the ORDER side ("BUY"/"SELL").
// qtyExecuted is in x1e8 units (coin qty scaled by 1e8).
// avgPrice is in satang per 1 coin.
//
// All arithmetic is integer satang / x1e8 — no float.
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
func computeFill(pos order.Position, side string, qtyExecuted, avgPrice int64) (newQty, newAvgCost, newRealizedPnl, lossDelta int64) {
	if side == "BUY" {
		costBefore := pos.Qty * pos.AvgCost / 1e8 // satang
		addCost := qtyExecuted * avgPrice / 1e8    // satang
		newQty = pos.Qty + qtyExecuted
		newAvgCost = pos.AvgCost
		if newQty > 0 {
			newAvgCost = (costBefore + addCost) * 1e8 / newQty // satang/coin
		}
		return newQty, newAvgCost, pos.RealizedPnl, 0
	}

	// SELL: realise PnL on the sold qty against avg cost.
	proceeds := qtyExecuted * avgPrice / 1e8  // satang
	cost := qtyExecuted * pos.AvgCost / 1e8  // satang
	realized := proceeds - cost
	newQty = pos.Qty - qtyExecuted
	lossDelta = 0
	if realized < 0 {
		lossDelta = -realized
	}
	return newQty, pos.AvgCost, pos.RealizedPnl + realized, lossDelta
}
