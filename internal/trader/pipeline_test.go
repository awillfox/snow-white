package trader

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"snow-white/internal/invx"
	"snow-white/internal/order"
)

// ─── fakes ───────────────────────────────────────────────────────────────────

type fakeBroker struct {
	orderID   int64
	err       error
	sendCalls []invx.SendOrderInput
}

func (f *fakeBroker) SendOrder(_ context.Context, in invx.SendOrderInput) (int64, error) {
	f.sendCalls = append(f.sendCalls, in)
	return f.orderID, f.err
}

type fakeStore struct {
	riskState order.RiskState
	position  order.Position

	insertedPending []order.InsertPendingInput
	settledCalls    []settleCall
	applyFillCalls  []order.FillInput
	haltedCalls     []haltCall

	nextID int64 // auto-increments per InsertPending
}

type settleCall struct {
	id          int64
	status      order.Status
	exchangeRef string
	reason      string
}

type haltCall struct {
	day    time.Time
	halted bool
	reason string
}

func (f *fakeStore) RiskToday(_ context.Context, _ time.Time) (order.RiskState, error) {
	return f.riskState, nil
}

func (f *fakeStore) InsertPending(_ context.Context, in order.InsertPendingInput) (order.Order, error) {
	f.nextID++
	f.insertedPending = append(f.insertedPending, in)
	return order.Order{
		ID:       f.nextID,
		Symbol:   in.Symbol,
		Side:     in.Side,
		Type:     in.Type,
		Mode:     in.Mode,
		Strategy: in.Strategy,
		Status:   string(order.Pending),
	}, nil
}

func (f *fakeStore) Settle(_ context.Context, id int64, status order.Status, exchangeRef, reason string) error {
	f.settledCalls = append(f.settledCalls, settleCall{id: id, status: status, exchangeRef: exchangeRef, reason: reason})
	return nil
}

func (f *fakeStore) ApplyFill(_ context.Context, in order.FillInput) error {
	f.applyFillCalls = append(f.applyFillCalls, in)
	return nil
}

func (f *fakeStore) SetHalted(_ context.Context, day time.Time, halted bool, reason string) error {
	f.haltedCalls = append(f.haltedCalls, haltCall{day: day, halted: halted, reason: reason})
	return nil
}

func (f *fakeStore) GetPosition(_ context.Context, symbol string) (order.Position, error) {
	if f.position.Symbol == "" {
		return order.Position{Symbol: symbol}, nil
	}
	return f.position, nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func fixedNow() func() time.Time {
	t := time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC)
	return func() time.Time { return t }
}

func defaultCaps() Caps {
	return Caps{
		MaxOrder: 50_000_00, // 50,000 THB in satang
		MaxDaily: 200_000_00,
		MaxLoss:  10_000_00,
	}
}

// ─── tests ───────────────────────────────────────────────────────────────────

