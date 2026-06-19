package trader

import (
	"testing"

	"snow-white/internal/order"
)

// helper: assert no overflow error on valid computeFill calls
func mustComputeFill(t *testing.T, pos order.Position, side string, qtyExecuted, avgPrice int64) (newQty, newAvgCost, newRealizedPnl, lossDelta int64) {
	t.Helper()
	nq, na, np, ld, err := computeFill(pos, side, qtyExecuted, avgPrice)
	if err != nil {
		t.Fatalf("unexpected computeFill error: %v", err)
	}
	return nq, na, np, ld
}

// TestComputeFill_BuyFromFlat: buy 0.001 BTC @ 100000 satang/coin from a flat position.
// qty 0.001 = 100_000 in x1e8 units; price 100000 satang.
// Expected: newQty=100_000, newAvgCost=100000, newPnl=0, lossDelta=0.
func TestComputeFill_BuyFromFlat(t *testing.T) {
	pos := order.Position{Symbol: "BTCTHB", Qty: 0, AvgCost: 0, RealizedPnl: 0}
	qty := int64(100_000)      // 0.001 BTC in x1e8
	price := int64(100_000_00) // 1,000,000 satang = 10000 THB

	newQty, newAvg, newPnl, lossDelta := mustComputeFill(t, pos, "BUY", qty, price)

	if newQty != qty {
		t.Errorf("newQty: want %d, got %d", qty, newQty)
	}
	if newAvg != price {
		t.Errorf("newAvgCost: want %d, got %d", price, newAvg)
	}
	if newPnl != 0 {
		t.Errorf("newPnl: want 0, got %d", newPnl)
	}
	if lossDelta != 0 {
		t.Errorf("lossDelta: want 0, got %d", lossDelta)
	}
}

// TestComputeFill_BuyAddsToPosition: buy more BTC on top of existing holding — weighted avg cost.
// Existing: 0.001 BTC @ 100000 satang/coin (cost = 100_000 * 100000 / 1e8 = 100 satang).
// Adding:   0.002 BTC @ 150000 satang/coin (cost = 200_000 * 150000 / 1e8 = 300 satang).
// Total qty = 300_000; total cost = 400 satang; newAvgCost = 400 * 1e8 / 300_000 = 133333 satang/coin.
func TestComputeFill_BuyAddsToPosition(t *testing.T) {
	pos := order.Position{
		Symbol:      "BTCTHB",
		Qty:         100_000,    // 0.001 BTC in x1e8
		AvgCost:     100_000_00, // 1,000,000 satang/coin
		RealizedPnl: 0,
	}
	addQty := int64(200_000)   // 0.002 BTC in x1e8
	addPrice := int64(150_000_00) // 1,500,000 satang/coin

	newQty, newAvg, newPnl, lossDelta := mustComputeFill(t, pos, "BUY", addQty, addPrice)

	wantQty := int64(300_000) // 0.003 BTC
	if newQty != wantQty {
		t.Errorf("newQty: want %d, got %d", wantQty, newQty)
	}
	// costBefore = 100_000 * 100_000_00 / 1e8
	//   100_000_00 = 10_000_000 (Go literal); 1e8 = 100_000_000
	//   = 100_000 * 10_000_000 / 100_000_000
	//   = 1_000_000_000_000 / 100_000_000 = 10_000 satang
	// addCost = 200_000 * 150_000_00 / 1e8
	//   150_000_00 = 15_000_000
	//   = 200_000 * 15_000_000 / 100_000_000
	//   = 3_000_000_000_000 / 100_000_000 = 30_000 satang
	// totalCost = 40_000 satang
	// newAvgCost = 40_000 * 1e8 / 300_000
	//            = 4_000_000_000_000 / 300_000 = 13_333_333 satang/coin (integer division)
	wantAvg := int64(13_333_333)
	if newAvg != wantAvg {
		t.Errorf("newAvgCost: want %d, got %d", wantAvg, newAvg)
	}
	if newPnl != 0 {
		t.Errorf("newPnl: want 0, got %d", newPnl)
	}
	if lossDelta != 0 {
		t.Errorf("lossDelta: want 0, got %d", lossDelta)
	}
}

// TestComputeFill_SellAtProfit: sell 0.001 BTC @ 150000 satang when avg cost is 100000 satang.
// proceeds = 100_000 * 150000 / 1e8 = 150 satang
// cost     = 100_000 * 100000 / 1e8 = 100 satang
// realized = +50 satang; lossDelta = 0.
func TestComputeFill_SellAtProfit(t *testing.T) {
	pos := order.Position{
		Symbol:      "BTCTHB",
		Qty:         100_000, // 0.001 BTC in x1e8
		AvgCost:     100_000, // 100000 satang/coin
		RealizedPnl: 0,
	}
	sellQty := int64(100_000) // 0.001 BTC in x1e8
	sellPrice := int64(150_000) // 150000 satang/coin

	newQty, newAvg, newPnl, lossDelta := mustComputeFill(t, pos, "SELL", sellQty, sellPrice)

	if newQty != 0 {
		t.Errorf("newQty: want 0, got %d", newQty)
	}
	if newAvg != pos.AvgCost {
		t.Errorf("newAvgCost: want %d (unchanged), got %d", pos.AvgCost, newAvg)
	}
	// proceeds = 100_000 * 150_000 / 1e8 = 15_000_000_000 / 100_000_000 = 150
	// cost     = 100_000 * 100_000 / 1e8 = 10_000_000_000 / 100_000_000 = 100
	// realized = 50
	wantPnl := int64(50)
	if newPnl != wantPnl {
		t.Errorf("newPnl: want %d, got %d", wantPnl, newPnl)
	}
	if lossDelta != 0 {
		t.Errorf("lossDelta: want 0 (profitable trade), got %d", lossDelta)
	}
}

