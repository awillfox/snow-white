package session

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"snow-white/sqlc"
)

// SessionStore wraps *sqlc.Queries to implement the Store interface.
type SessionStore struct {
	q *sqlc.Queries
}

// NewStore returns a SessionStore backed by the given pgxpool.Pool.
func NewStore(pool *pgxpool.Pool) *SessionStore {
	return &SessionStore{q: sqlc.New(pool)}
}

// InsertSessionTrack implements Store. It inserts a session_tracks row
// using the generated sqlc query.
func (s *SessionStore) InsertSessionTrack(ctx context.Context, event int32, balanceSatang int64) error {
	_, err := s.q.InsertSessionTrack(ctx, sqlc.InsertSessionTrackParams{
		SessionEvent: event,
		Balance:      balanceSatang,
	})
	if err != nil {
		return fmt.Errorf("insert session track: %w", err)
	}
	return nil
}
