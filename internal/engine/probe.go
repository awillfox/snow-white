package engine

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Probe opens and pings a PostgreSQL connection. Returns a descriptive error on failure.
func Probe(dsn string) error {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close(ctx)
	if err := conn.Ping(ctx); err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	return nil
}
