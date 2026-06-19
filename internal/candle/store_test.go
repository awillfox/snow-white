package candle

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	pg, err := postgres.Run(ctx, "postgres:16",
		postgres.WithDatabase("snow_white"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	// Minimal candles DDL for the test (mirrors schema.hcl).
	_, err = pool.Exec(ctx, `
		CREATE TABLE candles (
			id bigserial PRIMARY KEY,
			symbol text NOT NULL,
			open_time timestamptz NOT NULL,
			open bigint NOT NULL, high bigint NOT NULL, low bigint NOT NULL, close bigint NOT NULL,
			volume bigint NOT NULL, inside_bid bigint NOT NULL, inside_ask bigint NOT NULL,
			source text NOT NULL DEFAULT 'ticker',
			ingested_at timestamptz NOT NULL DEFAULT now(),
			UNIQUE (symbol, open_time)
		);`)
	require.NoError(t, err)
	return pool
}

func TestUpsertIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := NewStore(pool)

	ot := time.Date(2026, 6, 14, 8, 47, 0, 0, time.UTC)
	c := Candle{Symbol: "BTCTHB", OpenTime: ot, Open: 100, High: 110, Low: 90, Close: 105, Volume: 0, InsideBid: 99, InsideAsk: 106}

	require.NoError(t, store.Upsert(ctx, c))
	c.Close = 200 // same key, new value
	require.NoError(t, store.Upsert(ctx, c))

	got, err := store.List(ctx, "BTCTHB", ot.Add(-time.Minute), ot.Add(time.Minute), 100)
	require.NoError(t, err)
	require.Len(t, got, 1, "duplicate (symbol, open_time) must upsert to one row")
	require.Equal(t, int64(200), got[0].Close)
}
