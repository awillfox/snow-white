package candle

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"snow-white/sqlc"
)

type Store struct {
	q *sqlc.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{q: sqlc.New(pool)}
}

func (s *Store) Upsert(ctx context.Context, c Candle) error {
	err := s.q.UpsertCandle(ctx, sqlc.UpsertCandleParams{
		Symbol:    c.Symbol,
		OpenTime:  pgtype.Timestamptz{Time: c.OpenTime, Valid: true},
		Open:      c.Open,
		High:      c.High,
		Low:       c.Low,
		Close:     c.Close,
		Volume:    c.Volume,
		InsideBid: c.InsideBid,
		InsideAsk: c.InsideAsk,
		Source:    c.sourceOrDefault(),
	})
	if err != nil {
		return fmt.Errorf("upsert candle %s@%s: %w", c.Symbol, c.OpenTime, err)
	}
	return nil
}

func (c Candle) sourceOrDefault() string {
	if c.Source == "" {
		return "ticker"
	}
	return c.Source
}

func (s *Store) List(ctx context.Context, symbol string, from, to time.Time, limit int32) ([]Candle, error) {
	rows, err := s.q.ListCandles(ctx, sqlc.ListCandlesParams{
		Symbol:   symbol,
		FromTime: pgtype.Timestamptz{Time: from, Valid: true},
		ToTime:   pgtype.Timestamptz{Time: to, Valid: true},
		RowLimit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list candles %s: %w", symbol, err)
	}
	out := make([]Candle, 0, len(rows))
	for _, r := range rows {
		out = append(out, NewFromSQLC(r))
	}
	return out, nil
}