func TestPipeline(t *testing.T) {
	ctx := context.Background()

	t.Run("paper mode: no SendOrder; order row inserted with mode=paper status=accepted", func(t *testing.T) {
		broker := &fakeBroker{orderID: 99}
		store := &fakeStore{}
		p := NewPipeline(broker, store, defaultCaps(), false /* paper */, "", fixedNow())

		intent := Intent{
			Symbol:   "BTCTHB",
			Side:     invx.Buy,
			RefPrice: 3_000_000_00, // 3,000,000 THB satang
			ValueTHB: 10_000_00,    // 10,000 THB
			Strategy: "ma-cross",
		}
		o, err := p.Place(ctx, intent)
		require.NoError(t, err)

		// No broker call in paper mode.
		require.Empty(t, broker.sendCalls, "paper mode must not call broker.SendOrder")

		// One InsertPending call with mode=paper.
		require.Len(t, store.insertedPending, 1)
		require.Equal(t, "paper", store.insertedPending[0].Mode)

		// One ApplyFill call (paper simulates fill).
		require.Len(t, store.applyFillCalls, 1)
		require.Equal(t, "paper", store.applyFillCalls[0].ExchangeRef)

		// Returned order reflects paper/accepted.
		require.Equal(t, string(order.Paper), o.Mode)
		require.Equal(t, string(order.Accepted), o.Status)
	})

	t.Run("guard block (over per-order cap): no insert, no SendOrder", func(t *testing.T) {
		broker := &fakeBroker{}
		store := &fakeStore{}
		p := NewPipeline(broker, store, defaultCaps(), false, "", fixedNow())

		// ValueTHB exceeds MaxOrder (50,000 THB).
		intent := Intent{
			Symbol:   "BTCTHB",
			Side:     invx.Buy,
			RefPrice: 3_000_000_00,
			ValueTHB: 60_000_00, // 60,000 THB > 50,000 cap
			Strategy: "ma-cross",
		}
		_, err := p.Place(ctx, intent)
		require.Error(t, err)
		require.True(t, strings.Contains(err.Error(), "cap") || strings.Contains(err.Error(), "blocked"),
			"error should mention cap or blocked, got: %s", err.Error())

		require.Empty(t, store.insertedPending, "guard block must not insert any order")
		require.Empty(t, broker.sendCalls, "guard block must not call broker")
	})

	t.Run("live accepted: InsertPending then SendOrder then ApplyFill with exchangeRef", func(t *testing.T) {
		const exchangeOrderID = int64(12345)
		broker := &fakeBroker{orderID: exchangeOrderID}
		store := &fakeStore{}
		p := NewPipeline(broker, store, defaultCaps(), true /* live */, "", fixedNow())

		intent := Intent{
			Symbol:   "BTCTHB",
			Side:     invx.Buy,
			RefPrice: 3_000_000_00,
			ValueTHB: 10_000_00,
			Strategy: "ma-cross",
		}
		o, err := p.Place(ctx, intent)
		require.NoError(t, err)

		// InsertPending called first with mode=live.
		require.Len(t, store.insertedPending, 1)
		require.Equal(t, "live", store.insertedPending[0].Mode)

		// SendOrder called with clientOrderID matching the inserted order's ID.
		require.Len(t, broker.sendCalls, 1)
		require.Equal(t, store.nextID, broker.sendCalls[0].ClientOrderID,
			"clientOrderID must be the pending order's DB id")

		// ApplyFill called with exchange ref set.
		require.Len(t, store.applyFillCalls, 1)
		require.Equal(t, fmt.Sprintf("%d", exchangeOrderID), store.applyFillCalls[0].ExchangeRef,
			"ApplyFill must carry the exchange-returned order id")

		// No Settle call (live accepted goes through ApplyFill, not Settle).
		require.Empty(t, store.settledCalls)

		// Returned order shows accepted with exchange ref.
		require.Equal(t, string(order.Accepted), o.Status)
		require.Equal(t, fmt.Sprintf("%d", exchangeOrderID), o.ExchangeRef)
	})

	t.Run("live rejected (broker error): pending row settled rejected, no ApplyFill", func(t *testing.T) {
		brokerErr := errors.New("exchange unavailable")
		broker := &fakeBroker{err: brokerErr}
		store := &fakeStore{}
		p := NewPipeline(broker, store, defaultCaps(), true /* live */, "", fixedNow())

		intent := Intent{
			Symbol:   "BTCTHB",
			Side:     invx.Buy,
			RefPrice: 3_000_000_00,
			ValueTHB: 10_000_00,
			Strategy: "ma-cross",
		}
		_, err := p.Place(ctx, intent)
		require.Error(t, err)

		// InsertPending was called (we had a pending row before the broker failed).
		require.Len(t, store.insertedPending, 1)

		// Settle called with status=rejected and the error reason.
		require.Len(t, store.settledCalls, 1)
		require.Equal(t, order.Rejected, store.settledCalls[0].status)
		require.Contains(t, store.settledCalls[0].reason, "exchange unavailable",
			"reject reason must carry the broker error text")

		// ApplyFill must NOT be called on broker failure.
		require.Empty(t, store.applyFillCalls, "ApplyFill must not be called when broker returns error")
	})

	t.Run("loss-stop intent: guard trips halt, SetHalted called, order not placed", func(t *testing.T) {
		broker := &fakeBroker{}
		// Loss today already at the cap threshold.
		store := &fakeStore{
			riskState: order.RiskState{
				LossToday: 10_000_00, // exactly at MaxLoss
			},
		}
		p := NewPipeline(broker, store, defaultCaps(), false, "", fixedNow())

		intent := Intent{
			Symbol:   "BTCTHB",
			Side:     invx.Buy,
			RefPrice: 3_000_000_00,
			ValueTHB: 1_000_00, // small — guard blocks on loss, not size
			Strategy: "ma-cross",
		}
		_, err := p.Place(ctx, intent)
		require.Error(t, err)

		// SetHalted must have been called (TripHalt path).
		require.Len(t, store.haltedCalls, 1)
		require.True(t, store.haltedCalls[0].halted)

		// No order inserted, no broker call.
		require.Empty(t, store.insertedPending)
		require.Empty(t, broker.sendCalls)
	})
}
