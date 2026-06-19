package trader

import (
	"context"
	"fmt"
	"log"
	"time"

	"snow-white/internal/invx"
	"snow-white/internal/order"
)

// OrderHistorySource is the subset of invx.Client used by Reconcile.
type OrderHistorySource interface {
	OrderHistory(ctx context.Context, symbol string, depth int) ([]invx.OrderInfo, error)
}

// reconcileStore is the subset of order.Store used by Reconcile.
type reconcileStore interface {
	ListPendingLive(ctx context.Context) ([]order.Order, error)
	Settle(ctx context.Context, id int64, status order.Status, exchangeRef, reason string) error
	GetPosition(ctx context.Context, symbol string) (order.Position, error)
	ApplyFill(ctx context.Context, in order.FillInput) error
}

// Reconcile settles leftover pending live orders against exchange history,
// matching on clientOrderId == our order id. Run at trader startup (and each
// tick in live mode) so a crash mid-send never leaves a phantom pending row and
// position/PnL/loss_today are always current.
//
// FullyExecuted → ApplyFill with SkipPosition=false (updates position + loss_today).
// Rejected/Canceled/Expired → Settle(Rejected, reason) — status only.
// Working/Unknown → leave pending (partial fills are deferred; documented limitation).
//
// SpentDelta is 0 in the fill — spend was already counted when the order was placed.
//
// Returns the count of orders that were reconciled (settled).
//
// SINGLE-INSTANCE ASSUMPTION: GetPosition is read outside ApplyFill's transaction,
// so Reconcile assumes a SINGLE trader process per symbol. The daemon is
// single-threaded and startup reconcile completes before Run begins, so this is
// safe by construction. Concurrent Reconcile instances for the same symbol would
// race on the position read and could double-apply fills — do NOT run two daemons
// for the same symbol.
func Reconcile(ctx context.Context, store reconcileStore, hist OrderHistorySource, symbol string, now func() time.Time) (int, error) {
	pending, err := store.ListPendingLive(ctx)
	if err != nil {
		return 0, fmt.Errorf("list pending live: %w", err)
	}
	if len(pending) == 0 {
		return 0, nil
	}

	infos, err := hist.OrderHistory(ctx, symbol, 200)
	if err != nil {
		return 0, fmt.Errorf("order history: %w", err)
	}

	// Index history by ClientOrderID for O(1) lookup.
	byClient := make(map[int64]invx.OrderInfo, len(infos))
	for _, oi := range infos {
		byClient[oi.ClientOrderID] = oi
	}

	today := now().UTC()

	n := 0
	for _, o := range pending {
		oi, ok := byClient[o.ID]
		if !ok {
			// Not found in history yet — leave pending.
			continue
		}
		switch oi.State {
		case "FullyExecuted":
			// Read current position, compute the fill deltas, then write atomically.
			pos, err := store.GetPosition(ctx, o.Symbol)
			if err != nil {
				return n, fmt.Errorf("get position for order %d: %w", o.ID, err)
			}
			newQty, newAvg, newPnl, lossDelta, fillErr := computeFill(pos, o.Side, oi.QuantityExecuted, oi.AvgPrice)
			if fillErr != nil {
				// Overflow in position arithmetic — do NOT apply a corrupted fill.
				// Leave the order PENDING so an operator sees it stuck and can investigate.
				log.Printf("reconcile: computeFill overflow for order %d (left pending): %v", o.ID, fillErr)
				continue
			}
			if err := store.ApplyFill(ctx, order.FillInput{
				OrderID:        o.ID,
				Symbol:         o.Symbol,
				Day:            today,
				NewQty:         newQty,
				NewAvgCost:     newAvg,
				NewRealizedPnl: newPnl,
				SpentDelta:     0, // spend already counted at order placement — do NOT double-count
				LossDelta:      lossDelta,
				ExchangeRef:    fmt.Sprintf("%d", oi.OrderID),
				SkipPosition:   false, // this is the whole point: record the real fill
			}); err != nil {
				return n, fmt.Errorf("apply fill for order %d: %w", o.ID, err)
			}
		case "Rejected", "Canceled", "Expired":
			if err := store.Settle(ctx, o.ID, order.Rejected, fmt.Sprintf("%d", oi.OrderID), oi.State); err != nil {
				return n, fmt.Errorf("settle order %d: %w", o.ID, err)
			}
		default:
			// Working (still on book), Unknown, or any unexpected state: leave pending.
			// A resting limit order is not a completed fill. Partial fills stay pending.
			continue
		}
		n++
	}
	return n, nil
}
