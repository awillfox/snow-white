package trader

import (
	"context"
	"fmt"
	"testing"

	"snow-white/internal/invx"
	"snow-white/internal/order"
)

// fakeReconcileStore is a test double for reconcileStore.
type fakeReconcileStore struct {
	pending []order.Order
	settled []settledCall
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

// fakeHistorySource is a test double for OrderHistorySource.
type fakeHistorySource struct {
	infos []invx.OrderInfo
}

func (f *fakeHistorySource) OrderHistory(_ context.Context, _ string, _ int) ([]invx.OrderInfo, error) {
	return f.infos, nil
}

func TestReconcile_FullyExecuted(t *testing.T) {
	store := &fakeReconcileStore{
		pending: []order.Order{{ID: 5}},
	}
	hist := &fakeHistorySource{
		infos: []invx.OrderInfo{
			{ClientOrderID: 5, OrderID: 99, State: "FullyExecuted"},
		},
	}

	n, err := Reconcile(context.Background(), store, hist, "BTCTHB")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 reconciled, got %d", n)
	}
	if len(store.settled) != 1 {
		t.Fatalf("expected 1 Settle call, got %d", len(store.settled))
	}
	got := store.settled[0]
	if got.id != 5 {
		t.Errorf("expected id=5, got %d", got.id)
	}
	if got.status != order.Accepted {
		t.Errorf("expected status=Accepted, got %q", got.status)
	}
	if got.exchangeRef != "99" {
		t.Errorf("expected exchangeRef=%q, got %q", "99", got.exchangeRef)
	}
	if got.reason != "" {
		t.Errorf("expected empty reason, got %q", got.reason)
	}
}

func TestReconcile_Rejected(t *testing.T) {
	store := &fakeReconcileStore{
		pending: []order.Order{{ID: 5}},
	}
	hist := &fakeHistorySource{
		infos: []invx.OrderInfo{
			{ClientOrderID: 5, OrderID: 99, State: "Rejected"},
		},
	}

	n, err := Reconcile(context.Background(), store, hist, "BTCTHB")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 reconciled, got %d", n)
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

	n, err := Reconcile(context.Background(), store, hist, "BTCTHB")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 reconciled, got %d", n)
	}
}

func TestReconcile_NotFoundInHistory(t *testing.T) {
	store := &fakeReconcileStore{
		pending: []order.Order{{ID: 5}},
	}
	// history has a different ClientOrderID — order 5 is not found
	hist := &fakeHistorySource{
		infos: []invx.OrderInfo{
			{ClientOrderID: 999, OrderID: 42, State: "FullyExecuted"},
		},
	}

	n, err := Reconcile(context.Background(), store, hist, "BTCTHB")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 reconciled (not found), got %d", n)
	}
	if len(store.settled) != 0 {
		t.Errorf("expected no Settle calls, got %d", len(store.settled))
	}
}

func TestReconcile_StoreError(t *testing.T) {
	hist := &fakeHistorySource{}
	// Replace ListPendingLive with an error-returning double
	errStore := &erroringStore{err: fmt.Errorf("db down")}

	_, err := Reconcile(context.Background(), errStore, hist, "BTCTHB")
	if err == nil {
		t.Fatal("expected error from ListPendingLive, got nil")
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
