package collector

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"snow-white/internal/candle"
	"snow-white/internal/invx"
)

type fakeFetcher struct{ candles []invx.TickerCandle }

func (f fakeFetcher) Ticker(_ context.Context, symbol string) ([]invx.TickerCandle, error) {
	return f.candles, nil
}

type fakeStore struct{ upserts []candle.Candle }

func (s *fakeStore) Upsert(_ context.Context, c candle.Candle) error {
	s.upserts = append(s.upserts, c)
	return nil
}

func TestPollOnceUpsertsAllReturnedCandles(t *testing.T) {
	ot := time.Date(2026, 6, 14, 8, 47, 0, 0, time.UTC)
	f := fakeFetcher{candles: []invx.TickerCandle{
		{DateTime: ot, Open: 100, High: 110, Low: 90, Close: 105, Volume: 5, InsideBid: 99, InsideAsk: 106, Symbol: "BTCTHB"},
		{DateTime: ot.Add(time.Minute), Open: 105, High: 115, Low: 95, Close: 108, Volume: 6, InsideBid: 104, InsideAsk: 109, Symbol: "BTCTHB"},
	}}
	store := &fakeStore{}
	col := New(f, store, []string{"BTCTHB"}, time.Minute)

	n, err := col.PollOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, n, "must upsert every candle in data[] (backfill)")
	require.Len(t, store.upserts, 2)
	require.Equal(t, "BTCTHB", store.upserts[0].Symbol)
	require.Equal(t, ot, store.upserts[0].OpenTime)
	require.Equal(t, int64(105), store.upserts[0].Close)
	require.Equal(t, "ticker", store.upserts[0].Source)
}
