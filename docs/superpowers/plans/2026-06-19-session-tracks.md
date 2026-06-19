# Session Tracks Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `session_tracks` DB table + `MarkSessionStart`/`MarkSessionEnd` functions that snapshot the account's combined net worth (THB + crypto valued at market) in satang whenever a trading session begins/ends, with CLI commands and auto-call at the trade daemon's start/stop.

**Architecture:** Schema-first (Atlas HCL → migrate → sqlc query → generated Go). A new `internal/session` package provides a `Tracker` with interfaces `Quoter` and `Store` so all logic is testable without DB/network. A store wrapper wraps `*sqlc.Queries`, and a new CLI `session start/end` command wires it together. The trade daemon calls `MarkSessionStart` on entry and `MarkSessionEnd` on exit via defer.

**Tech Stack:** Go 1.21+, Atlas HCL (schema.hcl), sqlc v1.31.1, pgx/v5, cobra, snow-white internal packages (invx, config, scale, sqlc)

## Global Constraints

- All money as int64 satang; float64 only at display (scale.Format). No orders placed.
- `context.Context` is always the first parameter. Errors always wrap with `%w`.
- Never hand-edit files under `sqlc/` — only `sqlc generate` writes those.
- `go test ./...` must pass after every task.
- Don't touch guard/pipeline/order money logic.
- Atlas v1.1.7: multi-line column blocks required in schema.hcl (single-line blocks are rejected).
- sqlc v1.31.1 generates `int32` for `integer` columns and `int64` for `bigint`.

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `schema.hcl` | Modify | Add `session_tracks` table |
| `schema.sql` | Regenerate | Refresh for sqlc (via task generate-sql-schema) |
| `internal/sql/session.sql` | Create | sqlc query: InsertSessionTrack |
| `sqlc/session.sql.go` | Generated | sqlc output — do not hand-edit |
| `sqlc/models.go` | Generated | SessionTrack model added by sqlc |
| `internal/session/session.go` | Create | Tracker, Quoter interface, MarkSessionStart/End, CombinedBalanceSatang |
| `internal/session/store.go` | Create | Store wrapper around *sqlc.Queries |
| `internal/session/session_test.go` | Create | Fakes-based unit tests |
| `internal/cli/session.go` | Create | CLI `session start/end` subcommands |
| `internal/cli/root.go` | Modify | AddCommand(newSessionCmd()) |
| `internal/cli/trade.go` | Modify | Wire MarkSessionStart at startup, MarkSessionEnd on exit |

---

## Task 1: Add `session_tracks` to schema.hcl and run migrations

**Files:**
- Modify: `schema.hcl`

**Interfaces:**
- Produces: `session_tracks` table in dev + prod Postgres DBs; updated `schema.sql`

- [ ] **Step 1: Add the table block to schema.hcl**

Open `/home/nate/Dev/snow-white/schema.hcl` and append after the closing `}` of the `risk_state` table:

```hcl
table "session_tracks" {
  schema = schema.public
  column "id" {
    null = false
    type = bigint
    identity {
      generated = BY_DEFAULT
    }
  }
  column "session_event" {
    null = false
    type = integer
  }
  column "balance" {
    null = false
    type = bigint
  }
  column "event_at" {
    null    = false
    type    = timestamptz
    default = sql("now()")
  }
  primary_key {
    columns = [column.id]
  }
}
```

- [ ] **Step 2: Apply schema to dev DB**

```bash
task migrate-dev
```

Expected output: something like `-- Planned Changes:` followed by `CREATE TABLE "public"."session_tracks"` and `Schema updated successfully.`

- [ ] **Step 3: Apply schema to prod DB**

```bash
task migrate-run
```

Expected output: same `CREATE TABLE` plan + `Schema updated successfully.`

- [ ] **Step 4: Refresh schema.sql**

```bash
task generate-sql-schema
```

Expected: `schema.sql` is rewritten; no error output.

- [ ] **Step 5: Verify table exists in dev DB**

```bash
psql "$PSQL_DEV_URL" -c "\d session_tracks"
```

Expected output shows columns: `id bigint`, `session_event integer`, `balance bigint`, `event_at timestamptz`.

---

## Task 2: Add sqlc query and regenerate

**Files:**
- Create: `internal/sql/session.sql`
- Regenerate: `sqlc/session.sql.go`, `sqlc/models.go`

