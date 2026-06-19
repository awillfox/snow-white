package order

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"snow-white/sqlc"
)

type Store struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, q: sqlc.New(pool)}
}

type InsertPendingInput struct {
	ClientUID  string
	Symbol     string
	Side       string
	Type       string
	LimitPrice int64
	Quantity   int64
	Mode       string
	Strategy   string
}

func (s *Store) InsertPending(ctx context.Context, in InsertPendingInput) (Order, error) {
	var uid pgtype.UUID
	if err := uid.Scan(in.ClientUID); err != nil {
		return Order{}, fmt.Errorf("parse client uid: %w", err)
	}
	row, err := s.q.InsertOrder(ctx, sqlc.InsertOrderParams{
		ClientUid:  uid,
		Symbol:     in.Symbol,
		Side:       in.Side,
		Type:       in.Type,
		LimitPrice: pgtype.Int8{Int64: in.LimitPrice, Valid: in.LimitPrice != 0},
		Quantity:   in.Quantity,
		Mode:       in.Mode,
		Strategy:   pgtype.Text{String: in.Strategy, Valid: in.Strategy != ""},
	})
	if err != nil {
		return Order{}, fmt.Errorf("insert pending order: %w", err)
	}
	return NewFromSQLC(row), nil
}

func (s *Store) Settle(ctx context.Context, id int64, status Status, exchangeRef, reason string) error {
	err := s.q.SettleOrder(ctx, sqlc.SettleOrderParams{
		ID:          id,
		Status:      string(status),
		ExchangeRef: pgtype.Text{String: exchangeRef, Valid: exchangeRef != ""},
		Reason:      pgtype.Text{String: reason, Valid: reason != ""},
	})
	if err != nil {
		return fmt.Errorf("settle order %d: %w", id, err)
	}
	return nil
}

// dayDate converts a time.Time to a pgtype.Date (UTC, date-only).
func dayDate(day time.Time) pgtype.Date {
	return pgtype.Date{
		Time:  time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC),
		Valid: true,
	}
}

// RiskToday returns the risk_state row for day, creating it if absent (INSERT … ON CONFLICT DO UPDATE).
func (s *Store) RiskToday(ctx context.Context, day time.Time) (RiskState, error) {
	row, err := s.q.InsertRiskState(ctx, dayDate(day))
	if err != nil {
		return RiskState{}, fmt.Errorf("get/create risk_state: %w", err)
	}
	return RiskState{
		Day:        row.Day.Time,
		Halted:     row.Halted,
		HaltReason: row.HaltReason.String,
		SpentToday: row.SpentToday,
		LossToday:  row.LossToday,
	}, nil
}

type FillInput struct {
	OrderID        int64
	Symbol         string
	Day            time.Time
	NewQty         int64
	NewAvgCost     int64
	NewRealizedPnl int64
	SpentDelta     int64
	LossDelta      int64
	ExchangeRef    string
}

// ApplyFill settles the order as accepted, upserts the position, and increments the
// day's spent/loss counters — all in one transaction.
func (s *Store) ApplyFill(ctx context.Context, in FillInput) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback on any non-commit path

	q := s.q.WithTx(tx)

	if err := q.SettleOrder(ctx, sqlc.SettleOrderParams{
		ID:          in.OrderID,
		Status:      string(Accepted),
		ExchangeRef: pgtype.Text{String: in.ExchangeRef, Valid: in.ExchangeRef != ""},
		Reason:      pgtype.Text{}, // not set on fill
	}); err != nil {
		return fmt.Errorf("settle in tx: %w", err)
	}

	if err := q.UpsertPosition(ctx, sqlc.UpsertPositionParams{
		Symbol:      in.Symbol,
		Qty:         in.NewQty,
		AvgCost:     in.NewAvgCost,
		RealizedPnl: in.NewRealizedPnl,
	}); err != nil {
		return fmt.Errorf("upsert position in tx: %w", err)
	}

	if err := q.UpdateRiskSpentLoss(ctx, sqlc.UpdateRiskSpentLossParams{
		Day:        dayDate(in.Day),
		SpentToday: in.SpentDelta,
		LossToday:  in.LossDelta,
	}); err != nil {
		return fmt.Errorf("update risk in tx: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit fill: %w", err)
	}
	return nil
}

func (s *Store) SetHalted(ctx context.Context, day time.Time, halted bool, reason string) error {
	// Ensure the row exists before updating.
	if _, err := s.q.InsertRiskState(ctx, dayDate(day)); err != nil {
		return fmt.Errorf("ensure risk row: %w", err)
	}
	if err := s.q.SetRiskHalted(ctx, sqlc.SetRiskHaltedParams{
		Day:        dayDate(day),
		Halted:     halted,
		HaltReason: pgtype.Text{String: reason, Valid: reason != ""},
	}); err != nil {
		return fmt.Errorf("set halted: %w", err)
	}
	return nil
}

func (s *Store) GetPosition(ctx context.Context, symbol string) (Position, error) {
	row, err := s.q.GetPosition(ctx, symbol)
	if err == pgx.ErrNoRows {
		return Position{Symbol: symbol}, nil
	}
	if err != nil {
		return Position{}, fmt.Errorf("get position: %w", err)
	}
	return Position{
		Symbol:      row.Symbol,
		Qty:         row.Qty,
		AvgCost:     row.AvgCost,
		RealizedPnl: row.RealizedPnl,
	}, nil
}

func (s *Store) ListPendingLive(ctx context.Context) ([]Order, error) {
	rows, err := s.q.ListPendingOrders(ctx)
	if err != nil {
		return nil, fmt.Errorf("list pending: %w", err)
	}
	out := make([]Order, 0, len(rows))
	for _, r := range rows {
		out = append(out, NewFromSQLC(r))
	}
	return out, nil
}
