package session_test

import (
	"context"
	"errors"
	"testing"

	"snow-white/internal/invx"
	"snow-white/internal/session"
)

// fakeQuoter implements session.Quoter for tests.
type fakeQuoter struct {
	balances []invx.Balance
	tickers  map[string][]invx.TickerCandle
	tickErr  map[string]error
}

func (f *fakeQuoter) AccountBalance(_ context.Context) ([]invx.Balance, error) {
	return f.balances, nil
}

func (f *fakeQuoter) Ticker(_ context.Context, symbol string) ([]invx.TickerCandle, error) {
	if err, ok := f.tickErr[symbol]; ok {
		return nil, err
	}
	return f.tickers[symbol], nil
}

// fakeStore implements session.Store for tests.
type fakeStore struct {
	calls []struct {
		event   int32
		balance int64
	}
}

func (f *fakeStore) InsertSessionTrack(_ context.Context, event int32, balance int64) error {
	f.calls = append(f.calls, struct {
		event   int32
		balance int64
	}{event, balance})
	return nil
}

func TestCombinedBalanceSatang(t *testing.T) {
	// THB 2000 (amount = 2000*1e8 = 200_000_000_000)
	// BTC 0.001 (amount = 100_000 ×1e8-units, i.e. 100_000/1e8 = 0.001 BTC)
	// BTCTHB close = 200_000_000 satang/coin (= 2,000,000 THB/BTC per spec)
	// Expected:
	//   THB satang = 200_000_000_000 * 100 / 1e8 = 200_000
	//   BTC satang = 100_000 * 200_000_000 / 1e8  = 200_000
	//   Total      = 400_000
	q := &fakeQuoter{
		balances: []invx.Balance{
			{Product: "THB", Amount: 200_000_000_000},
			{Product: "BTC", Amount: 100_000},
		},
		tickers: map[string][]invx.TickerCandle{
			"BTCTHB": {{Close: 200_000_000}},
		},
	}
	store := &fakeStore{}
	tr := session.NewTracker(q, store)

	got, err := tr.CombinedBalanceSatang(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := int64(400_000)
	if got != want {
		t.Errorf("CombinedBalanceSatang = %d, want %d", got, want)
	}
}

func TestMarkSessionStart(t *testing.T) {
	q := &fakeQuoter{
		balances: []invx.Balance{
			{Product: "THB", Amount: 200_000_000_000},
			{Product: "BTC", Amount: 100_000},
		},
		tickers: map[string][]invx.TickerCandle{
			"BTCTHB": {{Close: 200_000_000}},
		},
	}
	store := &fakeStore{}
	tr := session.NewTracker(q, store)

	if err := tr.MarkSessionStart(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.calls) != 1 {
		t.Fatalf("expected 1 store call, got %d", len(store.calls))
	}
	if store.calls[0].event != 1 {
		t.Errorf("event = %d, want 1", store.calls[0].event)
	}
	if store.calls[0].balance != 400_000 {
		t.Errorf("balance = %d, want 400000", store.calls[0].balance)
	}
}

func TestMarkSessionEnd(t *testing.T) {
	q := &fakeQuoter{
		balances: []invx.Balance{
			{Product: "THB", Amount: 200_000_000_000},
		},
		tickers: map[string][]invx.TickerCandle{},
	}
	store := &fakeStore{}
	tr := session.NewTracker(q, store)

	if err := tr.MarkSessionEnd(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.calls) != 1 {
		t.Fatalf("expected 1 store call, got %d", len(store.calls))
	}
	if store.calls[0].event != 2 {
		t.Errorf("event = %d, want 2", store.calls[0].event)
	}
}

func TestCombinedBalanceSatang_SkipsZeroAmount(t *testing.T) {
	q := &fakeQuoter{
		balances: []invx.Balance{
			{Product: "THB", Amount: 200_000_000_000},
			{Product: "BTC", Amount: 0}, // zero — must skip ticker fetch
		},
		tickers: map[string][]invx.TickerCandle{},
	}
	store := &fakeStore{}
	tr := session.NewTracker(q, store)

	got, err := tr.CombinedBalanceSatang(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 200_000 {
		t.Errorf("got %d, want 200000", got)
	}
}

func TestCombinedBalanceSatang_SkipsTickerError(t *testing.T) {
	// ETHTHB ticker errors → skip; THB still contributes
	q := &fakeQuoter{
		balances: []invx.Balance{
			{Product: "THB", Amount: 200_000_000_000},
			{Product: "ETH", Amount: 1_000_000_000},
		},
		tickers: map[string][]invx.TickerCandle{},
		tickErr: map[string]error{
			"ETHTHB": errors.New("ticker error"),
		},
	}
	store := &fakeStore{}
	tr := session.NewTracker(q, store)

	got, err := tr.CombinedBalanceSatang(context.Background())
	if err != nil {
		t.Fatalf("unexpected error even with ticker failure: %v", err)
	}
	// ETH is skipped due to ticker error; only THB contributes
	if got != 200_000 {
		t.Errorf("got %d, want 200000", got)
	}
}
