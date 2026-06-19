package session

import (
	"context"
	"fmt"
	"log"

	"snow-white/internal/invx"
)

// Quoter fetches account balances and ticker data from the exchange.
type Quoter interface {
	AccountBalance(ctx context.Context) ([]invx.Balance, error)
	Ticker(ctx context.Context, symbol string) ([]invx.TickerCandle, error)
}

// Store persists session track records to the database.
type Store interface {
	InsertSessionTrack(ctx context.Context, event int32, balanceSatang int64) error
}

// Tracker snapshots combined net worth at session start/end.
type Tracker struct {
	q     Quoter
	store Store
}

// NewTracker creates a Tracker backed by the given Quoter and Store.
func NewTracker(q Quoter, store Store) *Tracker {
	return &Tracker{q: q, store: store}
}

// CombinedBalanceSatang sums the account's THB balance + crypto holdings valued
// at the current market price, all expressed in satang (1 THB = 100 satang).
//
// Unit conventions (from invx package):
//   - Balance.Amount is ×1e8 for ALL products (THB and crypto alike).
//   - TickerCandle.Close is satang per WHOLE coin.
//
// Conversion:
//   - THB: satang = amount_x1e8 * 100 / 1e8
//   - crypto: satang = amount_x1e8 * close_satang / 1e8
//
// Products with Amount == 0 are skipped.
// A product whose ticker call fails is skipped with a log line (non-fatal).
func (t *Tracker) CombinedBalanceSatang(ctx context.Context) (int64, error) {
	balances, err := t.q.AccountBalance(ctx)
	if err != nil {
		return 0, fmt.Errorf("session: fetch account balance: %w", err)
	}

	var total int64
	for _, b := range balances {
		if b.Amount == 0 {
			continue
		}
		if b.Product == "THB" {
			// THB: amount is ×1e8 THB → convert to satang (×100) then undo the ×1e8 scale.
			total += b.Amount * 100 / 1e8
			continue
		}
		// Crypto: fetch THB ticker to get the satang price per whole coin.
		symbol := b.Product + "THB"
		candles, err := t.q.Ticker(ctx, symbol)
		if err != nil {
			log.Printf("session: ticker %s error (skipping): %v", symbol, err)
			continue
		}
		if len(candles) == 0 {
			log.Printf("session: ticker %s returned no candles (skipping)", symbol)
			continue
		}
		closeSatang := candles[0].Close
		// crypto satang = amount_x1e8 × close_satang_per_coin / 1e8
		total += b.Amount * closeSatang / 1e8
	}
	return total, nil
}

const (
	eventStart int32 = 1
	eventEnd   int32 = 2
)

// MarkSessionStart snapshots combined balance and records a session_start event (event=1).
func (t *Tracker) MarkSessionStart(ctx context.Context) error {
	bal, err := t.CombinedBalanceSatang(ctx)
	if err != nil {
		return fmt.Errorf("session start: %w", err)
	}
	if err := t.store.InsertSessionTrack(ctx, eventStart, bal); err != nil {
		return fmt.Errorf("session start: insert track: %w", err)
	}
	return nil
}

// MarkSessionEnd snapshots combined balance and records a session_end event (event=2).
func (t *Tracker) MarkSessionEnd(ctx context.Context) error {
	bal, err := t.CombinedBalanceSatang(ctx)
	if err != nil {
		return fmt.Errorf("session end: %w", err)
	}
	if err := t.store.InsertSessionTrack(ctx, eventEnd, bal); err != nil {
		return fmt.Errorf("session end: insert track: %w", err)
	}
	return nil
}