// TestComputeFill_SellAtLoss: sell 0.001 BTC @ 80000 satang when avg cost is 100000 satang.
// proceeds = 100_000 * 80000 / 1e8 = 80 satang
// cost     = 100_000 * 100000 / 1e8 = 100 satang
// realized = -20 satang; lossDelta = 20.
func TestComputeFill_SellAtLoss(t *testing.T) {
	pos := order.Position{
		Symbol:      "BTCTHB",
		Qty:         100_000, // 0.001 BTC in x1e8
		AvgCost:     100_000, // 100000 satang/coin
		RealizedPnl: 0,
	}
	sellQty := int64(100_000) // 0.001 BTC in x1e8
	sellPrice := int64(80_000) // 80000 satang/coin

	newQty, newAvg, newPnl, lossDelta := mustComputeFill(t, pos, "SELL", sellQty, sellPrice)

	if newQty != 0 {
		t.Errorf("newQty: want 0, got %d", newQty)
	}
	if newAvg != pos.AvgCost {
		t.Errorf("newAvgCost: want %d (unchanged), got %d", pos.AvgCost, newAvg)
	}
	// proceeds = 100_000 * 80_000 / 1e8 = 8_000_000_000 / 100_000_000 = 80
	// cost     = 100_000 * 100_000 / 1e8 = 10_000_000_000 / 100_000_000 = 100
	// realized = -20
	wantPnl := int64(-20)
	if newPnl != wantPnl {
		t.Errorf("newPnl: want %d, got %d", wantPnl, newPnl)
	}
	// lossDelta = magnitude of the loss = 20
	wantLoss := int64(20)
	if lossDelta != wantLoss {
		t.Errorf("lossDelta: want %d, got %d", wantLoss, lossDelta)
	}
}

// TestComputeFill_PartialSell: sell half of a position at a profit.
// Hold 0.002 BTC (qty=200_000 ×1e8) at avgCost=200_000_000_00 satang/coin.
// Sell 0.001 BTC (qty=100_000) at avgPrice=210_000_000_00 satang/coin.
// newQty = 100_000 (0.001 BTC left).
// avgCost unchanged on SELL.
// proceeds = 100_000 * 210_000_000_00 / 1e8 = 21_000_000_00 satang
// cost     = 100_000 * 200_000_000_00 / 1e8 = 20_000_000_00 satang
// realized = +1_000_000_00 satang (positive, so lossDelta=0).
func TestComputeFill_PartialSell(t *testing.T) {
	pos := order.Position{
		Symbol:      "BTCTHB",
		Qty:         200_000,        // 0.002 BTC in ×1e8
		AvgCost:     200_000_000_00, // satang per whole coin
		RealizedPnl: 0,
	}
	sellQty := int64(100_000)        // 0.001 BTC in ×1e8
	sellPrice := int64(210_000_000_00) // satang per whole coin

	newQty, newAvg, newPnl, lossDelta := mustComputeFill(t, pos, "SELL", sellQty, sellPrice)

	if newQty != 100_000 {
		t.Errorf("newQty: want 100_000 (0.001 BTC left), got %d", newQty)
	}
	if newAvg != pos.AvgCost {
		t.Errorf("newAvgCost: want %d (unchanged on SELL), got %d", pos.AvgCost, newAvg)
	}
	// proceeds = 100_000 * 21_000_000_000 / 100_000_000
	//          = 2_100_000_000_000_000 / 100_000_000 = 21_000_000 satang
	// cost     = 100_000 * 20_000_000_000 / 100_000_000
	//          = 2_000_000_000_000_000 / 100_000_000 = 20_000_000 satang
	// realized = 1_000_000 satang
	wantPnl := int64(1_000_000)
	if newPnl != wantPnl {
		t.Errorf("newPnl: want %d, got %d", wantPnl, newPnl)
	}
	if lossDelta != 0 {
		t.Errorf("lossDelta: want 0 (profitable sell), got %d", lossDelta)
	}
}

// TestComputeFill_Overflow_ReturnsError: overflow guard catches astronomically large
// position × price products that would silently corrupt int64 arithmetic.
// Use math.MaxInt64 / 2 as qty and price = 3 to guarantee a * b > MaxInt64.
func TestComputeFill_Overflow_ReturnsError(t *testing.T) {
	// pos.Qty * pos.AvgCost overflows: (MaxInt64/2+1) * 3 > MaxInt64.
	const hugeQty = int64(4_611_686_018_427_387_904)  // MaxInt64/2 + 1 (approximately)
	const hugeAvg = int64(3)

	pos := order.Position{
		Symbol:  "BTCTHB",
		Qty:     hugeQty,
		AvgCost: hugeAvg,
	}
	// Even a tiny executed qty still triggers overflow on pos.Qty * pos.AvgCost.
	_, _, _, _, err := computeFill(pos, "BUY", 1, 1)
	if err == nil {
		t.Fatal("expected overflow error, got nil")
	}
}
