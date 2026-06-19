package trader

import (
	"context"
	"fmt"
	"testing"
	"time"

	"snow-white/internal/invx"
	"snow-white/internal/order"
)

// reconcileNow returns a deterministic now func for reconcile tests.
func reconcileNow() func() time.Time {
	t := time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC)
	return func() time.Time { return t }
}

// fakeReconcileStore is a test double for reconcileStore.
type fakeReconcileStore struct {
	pending    []order.Order
	position   order.Position
	settled    []settledCall
	fillInputs []order.FillInput
}

type settledCall struct {
	id          int64
	status      order.Status
	exchangeRef string
	reason      string
}

func (f *fakeReconcileStore) ListPendingLive(_ context.Context) ([]order.Order, error) {
	return f.pending, nil
}

func (f *fakeReconcileStore) Settle(_ context.Context, id int64, status order.Status, exchangeRef, reason string) error {
	f.settled = append(f.settled, settledCall{id: id, status: status, exchangeRef: exchangeRef, reason: reason})
	return nil
}

func (f *fakeReconcileStore) GetPosition(_ context.Context, _ string) (order.Position, error) {
	return f.position, nil
}

func (f *fakeReconcileStore) ApplyFill(_ context.Context, in order.FillInput) error {
	f.fillInputs = append(f.fillInputs, in)
	return nil
}

// fakeHistorySource is a test double for OrderHistorySource.
type fakeHistorySource struct {
	infos []invx.OrderInfo
}

func (f *fakeHistorySource) OrderHistory(_ context.Context, _ string, _ int) ([]invx.OrderInfo, error) {
	return f.infos, nil
}

func TestReconcile_FullyExecuted_AppliesFill(t *testing.T) {
	// Pending BUY order for 0.001 BTC.
	// Position is flat before reconcile.
	// Fill: qty=100_000 (×1e8), avgPrice=100_000 satang.
	store := &fakeReconcileStore{
		pending:  []order.Order{{ID: 5, Symbol: "BTCTHB", Side: "BUY"}},
		position: order.Position{Symbol: "BTCTHB", Qty: 0, AvgCost: 0, RealizedPnl: 0},
	}
	hist := &fakeHistorySource{
		infos: []invx.OrderInfo{
			{
				ClientOrderID:    5,
				OrderID:          99,
				State:            "FullyExecuted",
				QuantityExecuted: 100_000,  // 0.001 BTC × 1e8
				AvgPrice:         100_000,  // satang
			},
		},
	}

	n, err := Reconcile(context.Background(), store, hist, "BTCTHB", reconcileNow())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 reconciled, got %d", n)
	}

	// Must NOT call Settle — ApplyFill handles the FullyExecuted case.
	if len(store.settled) != 0 {
		t.Errorf("expected 0 Settle calls, got %d", len(store.settled))
	}

	// Must call ApplyFill exactly once.
	if len(store.fillInputs) != 1 {
		t.Fatalf("expected 1 ApplyFill call, got %d", len(store.fillInputs))
	}
	fi := store.fillInputs[0]

	if fi.OrderID != 5 {
		t.Errorf("ApplyFill.OrderID: want 5, got %d", fi.OrderID)
	}
	if fi.Symbol != "BTCTHB" {
		t.Errorf("ApplyFill.Symbol: want BTCTHB, got %q", fi.Symbol)
	}
	if fi.ExchangeRef != "99" {
		t.Errorf("ApplyFill.ExchangeRef: want %q, got %q", "99", fi.ExchangeRef)
	}
	// computeFill(flat, "BUY", 100_000, 100_000) → newQty=100_000, newAvg=100_000, pnl=0
	if fi.NewQty != 100_000 {
		t.Errorf("ApplyFill.NewQty: want 100_000, got %d", fi.NewQty)
	}
	if fi.NewAvgCost != 100_000 {
		t.Errorf("ApplyFill.NewAvgCost: want 100_000, got %d", fi.NewAvgCost)
	}
	if fi.NewRealizedPnl != 0 {
		t.Errorf("ApplyFill.NewRealizedPnl: want 0, got %d", fi.NewRealizedPnl)
	}

	// CRITICAL: SpentDelta must be 0 — spend was already counted at placement.
	if fi.SpentDelta != 0 {
		t.Errorf("ApplyFill.SpentDelta: want 0 (no double-count), got %d", fi.SpentDelta)
	}
	if fi.LossDelta != 0 {
		t.Errorf("ApplyFill.LossDelta: want 0 (BUY, no loss), got %d", fi.LossDelta)
	}
	if fi.SkipPosition {
		t.Errorf("ApplyFill.SkipPosition: want false, got true — position must be written")
	}
}