**Interfaces:**
- Consumes: `session_tracks` table defined in Task 1
- Produces: `sqlc.InsertSessionTrack(ctx, sqlc.InsertSessionTrackParams{SessionEvent int32, Balance int64})` returning `sqlc.SessionTrack`

- [ ] **Step 1: Create internal/sql/session.sql**

Create `/home/nate/Dev/snow-white/internal/sql/session.sql` with:

```sql
-- name: InsertSessionTrack :one
INSERT INTO session_tracks (session_event, balance, event_at)
VALUES ($1, $2, now())
RETURNING *;
```

- [ ] **Step 2: Run sqlcgen**

```bash
task sqlcgen
```

Expected: exits 0; creates `sqlc/session.sql.go`; no errors.

- [ ] **Step 3: Verify generated names**

```bash
grep -n "SessionEvent\|InsertSessionTrack\|SessionTrack" /home/nate/Dev/snow-white/sqlc/session.sql.go /home/nate/Dev/snow-white/sqlc/models.go
```

Expected: `InsertSessionTrackParams` with `SessionEvent int32` and `Balance int64`; `SessionTrack` struct in models.go with `ID int64`, `SessionEvent int32`, `Balance int64`, `EventAt pgtype.Timestamptz`.

- [ ] **Step 4: Build to confirm no compile errors**

```bash
cd /home/nate/Dev/snow-white && go build ./...
```

Expected: exits 0.

---

## Task 3: internal/session package — Tracker + tests

**Files:**
- Create: `internal/session/session.go`
- Create: `internal/session/session_test.go`

**Interfaces:**
- Consumes: `invx.Balance` (Product string, Amount int64 ×1e8), `invx.TickerCandle` (Close int64 = satang per whole coin), `sqlc.InsertSessionTrack` from Task 2
- Produces:
  - `type Quoter interface` with `AccountBalance(ctx) ([]invx.Balance, error)` and `Ticker(ctx, symbol string) ([]invx.TickerCandle, error)`
  - `type Store interface` with `InsertSessionTrack(ctx context.Context, event int32, balanceSatang int64) error`
  - `type Tracker struct` with `NewTracker(q Quoter, store Store) *Tracker`
  - `func (t *Tracker) CombinedBalanceSatang(ctx context.Context) (int64, error)`
  - `func (t *Tracker) MarkSessionStart(ctx context.Context) error`
  - `func (t *Tracker) MarkSessionEnd(ctx context.Context) error`

### Units clarification (critical — read before implementing):
- `invx.Balance.Amount` is ×1e8 for ALL products including THB.
- `invx.TickerCandle.Close` is satang per WHOLE coin (i.e., already has 2 decimal places baked in as satang).
- THB → satang: `amount_x1e8 * 100 / 1e8` (multiply by 100 to convert THB to satang, then divide by 1e8 because Amount is scaled ×1e8).
  - Example: THB amount 2000, stored as 2000*1e8 = 200000000000. Satang = 200000000000*100/1e8 = 200000 satang = 2000 THB × 100 satang/THB. ✓
- crypto → satang: `amount_x1e8 * close / 1e8` (amount in ×1e8 coins × satang-per-coin / 1e8 = satang value).
  - Example: BTC amount 0.001 coin = 100000 (×1e8). Close=200000_00=20000000 satang/coin. Value = 100000 * 20000000 / 1e8 = 200000 satang. ✓
