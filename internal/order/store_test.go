package order

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// newTestPool spins up a postgres:16 container and creates the orders,
// positions, and risk_state tables matching schema.hcl.
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

	_, err = pool.Exec(ctx, `
		CREATE TABLE orders (
			id          bigserial PRIMARY KEY,
			client_uid  uuid        NOT NULL UNIQUE,
			symbol      text        NOT NULL,
			side        text        NOT NULL,
			type        text        NOT NULL,
			limit_price bigint,
			quantity    bigint      NOT NULL,
			mode        text        NOT NULL,
			strategy    text,
			status      text        NOT NULL,
			exchange_ref text,
			reason      text,
			created_at  timestamptz NOT NULL DEFAULT now()
		);

		CREATE TABLE positions (
			id           bigserial PRIMARY KEY,
			symbol       text    NOT NULL UNIQUE,
			qty          bigint  NOT NULL DEFAULT 0,
			avg_cost     bigint  NOT NULL DEFAULT 0,
			realized_pnl bigint  NOT NULL DEFAULT 0,
			updated_at   timestamptz NOT NULL DEFAULT now()
		);

		CREATE TABLE risk_state (
			id          bigserial PRIMARY KEY,
			day         date    NOT NULL UNIQUE,
			halted      boolean NOT NULL DEFAULT false,
			halt_reason text,
			spent_today bigint  NOT NULL DEFAULT 0,
			loss_today  bigint  NOT NULL DEFAULT 0,
			updated_at  timestamptz NOT NULL DEFAULT now()
		);
	`)
	require.NoError(t, err)

	return pool
}

func TestApplyFillIsAtomic(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := NewStore(pool)
	day := time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC)

	// Insert a pending order.
	o, err := store.InsertPending(ctx, InsertPendingInput{
		ClientUID:  "019d1bae-e2f1-42d9-b9e8-23d495dbe9f9",
		Symbol:     "BTCTHB",
		Side:       "BUY",
		Type:       "LIMIT",
		LimitPrice: 100000,
		Quantity:   1000000,
		Mode:       "live",
		Strategy:   "macross(5,20)",
	})
	require.NoError(t, err)
	require.Equal(t, "pending", o.Status)

	// Ensure risk_state row exists for today.
	_, err = store.RiskToday(ctx, day)
	require.NoError(t, err)

	// Apply the fill atomically.
	require.NoError(t, store.ApplyFill(ctx, FillInput{
		OrderID:        o.ID,
		Symbol:         "BTCTHB",
		Day:            day,
		NewQty:         1000000,
		NewAvgCost:     100000,
		NewRealizedPnl: 0,
		SpentDelta:     1000,
		LossDelta:      0,
		ExchangeRef:    "55",
	}))

	// Assert position was upserted.
	pos, err := store.GetPosition(ctx, "BTCTHB")
	require.NoError(t, err)
	require.Equal(t, int64(1000000), pos.Qty)

	// Assert risk spent_today was incremented.
	risk, err := store.RiskToday(ctx, day)
	require.NoError(t, err)
	require.Equal(t, int64(1000), risk.SpentToday)
}
