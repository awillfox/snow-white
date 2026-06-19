package trader

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"snow-white/internal/invx"
	"snow-white/internal/order"
)

type Broker interface {
	SendOrder(ctx context.Context, in invx.SendOrderInput) (int64, error)
}

type OrderStore interface {
	InsertPending(ctx context.Context, in order.InsertPendingInput) (order.Order, error)
	Settle(ctx context.Context, id int64, status order.Status, exchangeRef, reason string) error
	RiskToday(ctx context.Context, day time.Time) (order.RiskState, error)
	ApplyFill(ctx context.Context, in order.FillInput) error
	SetHalted(ctx context.Context, day time.Time, halted bool, reason string) error
	GetPosition(ctx context.Context, symbol string) (order.Position, error)
}

type Pipeline struct {
	broker   Broker
	store    OrderStore
	caps     Caps
	live     bool
	killFile string
	now      func() time.Time
}

func NewPipeline(b Broker, s OrderStore, caps Caps, live bool, killFile string, now func() time.Time) *Pipeline {
	if now == nil {
		now = time.Now
	}
	return &Pipeline{broker: b, store: s, caps: caps, live: live, killFile: killFile, now: now}
}

type Intent struct {
	Symbol   string
	Side     invx.Side
	RefPrice int64 // satang, used as the limit price
	ValueTHB int64 // satang, for Buy
	Quantity int64 // x1e8, for Sell
	Strategy string
}

// orderValue returns the satang exposure of the intent for guard purposes.
func (in Intent) orderValue() int64 {
	if in.Side == invx.Buy {
		return in.ValueTHB
	}
	// Sell exposure ≈ quantity * refPrice / 1e8 (satang).
	return in.Quantity * in.RefPrice / 1e8
}

func sideStr(s invx.Side) string {
	if s == invx.Sell {
		return "SELL"
	}
	return "BUY"
}

func (p *Pipeline) Place(ctx context.Context, in Intent) (order.Order, error) {
	day := p.now().UTC()

	// Kill-file forces halt before anything else.
	if KillFileTripped(p.killFile) {
		_ = p.store.SetHalted(ctx, day, true, "kill file present")
		return order.Order{}, fmt.Errorf("blocked: kill file present")
	}

	state, err := p.store.RiskToday(ctx, day)
	if err != nil {
		return order.Order{}, err
	}

	dec := Check(state, p.caps, in.orderValue())
	if !dec.Allowed {
		if dec.TripHalt {
			_ = p.store.SetHalted(ctx, day, true, dec.Reason)
		}
		return order.Order{}, fmt.Errorf("blocked: %s", dec.Reason)
	}

	mode := order.Paper
	if p.live {
		mode = order.Live
	}

	pending, err := p.store.InsertPending(ctx, order.InsertPendingInput{
		ClientUID:  uuid.NewString(),
		Symbol:     in.Symbol,
		Side:       sideStr(in.Side),
		Type:       "LIMIT",
		LimitPrice: in.RefPrice,
		Quantity:   in.Quantity,
		Mode:       string(mode),
		Strategy:   in.Strategy,
	})
	if err != nil {
		return order.Order{}, err
	}

	if !p.live {
		// Paper: simulate an immediate fill at RefPrice so the position updates
		// and Buy/Sell alternation works (no API call). Update spend + realized loss.
		pos, err := p.store.GetPosition(ctx, in.Symbol)
		if err != nil {
			return order.Order{}, err
		}
		newQty, newAvg, newPnl, spent := simulatePaperFill(pos, in)
		lossDelta := int64(0)
		if d := newPnl - pos.RealizedPnl; d < 0 {
			lossDelta = -d
		}
		if err := p.store.ApplyFill(ctx, order.FillInput{
			OrderID: pending.ID, Symbol: in.Symbol, Day: day,
			NewQty: newQty, NewAvgCost: newAvg, NewRealizedPnl: newPnl,
			SpentDelta: spent, LossDelta: lossDelta, ExchangeRef: "paper",
		}); err != nil {
			return order.Order{}, err
		}
		pending.Status = string(order.Accepted)
		pending.Mode = string(order.Paper)
		return pending, nil
	}

	// Live: send the order using our orders.id as clientOrderId.
	send := invx.SendOrderInput{
		Symbol: in.Symbol, Side: in.Side, Type: invx.Limit,
		LimitPrice: in.RefPrice, ClientOrderID: pending.ID,
	}
	if in.Side == invx.Buy {
		send.Value = in.ValueTHB
	} else {
		send.Quantity = in.Quantity
	}
	orderID, err := p.broker.SendOrder(ctx, send)
	if err != nil {
		_ = p.store.Settle(ctx, pending.ID, order.Rejected, "", err.Error())
		return order.Order{}, fmt.Errorf("send order: %w", err)
	}
	if err := p.store.ApplyFill(ctx, order.FillInput{
		OrderID: pending.ID, Symbol: in.Symbol, Day: day,
		NewQty: 0, NewAvgCost: 0, NewRealizedPnl: 0,
		SpentDelta: in.orderValue(), LossDelta: 0,
		ExchangeRef: fmt.Sprintf("%d", orderID),
	}); err != nil {
		return order.Order{}, err
	}
	pending.Status = string(order.Accepted)
	pending.ExchangeRef = fmt.Sprintf("%d", orderID)
	return pending, nil
}

// simulatePaperFill computes the position after an immediate fill at RefPrice.
// Buy deploys ValueTHB (satang) into units; Sell liquidates Quantity and realizes PnL.
func simulatePaperFill(pos order.Position, in Intent) (qty, avg, pnl, spent int64) {
	if in.Side == invx.Buy {
		units := in.ValueTHB * 1e8 / in.RefPrice
		return pos.Qty + units, in.RefPrice, pos.RealizedPnl, in.ValueTHB
	}
	proceeds := in.Quantity * in.RefPrice / 1e8 // satang
	cost := in.Quantity * pos.AvgCost / 1e8     // satang
	return pos.Qty - in.Quantity, pos.AvgCost, pos.RealizedPnl + (proceeds - cost), 0
}