func TestReconcile_Rejected(t *testing.T) {
	store := &fakeReconcileStore{
		pending: []order.Order{{ID: 5, Symbol: "BTCTHB", Side: "BUY"}},
	}
	hist := &fakeHistorySource{
		infos: []invx.OrderInfo{
			{ClientOrderID: 5, OrderID: 99, State: "Rejected"},
		},
	}

	n, err := Reconcile(context.Background(), store, hist, "BTCTHB", reconcileNow())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 reconciled, got %d", n)
	}
	// Rejected → Settle only, no ApplyFill.
	if len(store.fillInputs) != 0 {
		t.Errorf("expected 0 ApplyFill calls for Rejected order, got %d", len(store.fillInputs))
	}
	if len(store.settled) != 1 {
		t.Fatalf("expected 1 Settle call, got %d", len(store.settled))
	}
	got := store.settled[0]
	if got.status != order.Rejected {
		t.Errorf("expected status=Rejected, got %q", got.status)
	}
	if got.reason != "Rejected" {
		t.Errorf("expected reason=%q, got %q", "Rejected", got.reason)
	}
}

func TestReconcile_NoPending(t *testing.T) {
	store := &fakeReconcileStore{pending: nil}
	hist := &fakeHistorySource{}

	n, err := Reconcile(context.Background(), store, hist, "BTCTHB", reconcileNow())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 reconciled, got %d", n)
	}
}

func TestReconcile_NotFoundInHistory(t *testing.T) {
	store := &fakeReconcileStore{
		pending: []order.Order{{ID: 5, Symbol: "BTCTHB", Side: "BUY"}},
	}
	// history has a different ClientOrderID — order 5 is not found
	hist := &fakeHistorySource{
		infos: []invx.OrderInfo{
			{ClientOrderID: 999, OrderID: 42, State: "FullyExecuted"},
		},
	}

	n, err := Reconcile(context.Background(), store, hist, "BTCTHB", reconcileNow())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 reconciled (not found), got %d", n)
	}
	if len(store.settled) != 0 {
		t.Errorf("expected no Settle calls, got %d", len(store.settled))
	}
	if len(store.fillInputs) != 0 {
		t.Errorf("expected no ApplyFill calls, got %d", len(store.fillInputs))
	}
}

func TestReconcile_StoreError(t *testing.T) {
	hist := &fakeHistorySource{}
	// Replace ListPendingLive with an error-returning double
	errStore := &erroringStore{err: fmt.Errorf("db down")}

	_, err := Reconcile(context.Background(), errStore, hist, "BTCTHB", reconcileNow())
	if err == nil {
		t.Fatal("expected error from ListPendingLive, got nil")
	}
}

func TestReconcile_Working_LeftPending(t *testing.T) {
	store := &fakeReconcileStore{
		pending: []order.Order{{ID: 7, Symbol: "BTCTHB", Side: "BUY"}},
	}
	hist := &fakeHistorySource{
		infos: []invx.OrderInfo{
			{ClientOrderID: 7, OrderID: 55, State: "Working"},
		},
	}

	n, err := Reconcile(context.Background(), store, hist, "BTCTHB", reconcileNow())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 reconciled (Working order must stay pending), got %d", n)
	}
	for _, s := range store.settled {
		if s.id == 7 {
			t.Errorf("Settle was called for id=7 (Working order) — must NOT settle a resting limit order")
		}
	}
	if len(store.fillInputs) != 0 {
		t.Errorf("expected no ApplyFill for Working order, got %d", len(store.fillInputs))
	}
}

// erroringStore returns an error on ListPendingLive to test error propagation.
type erroringStore struct {
	err error
}

func (e *erroringStore) ListPendingLive(_ context.Context) ([]order.Order, error) {
	return nil, e.err
}

func (e *erroringStore) Settle(_ context.Context, _ int64, _ order.Status, _, _ string) error {
	return nil
}

func (e *erroringStore) GetPosition(_ context.Context, _ string) (order.Position, error) {
	return order.Position{}, nil
}

func (e *erroringStore) ApplyFill(_ context.Context, _ order.FillInput) error {
	return nil
}