- Skip products with Amount == 0.
- Skip ticker fetch for THB (it's the quote currency, not traded). Identify by `product == "THB"`.
- A product whose Ticker call errors: log with `log.Printf` and skip (don't fail entire snapshot).

- [ ] **Step 1: Write the failing tests**

Create `/home/nate/Dev/snow-white/internal/session/session_test.go`:

```go
package session_test

import (
	"context"
	"errors"
	"testing"

	"snow-white/internal/invx"
	"snow-white/internal/session"
)

// fakeQuoter implements session.Quoter for tests.
type fakeQuoter struct {
	balances []invx.Balance
	tickers  map[string][]invx.TickerCandle
	tickErr  map[string]error
}

func (f *fakeQuoter) AccountBalance(_ context.Context) ([]invx.Balance, error) {
	return f.balances, nil
}

func (f *fakeQuoter) Ticker(_ context.Context, symbol string) ([]invx.TickerCandle, error) {
	if err, ok := f.tickErr[symbol]; ok {
		return nil, err
	}
	return f.tickers[symbol], nil
}

// fakeStore implements session.Store for tests.
type fakeStore struct {
	calls []struct {
		event   int32
		balance int64
	}
}

func (f *fakeStore) InsertSessionTrack(_ context.Context, event int32, balance int64) error {
	f.calls = append(f.calls, struct {
		event   int32
		balance int64
	}{event, balance})
	return nil
}

func TestCombinedBalanceSatang(t *testing.T) {
	// THB 2000 (amount = 2000*1e8 = 200_000_000_000)
	// BTC 0.001 (amount = 100_000 ×1e8-units)
	// BTCTHB close = 20_000_000 satang/coin (= 200,000 THB)
	// Expected:
	//   THB satang = 200_000_000_000 * 100 / 1e8 = 200_000
	//   BTC satang = 100_000 * 20_000_000 / 1e8  = 200_000
	//   Total      = 400_000
	q := &fakeQuoter{
		balances: []invx.Balance{
			{Product: "THB", Amount: 200_000_000_000},
			{Product: "BTC", Amount: 100_000},
		},
		tickers: map[string][]invx.TickerCandle{
			"BTCTHB": {{Close: 20_000_000}},
		},
	}
	store := &fakeStore{}
	tr := session.NewTracker(q, store)

	got, err := tr.CombinedBalanceSatang(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := int64(400_000)
	if got != want {
		t.Errorf("CombinedBalanceSatang = %d, want %d", got, want)
	}
}

func TestMarkSessionStart(t *testing.T) {
	q := &fakeQuoter{
		balances: []invx.Balance{
			{Product: "THB", Amount: 200_000_000_000},
			{Product: "BTC", Amount: 100_000},
		},
		tickers: map[string][]invx.TickerCandle{
			"BTCTHB": {{Close: 20_000_000}},
		},
	}
	store := &fakeStore{}
	tr := session.NewTracker(q, store)

	if err := tr.MarkSessionStart(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.calls) != 1 {
		t.Fatalf("expected 1 store call, got %d", len(store.calls))
	}
	if store.calls[0].event != 1 {
		t.Errorf("event = %d, want 1", store.calls[0].event)
	}
	if store.calls[0].balance != 400_000 {
		t.Errorf("balance = %d, want 400000", store.calls[0].balance)
	}
}

func TestMarkSessionEnd(t *testing.T) {
	q := &fakeQuoter{
		balances: []invx.Balance{
			{Product: "THB", Amount: 200_000_000_000},
		},
		tickers: map[string][]invx.TickerCandle{},
	}
	store := &fakeStore{}
	tr := session.NewTracker(q, store)

	if err := tr.MarkSessionEnd(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.calls) != 1 {
		t.Fatalf("expected 1 store call, got %d", len(store.calls))
	}
	if store.calls[0].event != 2 {
		t.Errorf("event = %d, want 2", store.calls[0].event)
	}
}

func TestCombinedBalanceSatang_SkipsZeroAmount(t *testing.T) {
	q := &fakeQuoter{
		balances: []invx.Balance{
			{Product: "THB", Amount: 200_000_000_000},
			{Product: "BTC", Amount: 0}, // zero — must skip ticker fetch
		},
		tickers: map[string][]invx.TickerCandle{},
	}
	store := &fakeStore{}
	tr := session.NewTracker(q, store)

	got, err := tr.CombinedBalanceSatang(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 200_000 {
		t.Errorf("got %d, want 200000", got)
	}
}

func TestCombinedBalanceSatang_SkipsTickerError(t *testing.T) {
	// ETH ticker errors → skip; THB still contributes
	q := &fakeQuoter{
		balances: []invx.Balance{
			{Product: "THB", Amount: 200_000_000_000},
			{Product: "ETH", Amount: 1_000_000_000},
		},
		tickers: map[string][]invx.TickerCandle{},
		tickErr: map[string]error{
			"THTHB": errors.New("symbol not found"), // note: ETH → THTHB? No: ETH+THB = THTHB is wrong
			"THTHB": errors.New("symbol not found"),
		},
	}
	// Fix: ETHTHB is the symbol
	q.tickErr = map[string]error{
		"THTHB":  errors.New("unused"),
		"ETHTHB": errors.New("ticker error"),
	}
	store := &fakeStore{}
	tr := session.NewTracker(q, store)

	got, err := tr.CombinedBalanceSatang(context.Background())
	if err != nil {
		t.Fatalf("unexpected error even with ticker failure: %v", err)
	}
	// ETH is skipped due to ticker error; only THB contributes
	if got != 200_000 {
		t.Errorf("got %d, want 200000", got)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd /home/nate/Dev/snow-white && go test ./internal/session/...
```

Expected: FAIL with `cannot find package "snow-white/internal/session"` or similar.

- [ ] **Step 3: Implement internal/session/session.go**

Create `/home/nate/Dev/snow-white/internal/session/session.go`:

```go
package session

import (
	"context"
	"fmt"
	"log"

	"snow-white/internal/invx"
)

// Quoter fetches account balances and ticker data from the exchange.
type Quoter interface {
	AccountBalance(ctx context.Context) ([]invx.Balance, error)
	Ticker(ctx context.Context, symbol string) ([]invx.TickerCandle, error)
}

// Store persists session track records to the database.
type Store interface {
	InsertSessionTrack(ctx context.Context, event int32, balanceSatang int64) error
}

// Tracker snapshots combined net worth at session start/end.
type Tracker struct {
	q     Quoter
	store Store
}

// NewTracker creates a Tracker backed by the given Quoter and Store.
func NewTracker(q Quoter, store Store) *Tracker {
	return &Tracker{q: q, store: store}
}

// CombinedBalanceSatang sums the account's THB balance + crypto holdings valued
// at the current market price, all expressed in satang (1 THB = 100 satang).
//
// Unit conventions (from invx package):
//   - Balance.Amount is ×1e8 for ALL products (THB and crypto alike).
//   - TickerCandle.Close is satang per WHOLE coin.
//
// Conversion:
//   - THB: satang = amount_x1e8 * 100 / 1e8
//   - crypto: satang = amount_x1e8 * close_satang / 1e8
//
// Products with Amount == 0 are skipped.
// A product whose ticker call fails is skipped with a log line (non-fatal).
func (t *Tracker) CombinedBalanceSatang(ctx context.Context) (int64, error) {
	balances, err := t.q.AccountBalance(ctx)
	if err != nil {
		return 0, fmt.Errorf("session: fetch account balance: %w", err)
	}

	var total int64
	for _, b := range balances {
		if b.Amount == 0 {
			continue
		}
		if b.Product == "THB" {
			// THB: amount is ×1e8 THB → convert to satang (×100) then undo the ×1e8 scale.
			total += b.Amount * 100 / 1e8
			continue
		}
		// Crypto: fetch THB ticker to get the satang price per whole coin.
		symbol := b.Product + "THB"
		candles, err := t.q.Ticker(ctx, symbol)
		if err != nil {
			log.Printf("session: ticker %s error (skipping): %v", symbol, err)
			continue
		}
		if len(candles) == 0 {
			log.Printf("session: ticker %s returned no candles (skipping)", symbol)
			continue
		}
		closeSatang := candles[0].Close
		// crypto satang = amount_x1e8 × close_satang_per_coin / 1e8
		total += b.Amount * closeSatang / 1e8
	}
	return total, nil
}

const (
	eventStart int32 = 1
	eventEnd   int32 = 2
)

// MarkSessionStart snapshots combined balance and records a session_start event (event=1).
func (t *Tracker) MarkSessionStart(ctx context.Context) error {
	bal, err := t.CombinedBalanceSatang(ctx)
	if err != nil {
		return fmt.Errorf("session start: %w", err)
	}
	if err := t.store.InsertSessionTrack(ctx, eventStart, bal); err != nil {
		return fmt.Errorf("session start: insert track: %w", err)
	}
	return nil
}

// MarkSessionEnd snapshots combined balance and records a session_end event (event=2).
func (t *Tracker) MarkSessionEnd(ctx context.Context) error {
	bal, err := t.CombinedBalanceSatang(ctx)
	if err != nil {
		return fmt.Errorf("session end: %w", err)
	}
	if err := t.store.InsertSessionTrack(ctx, eventEnd, bal); err != nil {
		return fmt.Errorf("session end: insert track: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

```bash
cd /home/nate/Dev/snow-white && go test ./internal/session/...
```

Expected: PASS (all 5 tests pass).

- [ ] **Step 5: Run full suite**

```bash
cd /home/nate/Dev/snow-white && go test ./...
```

Expected: all green.

- [ ] **Step 6: Commit**

```bash
cd /home/nate/Dev/snow-white && git add internal/session/session.go internal/session/session_test.go && git commit -m "feat: session package — Tracker with CombinedBalanceSatang + MarkSessionStart/End"
```

---

## Task 4: Session store wrapper (internal/session/store.go)

**Files:**
- Create: `internal/session/store.go`

**Interfaces:**
- Consumes: `sqlc.New(pool)`, `sqlc.InsertSessionTrackParams{SessionEvent int32, Balance int64}` from Task 2
- Produces: `session.NewStore(pool *pgxpool.Pool) *SessionStore` and `func (s *SessionStore) InsertSessionTrack(ctx context.Context, event int32, balanceSatang int64) error` — implements `session.Store` interface

- [ ] **Step 1: Create internal/session/store.go**

Create `/home/nate/Dev/snow-white/internal/session/store.go`:

```go
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
```

- [ ] **Step 2: Build to confirm no compile errors**

```bash
cd /home/nate/Dev/snow-white && go build ./...
```

Expected: exits 0.

- [ ] **Step 3: Run full suite**

```bash
cd /home/nate/Dev/snow-white && go test ./...
```

Expected: all green.

- [ ] **Step 4: Commit**

```bash
cd /home/nate/Dev/snow-white && git add internal/session/store.go && git commit -m "feat: session.SessionStore — DB wrapper implementing Store interface"
```

---

## Task 5: CLI session commands (internal/cli/session.go) + root wiring

**Files:**
- Create: `internal/cli/session.go`
- Modify: `internal/cli/root.go`

**Interfaces:**
- Consumes: `session.NewTracker`, `session.NewStore`, `invx.New`, `pgxpool.New`, `config.Load`, `scale.Format` — all exactly as used in `internal/cli/order.go` and `internal/cli/trade.go`
- Produces: `newSessionCmd()` returning `*cobra.Command` with subcommands `start` and `end`

- [ ] **Step 1: Create internal/cli/session.go**

Create `/home/nate/Dev/snow-white/internal/cli/session.go`:

```go
package cli

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"snow-white/internal/config"
	"snow-white/internal/invx"
	"snow-white/internal/session"
	"snow-white/pkg/scale"
)

func newSessionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Record a trading session start or end snapshot (combined net worth)",
	}
	cmd.AddCommand(newSessionStartCmd())
	cmd.AddCommand(newSessionEndCmd())
	return cmd
}

func newSessionStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Snapshot combined net worth and record a session_start event",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSessionEvent(cmd, 1)
		},
	}
}

func newSessionEndCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "end",
		Short: "Snapshot combined net worth and record a session_end event",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSessionEvent(cmd, 2)
		},
	}
}

func runSessionEvent(cmd *cobra.Command, event int32) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx := cmd.Context()

	pool, err := pgxpool.New(ctx, cfg.PSQLURL)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()

	client := invx.New(cfg.APIKey, cfg.Secret, cfg.Host, nil)
	store := session.NewStore(pool)
	tracker := session.NewTracker(client, store)

	bal, err := tracker.CombinedBalanceSatang(ctx)
	if err != nil {
		return err
	}
	if err := store.InsertSessionTrack(ctx, event, bal); err != nil {
		return err
	}

	eventName := "start"
	if event == 2 {
		eventName = "end"
	}
	fmt.Printf("session %s recorded: combined_balance=%s THB\n",
		eventName,
		scale.Format(bal, 2),
	)
	return nil
}
```

- [ ] **Step 2: Wire into root.go**

Open `/home/nate/Dev/snow-white/internal/cli/root.go` and add `root.AddCommand(newSessionCmd())` after the existing `AddCommand` calls:

```go
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "snow-white",
		Short:         "InnovestX market-data collection, analysis, and trading CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newCollectCmd())
	root.AddCommand(newAnalyzeCmd())
	root.AddCommand(newBacktestCmd())
	root.AddCommand(newTradeCmd())
	root.AddCommand(newOrderCmd())
	root.AddCommand(newBalanceCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newKillCmd())
	root.AddCommand(newResumeCmd())
	root.AddCommand(newNotifyCmd())
	root.AddCommand(newSessionCmd())
	return root
}
```

- [ ] **Step 3: Build**

```bash
cd /home/nate/Dev/snow-white && go build -o snow-white .
```

Expected: exits 0; `snow-white` binary updated.

- [ ] **Step 4: Smoke-test CLI help**

```bash
cd /home/nate/Dev/snow-white && ./snow-white session --help
```

Expected: shows `start` and `end` subcommands in usage.

- [ ] **Step 5: Run full suite**

```bash
cd /home/nate/Dev/snow-white && go test ./...
```

Expected: all green.

- [ ] **Step 6: Commit**

```bash
cd /home/nate/Dev/snow-white && git add internal/cli/session.go internal/cli/root.go && git commit -m "feat: CLI session start/end commands + wire to root"
```

---

## Task 6: Wire MarkSessionStart/End into the trade daemon

**Files:**
- Modify: `internal/cli/trade.go`

**Interfaces:**
- Consumes: `session.NewTracker`, `session.NewStore`, `*invx.Client` (already built in trade.go), `*pgxpool.Pool` (already built in trade.go)
- Produces: trade daemon calls `MarkSessionStart` after stores are built, `MarkSessionEnd` via defer after `tr.Run` returns

### Important detail:
- `MarkSessionStart`/`MarkSessionEnd` errors are **non-fatal** — log and continue. The trade daemon must not stop trading due to a snapshot failure.
- `MarkSessionEnd` in defer must use `context.Background()` because the run context (`ctx`) is already canceled when `tr.Run` returns during shutdown.

- [ ] **Step 1: Modify internal/cli/trade.go**

In `newTradeCmd`'s `RunE`, after the line `tr.SetNotifier(discord.New(cfg.DiscordWebhookURL))` and before the `mode` print, add session wiring. The final `RunE` body (showing only the additions in context) becomes:

Add import `"log"` and `"snow-white/internal/session"` to the import block at the top of trade.go (it already imports context, so just add session and log).

After `tr.SetNotifier(...)` line and before the `mode := "PAPER"` line, insert:

```go
			// Session tracking — non-fatal: snapshot net worth at daemon start.
			sessStore := session.NewStore(pool)
			sessTracker := session.NewTracker(client, sessStore)
			if err := sessTracker.MarkSessionStart(ctx); err != nil {
				log.Printf("session start snapshot failed (non-fatal): %v", err)
			}
			defer func() {
				// Use Background ctx: the run ctx is already canceled on shutdown.
				if err := sessTracker.MarkSessionEnd(context.Background()); err != nil {
					log.Printf("session end snapshot failed (non-fatal): %v", err)
				}
			}()
