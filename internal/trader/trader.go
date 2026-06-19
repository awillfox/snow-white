package trader

import (
	"context"
	"fmt"
	"log"
	"time"

	"snow-white/internal/candle"
	"snow-white/internal/invx"
	"snow-white/internal/order"
	"snow-white/internal/strategy"
)

// CandleSource loads recent candles for a symbol. Satisfied by *candle.Store.
type CandleSource interface {
	List(ctx context.Context, symbol string, from, to time.Time, limit int32) ([]candle.Candle, error)
}

// placer places an order intent through the pipeline. Satisfied by *Pipeline.
type placer interface {
	Place(ctx context.Context, in Intent) (order.Order, error)
}

// positionReader reads the current position for a symbol. Satisfied by OrderStore.
type positionReader interface {
	GetPosition(ctx context.Context, symbol string) (order.Position, error)
}

// Notifier sends a notification message. Satisfied by *discord.Client.
// Defined here to keep the trader package free of the discord import.
type Notifier interface {
	Send(ctx context.Context, content string) error
}

// Trader runs a strategy on a schedule and places buy/sell intents via the pipeline.
type Trader struct {
	src       CandleSource
	strat     strategy.Strategy
	pipe      placer
	pos       positionReader
	symbol    string
	buyValue  int64 // satang to deploy per Buy
	interval  time.Duration
	now       func() time.Time
	reconcile func(ctx context.Context) error // nil in paper mode
	notify    Notifier
}

// NewTrader constructs a Trader. buyValueTHB is in satang (THB * 100).
func NewTrader(src CandleSource, strat strategy.Strategy, pipe placer, pos positionReader, symbol string, buyValueTHB int64, interval time.Duration) *Trader {
	return &Trader{
		src:      src,
		strat:    strat,
		pipe:     pipe,
		pos:      pos,
		symbol:   symbol,
		buyValue: buyValueTHB,
		interval: interval,
		now:      time.Now,
	}
}

// SetReconcile registers a hook that Tick calls before evaluating the strategy.
// In live mode, this should call trader.Reconcile so fresh fills are applied
// (position + loss_today updated) before the guard fires.
// Pass nil to clear (paper mode — no hook).
func (t *Trader) SetReconcile(fn func(ctx context.Context) error) {
	t.reconcile = fn
}

// SetNotifier registers a Notifier that Tick pings after every successful order placement.
// Pass nil to clear (no notifications). Notify errors are logged but never returned.
func (t *Trader) SetNotifier(n Notifier) {
	t.notify = n
}

// sendNotify sends a notification after a successful Place.
// It logs and ignores any error so notification failures never affect trading.
func (t *Trader) sendNotify(ctx context.Context, side, symbol, stratName, mode string, orderID int64) {
	if t.notify == nil {
		return
	}
	emoji := "🟢"
	sideLabel := "BUY"
	if side == "SELL" {
		emoji = "🔴"
		sideLabel = "SELL"
	}
	msg := fmt.Sprintf("%s %s %s via %s (order %d, %s)", emoji, sideLabel, symbol, stratName, orderID, mode)
	if err := t.notify.Send(ctx, msg); err != nil {
		log.Printf("trader: notify error (non-fatal): %v", err)
	}
}

// Tick loads recent candles, evaluates the strategy, and places an intent if warranted.
// Buy: only when flat (pos.Qty == 0).
// Sell: only when holding (pos.Qty > 0).
// Hold: no action.
// A blocked/failed Place is logged but not returned as an error.
func (t *Trader) Tick(ctx context.Context) error {
	// Run the reconcile hook first (live only) so position/loss_today reflect
	// any fills that completed since the last tick.
	if t.reconcile != nil {
		if err := t.reconcile(ctx); err != nil {
			log.Printf("trader: reconcile error: %v", err)
			// Non-fatal: continue ticking — stale position is better than no tick.
		}
	}

	to := t.now().UTC()
	from := to.AddDate(0, 0, -1) // last ~1 day of candles covers typical warm-up

	cs, err := t.src.List(ctx, t.symbol, from, to, 100000)
	if err != nil {
		return fmt.Errorf("trader: load candles: %w", err)
	}
	if len(cs) == 0 {
		return nil
	}

	sig := t.strat.Evaluate(cs)
	if sig.Action == strategy.Hold {
		return nil
	}

	last := cs[len(cs)-1].Close

	pos, err := t.pos.GetPosition(ctx, t.symbol)
	if err != nil {
		return fmt.Errorf("trader: get position: %w", err)
	}

	var placed order.Order
	var placeErr error
	switch sig.Action {
	case strategy.Buy:
		if pos.Qty > 0 {
			return nil // already long — skip
		}
		placed, placeErr = t.pipe.Place(ctx, Intent{
			Symbol:   t.symbol,
			Side:     invx.Buy,
			RefPrice: last,
			ValueTHB: t.buyValue,
			Strategy: t.strat.Name(),
		})
		if placeErr == nil {
			t.sendNotify(ctx, "BUY", t.symbol, t.strat.Name(), placed.Mode, placed.ID)
		}
	case strategy.Sell:
		if pos.Qty <= 0 {
			return nil // nothing to sell — skip
		}
		placed, placeErr = t.pipe.Place(ctx, Intent{
			Symbol:   t.symbol,
			Side:     invx.Sell,
			RefPrice: last,
			Quantity: pos.Qty,
			Strategy: t.strat.Name(),
		})
		if placeErr == nil {
			t.sendNotify(ctx, "SELL", t.symbol, t.strat.Name(), placed.Mode, placed.ID)
		}
	}

	if placeErr != nil {
		log.Printf("trader: place blocked/failed: %v", placeErr)
	}
	return nil
}

// Run ticks the strategy every interval until ctx is cancelled.
func (t *Trader) Run(ctx context.Context) error {
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := t.Tick(ctx); err != nil {
				log.Printf("trader: tick error: %v", err)
			}
		}
	}
}

// Ensure *Pipeline satisfies placer (compile-time check).
var _ placer = (*Pipeline)(nil)

// Ensure OrderStore satisfies positionReader (compile-time check).
var _ positionReader = (OrderStore)(nil)
