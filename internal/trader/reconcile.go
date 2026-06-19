package trader

import (
	"context"
	"fmt"

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
}

// Reconcile settles leftover pending live orders against exchange history,
// matching on clientOrderId == our order id. Run at trader startup so a crash
// mid-send never leaves a phantom pending row.
// Returns the count of orders that were reconciled (settled).
func Reconcile(ctx context.Context, store reconcileStore, hist OrderHistorySource, symbol string) (int, error) {
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

	n := 0
	for _, o := range pending {
		oi, ok := byClient[o.ID]
		if !ok {
			// Not found in history yet — leave pending.
			continue
		}
		status := order.Accepted
		reason := ""
		if oi.State == "Rejected" || oi.State == "Canceled" || oi.State == "Expired" {
			status = order.Rejected
			reason = oi.State
		}
		if err := store.Settle(ctx, o.ID, status, fmt.Sprintf("%d", oi.OrderID), reason); err != nil {
			return n, fmt.Errorf("settle order %d: %w", o.ID, err)
		}
		n++
	}
	return n, nil
}
