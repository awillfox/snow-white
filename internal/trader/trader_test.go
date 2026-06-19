package trader

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"snow-white/internal/candle"
	"snow-white/internal/invx"
	"snow-white/internal/order"
	"snow-white/internal/strategy"
)

// --- fakes ---

type fakeSource struct {
	candles []candle.Candle
}

func (f *fakeSource) List(_ context.Context, _ string, _, _ time.Time, _ int32) ([]candle.Candle, error) {
	return f.candles, nil
}

type stubStrategy struct {
	action strategy.Action
	name   string
}

func (s *stubStrategy) Name() string                          { return s.name }
func (s *stubStrategy) WarmupBars() int                       { return 1 }
func (s *stubStrategy) Evaluate(_ []candle.Candle) strategy.Signal {
	return strategy.Signal{Action: s.action}
}

type fakePlacer struct {
	placed []Intent
}

func (f *fakePlacer) Place(_ context.Context, in Intent) (order.Order, error) {
	f.placed = append(f.placed, in)
	return order.Order{}, nil
}

type fakePosReader struct {
	qty int64
}

func (f *fakePosReader) GetPosition(_ context.Context, _ string) (order.Position, error) {
	return order.Position{Qty: f.qty}, nil
}

// --- helpers ---

func makeCandles(closes ...int64) []candle.Candle {
	cs := make([]candle.Candle, len(closes))
	for i, c := range closes {
		cs[i] = candle.Candle{Close: c, Symbol: "BTC_THB"}
	}
	return cs
}

const (
	testSymbol   = "BTC_THB"
	testBuyValue = int64(1_000_000_00) // 100 THB in satang
)

func newTestTrader(src CandleSource, strat strategy.Strategy, pl placer, pos positionReader) *Trader {
	return NewTrader(src, strat, pl, pos, testSymbol, testBuyValue, time.Minute)
}

// --- tests ---

// TestTick_BuySignal_FlatPosition: Buy signal + pos.Qty==0 → places one Buy intent
// with ValueTHB==buyValue and RefPrice==last close.
func TestTick_BuySignal_FlatPosition(t *testing.T) {
	cs := makeCandles(100_00, 200_00, 300_00) // last close = 300_00 satang
	src := &fakeSource{candles: cs}
	strat := &stubStrategy{action: strategy.Buy, name: "stub"}
	pl := &fakePlacer{}
	pos := &fakePosReader{qty: 0}

	tr := newTestTrader(src, strat, pl, pos)
	require.NoError(t, tr.Tick(context.Background()))

	require.Len(t, pl.placed, 1, "expected exactly one Place call")
	intent := pl.placed[0]
	assert.Equal(t, testSymbol, intent.Symbol)
	assert.Equal(t, invx.Buy, intent.Side)
	assert.Equal(t, int64(300_00), intent.RefPrice, "RefPrice must be last candle close")
	assert.Equal(t, testBuyValue, intent.ValueTHB, "ValueTHB must equal buyValue")
	assert.Equal(t, "stub", intent.Strategy)
}

// TestTick_HoldSignal: Hold → no Place call.
func TestTick_HoldSignal(t *testing.T) {
	cs := makeCandles(100_00, 200_00)
	src := &fakeSource{candles: cs}
	strat := &stubStrategy{action: strategy.Hold}
	pl := &fakePlacer{}
	pos := &fakePosReader{qty: 0}

	tr := newTestTrader(src, strat, pl, pos)
	require.NoError(t, tr.Tick(context.Background()))

	assert.Empty(t, pl.placed, "Hold signal must not call Place")
}

// TestTick_BuySignal_AlreadyLong: Buy signal + pos.Qty>0 → no Place call.
func TestTick_BuySignal_AlreadyLong(t *testing.T) {
	cs := makeCandles(100_00, 200_00)
	src := &fakeSource{candles: cs}
	strat := &stubStrategy{action: strategy.Buy}
	pl := &fakePlacer{}
	pos := &fakePosReader{qty: 1_000_000_00} // already holding

	tr := newTestTrader(src, strat, pl, pos)
	require.NoError(t, tr.Tick(context.Background()))

	assert.Empty(t, pl.placed, "Buy signal when already long must not call Place")
}

// TestTick_SellSignal_Holding: Sell signal + pos.Qty>0 → places one Sell intent
// with Quantity==pos.Qty and RefPrice==last close.
func TestTick_SellSignal_Holding(t *testing.T) {
	const holdingQty = int64(5_000_000_00) // units *1e8
	cs := makeCandles(100_00, 200_00, 400_00)
	src := &fakeSource{candles: cs}
	strat := &stubStrategy{action: strategy.Sell, name: "stub"}
	pl := &fakePlacer{}
	pos := &fakePosReader{qty: holdingQty}

	tr := newTestTrader(src, strat, pl, pos)
	require.NoError(t, tr.Tick(context.Background()))

	require.Len(t, pl.placed, 1, "expected exactly one Place call")
	intent := pl.placed[0]
	assert.Equal(t, invx.Sell, intent.Side)
	assert.Equal(t, int64(400_00), intent.RefPrice)
	assert.Equal(t, holdingQty, intent.Quantity)
}

// TestTick_SellSignal_Flat: Sell signal + pos.Qty==0 → no Place call (nothing to sell).
func TestTick_SellSignal_Flat(t *testing.T) {
	cs := makeCandles(100_00, 200_00)
	src := &fakeSource{candles: cs}
	strat := &stubStrategy{action: strategy.Sell}
	pl := &fakePlacer{}
	pos := &fakePosReader{qty: 0}

	tr := newTestTrader(src, strat, pl, pos)
	require.NoError(t, tr.Tick(context.Background()))

	assert.Empty(t, pl.placed, "Sell signal when flat must not call Place")
}

// TestTick_EmptyCandles: empty candle slice → no Place, no error.
func TestTick_EmptyCandles(t *testing.T) {
	src := &fakeSource{candles: nil}
	strat := &stubStrategy{action: strategy.Buy}
	pl := &fakePlacer{}
	pos := &fakePosReader{qty: 0}

	tr := newTestTrader(src, strat, pl, pos)
	require.NoError(t, tr.Tick(context.Background()))

	assert.Empty(t, pl.placed)
}

// TestRun_ContextCancel: Run cancels promptly when ctx is done.
func TestRun_ContextCancel(t *testing.T) {
	cs := makeCandles(100_00)
	src := &fakeSource{candles: cs}
	strat := &stubStrategy{action: strategy.Hold}
	pl := &fakePlacer{}
	pos := &fakePosReader{qty: 0}

	tr := NewTrader(src, strat, pl, pos, testSymbol, testBuyValue, 10*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := tr.Run(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}