```

The resulting block in RunE (showing surrounding context for correct placement):

```go
			tr.SetNotifier(discord.New(cfg.DiscordWebhookURL))

			// Session tracking — non-fatal: snapshot net worth at daemon start.
			sessStore := session.NewStore(pool)
			sessTracker := session.NewTracker(client, sessStore)
			if err := sessTracker.MarkSessionStart(ctx); err != nil {
				log.Printf("session start snapshot failed (non-fatal): %v", err)
			}
			defer func() {
				// Use Background ctx: the run ctx is already canceled on shutdown.
				if err := sessTracker.MarkSessionEnd(context.Background()); err != nil {
					log.Printf("session end snapshot failed (non-fatal): %v", err)
				}
			}()

			mode := "PAPER"
```

- [ ] **Step 2: Build**

```bash
cd /home/nate/Dev/snow-white && go build -o snow-white .
```

Expected: exits 0.

- [ ] **Step 3: Run full suite**

```bash
cd /home/nate/Dev/snow-white && go test ./...
```

Expected: all green.

- [ ] **Step 4: Commit**

```bash
cd /home/nate/Dev/snow-white && git add internal/cli/trade.go && git commit -m "feat: wire session MarkSessionStart/End into trade daemon (non-fatal)"
```

---

## Task 7: End-to-end verification + final commit

**Files:**
- No new files — verification only

**Interfaces:**
- None new

- [ ] **Step 1: Final build + full test suite**

```bash
cd /home/nate/Dev/snow-white && go build -o snow-white . && go test ./...
```

Expected: build exits 0; all tests pass.

- [ ] **Step 2: Run session start against real API**

```bash
cd /home/nate/Dev/snow-white && ./snow-white session start
```

Expected output: `session start recorded: combined_balance=<N>.NN THB`

- [ ] **Step 3: Verify DB row in dev**

```bash
source /home/nate/Dev/snow-white/.env && psql "$PSQL_DEV_URL" -c "SELECT id, session_event, balance, event_at FROM session_tracks ORDER BY id DESC LIMIT 3;"
```

Expected: at least one row with `session_event = 1` and a non-zero balance.

- [ ] **Step 4: Verify DB row in prod**

```bash
source /home/nate/Dev/snow-white/.env && psql "$PSQL_URL" -c "SELECT id, session_event, balance, event_at FROM session_tracks ORDER BY id DESC LIMIT 3;"
```

Expected: at least one row with `session_event = 1` and a non-zero balance.

- [ ] **Step 5: Final squash commit (if desired) or just verify sha**

```bash
cd /home/nate/Dev/snow-white && git log --oneline -6
```

Confirm all feature commits are present.

- [ ] **Step 6: Write report**

Write report to `/home/nate/Dev/snow-white/.superpowers/sdd/session-tracks-report.md` with:
- Schema apply output (from Task 1 Step 2/3)
- Generated field names from sqlc (from Task 2 Step 3)
- CombinedBalance test result (from Task 3 Step 4)
- Real `session start` output (from Task 7 Step 2)
- psql row output (from Task 7 Step 3/4)
- Files changed
- Any concerns (integer overflow risk on large balances, THB detection by product name, skipped-ticker logging)

---

## Self-Review

**Spec coverage check:**

| Spec requirement | Covered by |
|---|---|
| schema.hcl `session_tracks` table | Task 1 |
| Multi-line column block style | Task 1 (style matches existing candles/orders pattern) |
| `migrate-dev && migrate-run` | Task 1 Step 2/3 |
| `generate-sql-schema` | Task 1 Step 4 |
| `internal/sql/session.sql` InsertSessionTrack :one | Task 2 |
| `sqlcgen` | Task 2 Step 2 |
| `SessionEvent int32`, `Balance int64` | Task 2 Step 3 (verified via grep) |
| `Quoter` interface (AccountBalance, Ticker) | Task 3 |
| `Store` interface (InsertSessionTrack) | Task 3 |
| `Tracker` type + `NewTracker` | Task 3 |
| `CombinedBalanceSatang` | Task 3 |
| THB→satang: amount_x1e8*100/1e8 | Task 3 (in code + unit tests) |
| crypto→satang: amount_x1e8*close/1e8 | Task 3 (in code + unit tests) |
| Skip zero-amount products | Task 3 test + impl |
| Skip ticker error (log, don't fail) | Task 3 test + impl |
| `MarkSessionStart` (event=1) | Task 3 |
| `MarkSessionEnd` (event=2) | Task 3 |
| Fakes-based tests, no DB/network | Task 3 |
| Store wrapper (SessionStore) | Task 4 |
| `session start/end` CLI commands | Task 5 |
| Print balance via `scale.Format(bal, 2)` | Task 5 |
| `newSessionCmd()` in root | Task 5 |
| Trade daemon: MarkSessionStart before tr.Run | Task 6 |
| Trade daemon: MarkSessionEnd after tr.Run (defer) | Task 6 |
| Use context.Background() in defer | Task 6 |
| Non-fatal (log + continue on error) | Task 6 |
| `go build -o snow-white . && go test ./...` | Task 7 |
| `./snow-white session start` real API call | Task 7 |
| `psql` row verification | Task 7 |
| Report written to .superpowers/sdd/ | Task 7 |

**Placeholder scan:** None found.

**Type consistency check:**
- `Store.InsertSessionTrack(ctx, event int32, balance int64)` — consistent in interface (session.go), implementation (store.go), and caller (session.go MarkSessionStart/End, cli/session.go).
- `session.NewStore` returns `*SessionStore` which satisfies `session.Store` interface — verified at compile time.
- `sqlc.InsertSessionTrackParams.SessionEvent int32` matches `int32` in Store — consistent.
- `invx.Balance.Amount int64` and `invx.TickerCandle.Close int64` — both used as int64 in arithmetic in CombinedBalanceSatang.
