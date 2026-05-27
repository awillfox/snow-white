package engine

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// SnapshotTables returns all BASE TABLE names in the public schema of the given database.
func SnapshotTables(dsn string) (map[string]struct{}, error) {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("snapshot connect: %w", err)
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tables := make(map[string]struct{})
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables[name] = struct{}{}
	}
	return tables, rows.Err()
}

// DiffTables returns table names present in after but not in before.
func DiffTables(before, after map[string]struct{}) []string {
	var news []string
	for t := range after {
		if _, existed := before[t]; !existed {
			news = append(news, t)
		}
	}
	return news
}

// DropNewTables connects to the target, snapshots current tables, drops anything
// not present in the pre-run snapshot. Returns the dropped table names.
func DropNewTables(dsn string, before map[string]struct{}) ([]string, error) {
	after, err := SnapshotTables(dsn)
	if err != nil {
		return nil, fmt.Errorf("post-failure snapshot: %w", err)
	}

	toDelete := DiffTables(before, after)
	if len(toDelete) == 0 {
		return nil, nil
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("cleanup connect: %w", err)
	}
	defer conn.Close(ctx)

	var dropped []string
	for _, table := range toDelete {
		if _, err := conn.Exec(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS %q CASCADE`, table)); err != nil {
			return dropped, fmt.Errorf("drop %q: %w", table, err)
		}
		dropped = append(dropped, table)
	}
	return dropped, nil
}
