# InnovestX Trading CLI — Phase 1 (Data + Analysis) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Collect InnovestX ticker OHLCV candles into Postgres and provide read-only analysis + strategy backtesting — working, testable software with no order placement.

**Architecture:** A cobra CLI (`snow-white`) on viper config. An `invx` client signs every request (HMAC-SHA256) and fetches ticker candles. A `collect` daemon upserts all returned candles into Postgres (idempotent on `(symbol, open_time)`). Pure `indicator` and `strategy` packages compute TA over stored candles; `analyze` prints indicators and `backtest` replays a `Strategy` over history. Money is `bigint` satang; asset volume is scaled int (×1e8); float appears only at display.

**Tech Stack:** Go 1.25, cobra, viper, pgx/v5 + pgxpool, sqlc (pgx/v5), Atlas (schema.hcl), go-money, google/uuid, testify, testcontainers-go.

## Global Constraints

- Module is `snow-white`, Go `1.25.0` (already in `go.mod`). Do not lower the version.
- `main.go` contains exactly one function: `main()`. All wiring lives inside it; helpers go in `pkg/` or `internal/`.
- Never hand-edit anything under `sqlc/`. Regenerate via `task sqlcgen`.
- Money is `bigint` subunits (satang = THB×100) everywhere in DB and domain. Asset quantity/volume is scaled int ×1e8. `float64` is allowed ONLY at display formatting (analyze/backtest output). No float in domain structs, DB, or over the wire.
- Domain structs carry `json` tags only (no `db` tags, no pointers); map from sqlc via `NewFromSQLC`.
- `context.Context` is the first parameter on every function that does I/O.
- Errors wrap with `fmt.Errorf("doing X: %w", err)`.
- Secrets (`INVX_SECRET`, `INVX_APIKEY`) come from viper env/.env only — never logged, never a CLI arg.
- API signing string order is exact and load-bearing: `APIKEY + METHOD(upper) + host(lower) + path + query + ContentType + REQUEST-UID + TIMESTAMP + body`. The body bytes signed must equal the body bytes sent.
- Prefer the `Taskfile.yml` task over raw commands for migrate/sqlcgen/build/test.
- `go test ./...` must pass before each commit.

---

### Task 1: Project foundation — deps, Taskfile, cobra+viper skeleton

**Files:**
- Modify: `go.mod` (deps added by `go get`)
- Create: `Taskfile.yml`
- Create: `main.go`
- Create: `internal/cli/root.go`
- Test: `internal/cli/root_test.go`

**Interfaces:**
- Produces: `cli.NewRootCmd() *cobra.Command` — root command with subcommands attached; used by `main()` and later command tasks.

- [ ] **Step 1: Add dependencies**

Run:
```bash
go get github.com/spf13/cobra@latest
go get github.com/spf13/viper@latest
go get github.com/jackc/pgx/v5@latest
go get github.com/Rhymond/go-money@latest
go get github.com/google/uuid@latest
go get github.com/stretchr/testify@latest
```
Expected: `go.mod`/`go.sum` updated, no errors.

- [ ] **Step 2: Write the failing test**

`internal/cli/root_test.go`:
```go
package cli

import "testing"

func TestNewRootCmd(t *testing.T) {
	cmd := NewRootCmd()
	if cmd.Use != "snow-white" {
		t.Fatalf("Use = %q, want snow-white", cmd.Use)
	}
	if !cmd.HasSubCommands() {
		// no subcommands yet is OK for now; just assert it builds
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestNewRootCmd -v`
Expected: FAIL — `undefined: NewRootCmd`.

- [ ] **Step 4: Implement root command**

`internal/cli/root.go`:
```go
package cli

import "github.com/spf13/cobra"

// NewRootCmd builds the snow-white root command. Subcommands are attached by
// AddCommand in main() wiring or follow-up tasks.
func NewRootCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "snow-white",
		Short:         "InnovestX market-data collection, analysis, and trading CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
}
```

- [ ] **Step 5: Implement main.go**

`main.go`:
```go
package main

import (
	"fmt"
	"os"

	"snow-white/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 6: Create Taskfile**

`Taskfile.yml`:
```yaml
version: "3"

dotenv: [".env"]

tasks:
  build:
    desc: Build the snow-white binary
    cmds:
      - go build -o snow-white .

  test:
    desc: Run all tests
    cmds:
      - go test ./...

  migrate-dev:
    desc: Apply schema.hcl to the dev database
    cmds:
      - atlas schema apply --url "$PSQL_DEV_URL" --to "file://schema.hcl" --dev-url "docker://postgres/16/dev?search_path=public" --auto-approve

  generate-sql-schema:
    desc: Inspect dev DB and regenerate schema.sql for sqlc
    cmds:
      - atlas schema inspect --url "$PSQL_DEV_URL" --format "{{`{{ sql . }}`}}" > schema.sql

  sqlcgen:
    desc: Regenerate sqlc code from internal/sql + schema.sql
    cmds:
      - sqlc generate
```

- [ ] **Step 7: Run tests + build**

Run: `go test ./... && go build -o snow-white . && ./snow-white --help`
Expected: tests PASS; build succeeds; help shows "snow-white".

- [ ] **Step 8: Commit**

```bash
git add go.mod go.sum main.go Taskfile.yml internal/cli/
git commit -m "feat: cobra+viper CLI skeleton and Taskfile"
```

---

### Task 2: Config package (viper typed config)

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces:
  - `type Config struct { APIKey, Secret, Host, PSQLURL string; Symbols []string; CollectInterval time.Duration }`
  - `func Load() (*Config, error)` — reads env + optional `.env` via viper.

- [ ] **Step 1: Write the failing test**

`internal/config/config_test.go`:
```go
package config

import (
	"testing"
	"time"
)

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("INVX_APIKEY", "pub")
	t.Setenv("INVX_SECRET", "sec")
	t.Setenv("INVX_HOST", "api-dev.innovestxonline.com")
	t.Setenv("PSQL_URL", "postgres://localhost/x")
	t.Setenv("INVX_SYMBOLS", "BTCTHB,ETHTHB")
	t.Setenv("INVX_COLLECT_INTERVAL", "30s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIKey != "pub" || cfg.Secret != "sec" {
		t.Fatalf("key/secret not loaded: %+v", cfg)
	}
	if len(cfg.Symbols) != 2 || cfg.Symbols[0] != "BTCTHB" {
		t.Fatalf("symbols = %v", cfg.Symbols)
	}
	if cfg.CollectInterval != 30*time.Second {
		t.Fatalf("interval = %v", cfg.CollectInterval)
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("INVX_APIKEY", "x")
	t.Setenv("INVX_SECRET", "y")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CollectInterval != 60*time.Second {
		t.Fatalf("default interval = %v, want 60s", cfg.CollectInterval)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -v`
Expected: FAIL — `undefined: Load`.

- [ ] **Step 3: Implement config**

`internal/config/config.go`:
```go
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	APIKey          string        `mapstructure:"INVX_APIKEY"`
	Secret          string        `mapstructure:"INVX_SECRET"`
	Host            string        `mapstructure:"INVX_HOST"`
	PSQLURL         string        `mapstructure:"PSQL_URL"`
	Symbols         []string      `mapstructure:"INVX_SYMBOLS"`
	CollectInterval time.Duration `mapstructure:"INVX_COLLECT_INTERVAL"`
}

// Load reads configuration from environment variables, falling back to an
// optional .env file. Environment variables always win.
func Load() (*Config, error) {
	v := viper.New()
	v.SetConfigName(".env")
	v.SetConfigType("env")
	v.AddConfigPath(".")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetDefault("INVX_HOST", "api-dev.innovestxonline.com")
	v.SetDefault("INVX_COLLECT_INTERVAL", "60s")

	_ = v.ReadInConfig() // optional file; env still applies

	// AutomaticEnv does not populate Unmarshal targets unless keys are known.
	for _, k := range []string{
		"INVX_APIKEY", "INVX_SECRET", "INVX_HOST", "PSQL_URL",
		"INVX_SYMBOLS", "INVX_COLLECT_INTERVAL",
	} {
		_ = v.BindEnv(k)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &cfg, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -v`
Expected: PASS. (viper's default Unmarshal decode hooks handle `time.Duration` and comma→`[]string`.)

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: viper typed config loader"
```

---

### Task 3: Scaled-decimal parsing (pkg/scale)

**Files:**
- Create: `pkg/scale/scale.go`
- Test: `pkg/scale/scale_test.go`

**Interfaces:**
- Produces:
  - `func Parse(s string, decimals int) (int64, error)` — `"896789.99000000",2 -> 89678999`.
  - `func Format(v int64, decimals int) string` — `89678999,2 -> "896789.99"`.

These convert the API's decimal strings to scaled ints (satang for prices = 2 decimals; volume = 8 decimals) and back for display.

- [ ] **Step 1: Write the failing test**

`pkg/scale/scale_test.go`:
```go
package scale

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		in       string
		decimals int
		want     int64
		wantErr  bool
	}{
		{"896789.99000000", 2, 89678999, false},
		{"896789.99", 2, 89678999, false},
		{"0.00000000", 8, 0, false},
		{"1.23456789", 8, 123456789, false},
		{"100", 2, 10000, false},
		{"0.005", 2, 0, false},   // truncates beyond scale
		{"abc", 2, 0, true},
		{"", 2, 0, true},
	}
	for _, tc := range tests {
		got, err := Parse(tc.in, tc.decimals)
		if tc.wantErr {
			if err == nil {
				t.Errorf("Parse(%q,%d) expected error", tc.in, tc.decimals)
			}
			continue
		}
		if err != nil {
			t.Errorf("Parse(%q,%d): %v", tc.in, tc.decimals, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Parse(%q,%d) = %d, want %d", tc.in, tc.decimals, got, tc.want)
		}
	}
}

func TestFormat(t *testing.T) {
	if got := Format(89678999, 2); got != "896789.99" {
		t.Errorf("Format = %q, want 896789.99", got)
	}
	if got := Format(123456789, 8); got != "1.23456789" {
		t.Errorf("Format = %q, want 1.23456789", got)
	}
	if got := Format(0, 2); got != "0.00" {
		t.Errorf("Format = %q, want 0.00", got)
	}
}

func TestParseRoundTripNoFloatDrift(t *testing.T) {
	// Large value that would lose precision via float64.
	const s = "999999999.99"
	v, err := Parse(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	if v != 99999999999 {
		t.Fatalf("got %d", v)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/scale/ -v`
Expected: FAIL — `undefined: Parse`.

- [ ] **Step 3: Implement scale**

`pkg/scale/scale.go`:
```go
// Package scale converts fixed-point decimal strings to scaled int64 and back,
// without ever using float64 (no precision drift on money or asset amounts).
package scale

import (
	"fmt"
	"strings"
)

// Parse converts a decimal string to a scaled int64 with `decimals` places.
// Digits beyond `decimals` are truncated. Example: Parse("896789.99", 2) = 89678999.
func Parse(s string, decimals int) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("scale: empty string")
	}
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	intPart, fracPart, _ := strings.Cut(s, ".")

	// Pad/truncate the fractional part to exactly `decimals` digits.
	if len(fracPart) < decimals {
		fracPart += strings.Repeat("0", decimals-len(fracPart))
	} else {
		fracPart = fracPart[:decimals]
	}

	digits := intPart + fracPart
	if digits == "" {
		return 0, fmt.Errorf("scale: no digits in %q", s)
	}
	var v int64
	for _, r := range digits {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("scale: invalid digit in %q", s)
		}
		v = v*10 + int64(r-'0')
	}
	if neg {
		v = -v
	}
	return v, nil
}

// Format renders a scaled int64 as a decimal string with `decimals` places.
func Format(v int64, decimals int) string {
	neg := v < 0
	if neg {
		v = -v
	}
	s := fmt.Sprintf("%d", v)
	if decimals == 0 {
		if neg {
			return "-" + s
		}
		return s
	}
	for len(s) <= decimals {
		s = "0" + s
	}
	split := len(s) - decimals
	out := s[:split] + "." + s[split:]
	if neg {
		out = "-" + out
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/scale/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/scale/
git commit -m "feat: fixed-point decimal<->int64 scale helpers"
```

---

### Task 4: Postgres schema (Atlas HCL) + migration

**Files:**
- Create: `schema.hcl`
- Create: `schema.sql` (generated by `task generate-sql-schema`)

**Interfaces:**
- Produces: tables `candles`, `orders`, `positions`, `risk_state`. Phase 1 uses `candles`; the others are created now so Phase 2 needs no migration. Each table has `id bigserial` PK; natural keys are `UNIQUE`.

**Prereq:** a reachable dev Postgres and `PSQL_DEV_URL` set in `.env`. Atlas CLI installed (`curl -sSf https://atlasgo.sh | sh`).

- [ ] **Step 1: Write schema.hcl**

`schema.hcl`:
```hcl
schema "public" {}

table "candles" {
  schema = schema.public
  column "id"          { null = false, type = bigint, identity { generated = BY_DEFAULT } }
  column "symbol"      { null = false, type = text }
  column "open_time"   { null = false, type = timestamptz }
  column "open"        { null = false, type = bigint }
  column "high"        { null = false, type = bigint }
  column "low"         { null = false, type = bigint }
  column "close"       { null = false, type = bigint }
  column "volume"      { null = false, type = bigint }
  column "inside_bid"  { null = false, type = bigint }
  column "inside_ask"  { null = false, type = bigint }
  column "source"      { null = false, type = text, default = "ticker" }
  column "ingested_at" { null = false, type = timestamptz, default = sql("now()") }
  primary_key { columns = [column.id] }
  index "candles_symbol_open_time_key" {
    unique  = true
    columns = [column.symbol, column.open_time]
  }
}

table "orders" {
  schema = schema.public
  column "id"           { null = false, type = bigint, identity { generated = BY_DEFAULT } }
  column "client_uid"   { null = false, type = uuid }
  column "symbol"       { null = false, type = text }
  column "side"         { null = false, type = text }
  column "type"         { null = false, type = text }
  column "limit_price"  { null = true,  type = bigint }
  column "quantity"     { null = false, type = bigint }
  column "mode"         { null = false, type = text }
  column "strategy"     { null = true,  type = text }
  column "status"       { null = false, type = text }
  column "exchange_ref" { null = true,  type = text }
  column "reason"       { null = true,  type = text }
  column "created_at"   { null = false, type = timestamptz, default = sql("now()") }
  primary_key { columns = [column.id] }
  index "orders_client_uid_key" { unique = true, columns = [column.client_uid] }
}

table "positions" {
  schema = schema.public
  column "id"           { null = false, type = bigint, identity { generated = BY_DEFAULT } }
  column "symbol"       { null = false, type = text }
  column "qty"          { null = false, type = bigint, default = 0 }
  column "avg_cost"     { null = false, type = bigint, default = 0 }
  column "realized_pnl" { null = false, type = bigint, default = 0 }
  column "updated_at"   { null = false, type = timestamptz, default = sql("now()") }
  primary_key { columns = [column.id] }
  index "positions_symbol_key" { unique = true, columns = [column.symbol] }
}

table "risk_state" {
  schema = schema.public
  column "id"          { null = false, type = bigint, identity { generated = BY_DEFAULT } }
  column "day"         { null = false, type = date }
  column "halted"      { null = false, type = boolean, default = false }
  column "halt_reason" { null = true,  type = text }
  column "spent_today" { null = false, type = bigint, default = 0 }
  column "loss_today"  { null = false, type = bigint, default = 0 }
  column "updated_at"  { null = false, type = timestamptz, default = sql("now()") }
  primary_key { columns = [column.id] }
  index "risk_state_day_key" { unique = true, columns = [column.day] }
}
```

- [ ] **Step 2: Apply to dev DB**

Run: `task migrate-dev`
Expected: Atlas prints a plan and applies it; exit 0. Verify:
```bash
psql "$PSQL_DEV_URL" -c "\d candles"
```
Expected: table `candles` with the columns above and a unique index on `(symbol, open_time)`.

- [ ] **Step 3: Generate schema.sql for sqlc**

Run: `task generate-sql-schema`
Expected: `schema.sql` created containing `CREATE TABLE` statements for all four tables.

- [ ] **Step 4: Commit**

```bash
git add schema.hcl schema.sql
git commit -m "feat: postgres schema (candles, orders, positions, risk_state)"
```

---

### Task 5: sqlc setup + candle queries

**Files:**
- Create: `sqlc.yaml`
- Create: `internal/sql/candles.sql`
- Create: `sqlc/` (GENERATED by `task sqlcgen`)

**Interfaces:**
- Produces (generated): `sqlc.Queries` with `UpsertCandle(ctx, UpsertCandleParams) error`, `ListCandles(ctx, ListCandlesParams) ([]Candle, error)`, `ListSymbols(ctx) ([]string, error)`. Exact param/row field names come from sqlc; Task 6 maps them.

**Prereq:** sqlc installed (`go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`).

- [ ] **Step 1: Write sqlc.yaml**

`sqlc.yaml`:
```yaml
version: "2"
sql:
  - engine: "postgresql"
    schema: "schema.sql"
    queries: "internal/sql"
    gen:
      go:
        package: "sqlc"
        out: "sqlc"
        sql_package: "pgx/v5"
        emit_json_tags: true
```

- [ ] **Step 2: Write candle queries**

`internal/sql/candles.sql`:
```sql
-- name: UpsertCandle :exec
INSERT INTO candles (
    symbol, open_time, open, high, low, close, volume, inside_bid, inside_ask, source
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
ON CONFLICT (symbol, open_time) DO UPDATE SET
    open       = EXCLUDED.open,
    high       = EXCLUDED.high,
    low        = EXCLUDED.low,
    close      = EXCLUDED.close,
    volume     = EXCLUDED.volume,
    inside_bid = EXCLUDED.inside_bid,
    inside_ask = EXCLUDED.inside_ask,
    source     = EXCLUDED.source;

-- name: ListCandles :many
SELECT *
FROM candles
WHERE symbol = $1
  AND open_time >= sqlc.arg(from_time)
  AND open_time <= sqlc.arg(to_time)
ORDER BY open_time ASC
LIMIT sqlc.arg('row_limit');

-- name: ListSymbols :many
SELECT DISTINCT symbol FROM candles ORDER BY symbol ASC;
```

- [ ] **Step 3: Generate**

Run: `task sqlcgen`
Expected: `sqlc/` directory created with `db.go`, `models.go`, `candles.sql.go`; no errors.

- [ ] **Step 4: Verify it compiles**

Run: `go build ./sqlc/...`
Expected: builds clean.

- [ ] **Step 5: Commit**

```bash
git add sqlc.yaml internal/sql/candles.sql sqlc/
git commit -m "feat: sqlc candle queries (upsert, list, symbols)"
```

---

### Task 6: Candle domain + store (integration-tested upsert)

**Files:**
- Create: `internal/candle/candle.go`
- Create: `internal/candle/store.go`
- Test: `internal/candle/store_test.go`

**Interfaces:**
- Consumes: generated `sqlc.Queries` (Task 5).
- Produces:
  - `type Candle struct { ID int64; Symbol string; OpenTime time.Time; Open, High, Low, Close, Volume, InsideBid, InsideAsk int64; Source string; IngestedAt time.Time }`
  - `func NewFromSQLC(c sqlc.Candle) Candle`
  - `type Store struct{ ... }`; `func NewStore(pool *pgxpool.Pool) *Store`
  - `func (s *Store) Upsert(ctx context.Context, c Candle) error`
  - `func (s *Store) List(ctx context.Context, symbol string, from, to time.Time, limit int32) ([]Candle, error)`

**Prereq:** Docker available for testcontainers. `go get github.com/testcontainers/testcontainers-go@latest github.com/testcontainers/testcontainers-go/modules/postgres@latest`.

- [ ] **Step 1: Write domain + mapper**

`internal/candle/candle.go`:
```go
package candle

import (
	"time"

	"snow-white/sqlc"
)

// Candle is one OHLCV bar. Prices are satang (THB*100); Volume is asset units *1e8.
type Candle struct {
	ID         int64     `json:"id"`
	Symbol     string    `json:"symbol"`
	OpenTime   time.Time `json:"open_time"`
	Open       int64     `json:"open"`
	High       int64     `json:"high"`
	Low        int64     `json:"low"`
	Close      int64     `json:"close"`
	Volume     int64     `json:"volume"`
	InsideBid  int64     `json:"inside_bid"`
	InsideAsk  int64     `json:"inside_ask"`
	Source     string    `json:"source"`
	IngestedAt time.Time `json:"ingested_at"`
}

func NewFromSQLC(c sqlc.Candle) Candle {
	return Candle{
		ID:         c.ID,
		Symbol:     c.Symbol,
		OpenTime:   c.OpenTime.Time,
		Open:       c.Open,
		High:       c.High,
		Low:        c.Low,
		Close:      c.Close,
		Volume:     c.Volume,
		InsideBid:  c.InsideBid,
		InsideAsk:  c.InsideAsk,
		Source:     c.Source,
		IngestedAt: c.IngestedAt.Time,
	}
}
```
> NOTE: if `task sqlcgen` emitted `pgtype.Timestamptz` for `open_time`/`ingested_at`, `.Time` unwraps it (shown). If it emitted plain `time.Time`, drop the `.Time`. Adjust to the generated types — verify by reading `sqlc/models.go`.

- [ ] **Step 2: Write store**

`internal/candle/store.go`:
```go
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
```
> NOTE: field names (`UpsertCandleParams.OpenTime`, `ListCandlesParams.RowLimit`, etc.) and pgtype usage must match the generated code. Read `sqlc/candles.sql.go` and align names/types exactly.

- [ ] **Step 3: Write integration test (upsert idempotency)**

`internal/candle/store_test.go`:
```go
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
```

- [ ] **Step 4: Run the test**

Run: `go test ./internal/candle/ -v`
Expected: PASS (pulls postgres:16 image on first run). Fix any generated-name mismatches surfaced here.

- [ ] **Step 5: Commit**

```bash
git add internal/candle/ go.mod go.sum
git commit -m "feat: candle domain + store with idempotent upsert"
```

---

### Task 7: invx client — signing + ticker

**Files:**
- Create: `internal/invx/sign.go`
- Create: `internal/invx/client.go`
- Test: `internal/invx/sign_test.go`
- Test: `internal/invx/client_test.go`

**Interfaces:**
- Consumes: `pkg/scale` (Task 3).
- Produces:
  - `type Client struct{ ... }`
  - `func New(apikey, secret, host string, hc *http.Client) *Client`
  - `type TickerCandle struct { DateTime time.Time; Open, High, Low, Close, Volume, InsideBid, InsideAsk int64; Symbol string }`
  - `func (c *Client) Ticker(ctx context.Context, symbol string) ([]TickerCandle, error)`
  - internal: `func buildStringToSign(apikey, method, host, path, query, contentType, uid, ts, body string) string`, `func sign(secret, s string) string`.

- [ ] **Step 1: Write the failing signing test (golden vector)**

`internal/invx/sign_test.go`:
```go
package invx

import "testing"

func TestBuildStringToSign(t *testing.T) {
	got := buildStringToSign(
		"03e8b94ce5194678bb9b8938274ba437bc9fb653bf9b4e199ebbc8d51566b9cc",
		"POST",
		"api-dev.innovestxonline.com",
		"/api/v1/digital-asset/orderbook/lvl2",
		"",
		"application/json",
		"019d1bae-e2f1-42d9-b9e8-23d495dbe9f9",
		"1567755304968",
		`{"symbol":"ETHTHB"}`,
	)
	want := "03e8b94ce5194678bb9b8938274ba437bc9fb653bf9b4e199ebbc8d51566b9cc" +
		"POSTapi-dev.innovestxonline.com/api/v1/digital-asset/orderbook/lvl2" +
		"application/json019d1bae-e2f1-42d9-b9e8-23d495dbe9f91567755304968" +
		`{"symbol":"ETHTHB"}`
	if got != want {
		t.Fatalf("string_to_sign mismatch:\n got=%q\nwant=%q", got, want)
	}
}

func TestSignGolden(t *testing.T) {
	s := buildStringToSign(
		"03e8b94ce5194678bb9b8938274ba437bc9fb653bf9b4e199ebbc8d51566b9cc",
		"POST", "api-dev.innovestxonline.com",
		"/api/v1/digital-asset/orderbook/lvl2", "", "application/json",
		"019d1bae-e2f1-42d9-b9e8-23d495dbe9f9", "1567755304968",
		`{"symbol":"ETHTHB"}`,
	)
	got := sign("b76487089ff240988542a61a9bbaacb5", s)
	const want = "bd6ac085eecdcb21bc2f247c58f0258d7246cab8fdc48198029a94529c3687a8"
	if got != want {
		t.Fatalf("sign = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/invx/ -run TestSign -v`
Expected: FAIL — `undefined: buildStringToSign`.

- [ ] **Step 3: Implement signing**

`internal/invx/sign.go`:
```go
package invx

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

const contentType = "application/json"

// buildStringToSign assembles the exact canonical string InnovestX signs.
// Order is load-bearing; method is uppercased and host lowercased by the caller.
func buildStringToSign(apikey, method, host, path, query, ct, uid, ts, body string) string {
	return apikey + method + host + path + query + ct + uid + ts + body
}

// sign returns the lowercase hex HMAC-SHA256 of s keyed by secret.
func sign(secret, s string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(s))
	return hex.EncodeToString(mac.Sum(nil))
}
```

- [ ] **Step 4: Run signing test to verify it passes**

Run: `go test ./internal/invx/ -run TestSign -v && go test ./internal/invx/ -run TestBuildStringToSign -v`
Expected: PASS — both, including the golden hex.

- [ ] **Step 5: Write the client + transport test**

`internal/invx/client_test.go`:
```go
package invx

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTickerParsesCandles(t *testing.T) {
	var gotBody string
	var gotHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		gotHeaders = r.Header.Clone()
		_, _ = w.Write([]byte(`{
			"code":"0000","message":"SUCCESS",
			"data":[{"dateTime":"2023-06-14T08:47:00.000Z","high":"896789.99000000",
			"low":"800000.00000000","open":"850000.00000000","close":"896789.99000000",
			"volume":"1.50000000","insideBidPrice":"892910.64000000",
			"insideAskPrice":"896789.99000000","symbol":"BTCTHB"}]}`))
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	c := New("pub", "sec", host, srv.Client())
	c.baseURL = srv.URL // test override; real client builds https://host

	candles, err := c.Ticker(context.Background(), "BTCTHB")
	require.NoError(t, err)
	require.Len(t, candles, 1)
	require.Equal(t, int64(89678999), candles[0].Close) // 896789.99 -> satang
	require.Equal(t, int64(150000000), candles[0].Volume) // 1.5 -> *1e8

	require.Equal(t, "pub", gotHeaders.Get("X-INVX-APIKEY"))
	require.NotEmpty(t, gotHeaders.Get("X-INVX-SIGNATURE"))
	require.NotEmpty(t, gotHeaders.Get("X-INVX-REQUEST-UID"))
	require.NotEmpty(t, gotHeaders.Get("X-INVX-TIMESTAMP"))

	// Body sent must be the exact bytes signed (compact JSON, no trailing space).
	var probe map[string]string
	require.NoError(t, json.Unmarshal([]byte(gotBody), &probe))
	require.Equal(t, "BTCTHB", probe["symbol"])
}
```

- [ ] **Step 6: Implement the client**

`internal/invx/client.go`:
```go
package invx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"snow-white/pkg/scale"
)

const basePath = "/api/v1/digital-asset"

type Client struct {
	apikey  string
	secret  string
	host    string // lowercase hostname, e.g. api-dev.innovestxonline.com
	baseURL string // https://<host>
	hc      *http.Client
	now     func() time.Time
}

func New(apikey, secret, host string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	host = strings.ToLower(host)
	return &Client{
		apikey:  apikey,
		secret:  secret,
		host:    host,
		baseURL: "https://" + host,
		hc:      hc,
		now:     time.Now,
	}
}

type apiResponse struct {
	Code    string        `json:"code"`
	Message string        `json:"message"`
	Data    []tickerRawDA `json:"data"`
}

type tickerRawDA struct {
	DateTime       string `json:"dateTime"`
	High           string `json:"high"`
	Low            string `json:"low"`
	Open           string `json:"open"`
	Close          string `json:"close"`
	Volume         string `json:"volume"`
	InsideBidPrice string `json:"insideBidPrice"`
	InsideAskPrice string `json:"insideAskPrice"`
	Symbol         string `json:"symbol"`
}

type TickerCandle struct {
	DateTime  time.Time
	Open      int64
	High      int64
	Low       int64
	Close     int64
	Volume    int64
	InsideBid int64
	InsideAsk int64
	Symbol    string
}

func (c *Client) Ticker(ctx context.Context, symbol string) ([]TickerCandle, error) {
	body, err := json.Marshal(map[string]string{"symbol": symbol})
	if err != nil {
		return nil, fmt.Errorf("marshal ticker body: %w", err)
	}
	raw, err := c.post(ctx, "/ticker/subscribe", body)
	if err != nil {
		return nil, err
	}
	var resp apiResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode ticker resp: %w", err)
	}
	if resp.Code != "0000" {
		return nil, fmt.Errorf("ticker %s: api error %s: %s", symbol, resp.Code, resp.Message)
	}
	out := make([]TickerCandle, 0, len(resp.Data))
	for _, d := range resp.Data {
		tc, err := d.toCandle()
		if err != nil {
			return nil, fmt.Errorf("parse candle for %s: %w", symbol, err)
		}
		out = append(out, tc)
	}
	return out, nil
}

func (d tickerRawDA) toCandle() (TickerCandle, error) {
	dt, err := time.Parse(time.RFC3339, d.DateTime)
	if err != nil {
		return TickerCandle{}, fmt.Errorf("dateTime %q: %w", d.DateTime, err)
	}
	p := func(s string) (int64, error) { return scale.Parse(s, 2) }   // satang
	v := func(s string) (int64, error) { return scale.Parse(s, 8) }   // asset *1e8
	open, err := p(d.Open)
	if err != nil { return TickerCandle{}, err }
	high, err := p(d.High)
	if err != nil { return TickerCandle{}, err }
	low, err := p(d.Low)
	if err != nil { return TickerCandle{}, err }
	cls, err := p(d.Close)
	if err != nil { return TickerCandle{}, err }
	vol, err := v(d.Volume)
	if err != nil { return TickerCandle{}, err }
	bid, err := p(d.InsideBidPrice)
	if err != nil { return TickerCandle{}, err }
	ask, err := p(d.InsideAskPrice)
	if err != nil { return TickerCandle{}, err }
	return TickerCandle{
		DateTime: dt, Open: open, High: high, Low: low, Close: cls,
		Volume: vol, InsideBid: bid, InsideAsk: ask, Symbol: d.Symbol,
	}, nil
}

// post signs and sends a POST to basePath+path. body is the exact bytes signed and sent.
func (c *Client) post(ctx context.Context, path string, body []byte) ([]byte, error) {
	fullPath := basePath + path
	uid := uuid.NewString()
	ts := strconv.FormatInt(c.now().UnixMilli(), 10)

	sts := buildStringToSign(c.apikey, "POST", c.host, fullPath, "", contentType, uid, ts, string(body))
	signature := sign(c.secret, sts)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+fullPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-INVX-APIKEY", c.apikey)
	req.Header.Set("X-INVX-SIGNATURE", signature)
	req.Header.Set("X-INVX-REQUEST-UID", uid)
	req.Header.Set("X-INVX-TIMESTAMP", ts)

	res, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request %s: %w", path, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("read response %s: %w", path, err)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d for %s: %s", res.StatusCode, path, string(raw))
	}
	return raw, nil
}
```

- [ ] **Step 7: Run the client test**

Run: `go test ./internal/invx/ -v`
Expected: PASS — all signing + transport tests.

- [ ] **Step 8: Commit**

```bash
git add internal/invx/
git commit -m "feat: invx signed client with ticker candles"
```

---

### Task 8: Collector — poll ticker, upsert all candles

**Files:**
- Create: `internal/collector/collector.go`
- Test: `internal/collector/collector_test.go`

**Interfaces:**
- Consumes: `invx.TickerCandle` (Task 7), `candle.Candle` (Task 6).
- Produces:
  - `type TickerFetcher interface { Ticker(ctx context.Context, symbol string) ([]invx.TickerCandle, error) }`
  - `type CandleUpserter interface { Upsert(ctx context.Context, c candle.Candle) error }`
  - `func New(f TickerFetcher, u CandleUpserter, symbols []string, interval time.Duration) *Collector`
  - `func (c *Collector) PollOnce(ctx context.Context) (int, error)` — returns candles upserted across all symbols.
  - `func (c *Collector) Run(ctx context.Context) error` — loops PollOnce every interval until ctx cancelled.

- [ ] **Step 1: Write the failing test**

`internal/collector/collector_test.go`:
```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/collector/ -v`
Expected: FAIL — `undefined: New`.

- [ ] **Step 3: Implement collector**

`internal/collector/collector.go`:
```go
package collector

import (
	"context"
	"fmt"
	"log"
	"time"

	"snow-white/internal/candle"
	"snow-white/internal/invx"
)

type TickerFetcher interface {
	Ticker(ctx context.Context, symbol string) ([]invx.TickerCandle, error)
}

type CandleUpserter interface {
	Upsert(ctx context.Context, c candle.Candle) error
}

type Collector struct {
	fetcher  TickerFetcher
	store    CandleUpserter
	symbols  []string
	interval time.Duration
}

func New(f TickerFetcher, u CandleUpserter, symbols []string, interval time.Duration) *Collector {
	return &Collector{fetcher: f, store: u, symbols: symbols, interval: interval}
}

// PollOnce fetches each symbol once and upserts every returned candle.
func (c *Collector) PollOnce(ctx context.Context) (int, error) {
	total := 0
	for _, sym := range c.symbols {
		tcs, err := c.fetcher.Ticker(ctx, sym)
		if err != nil {
			return total, fmt.Errorf("fetch ticker %s: %w", sym, err)
		}
		for _, tc := range tcs {
			cd := candle.Candle{
				Symbol:    tc.Symbol,
				OpenTime:  tc.DateTime,
				Open:      tc.Open,
				High:      tc.High,
				Low:       tc.Low,
				Close:     tc.Close,
				Volume:    tc.Volume,
				InsideBid: tc.InsideBid,
				InsideAsk: tc.InsideAsk,
				Source:    "ticker",
			}
			if err := c.store.Upsert(ctx, cd); err != nil {
				return total, fmt.Errorf("upsert %s: %w", sym, err)
			}
			total++
		}
	}
	return total, nil
}

// Run polls every interval until ctx is cancelled. The first poll logs how many
// candles each call returned per symbol so backfill behavior is observed.
func (c *Collector) Run(ctx context.Context) error {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	n, err := c.PollOnce(ctx)
	if err != nil {
		log.Printf("collector: first poll error: %v", err)
	} else {
		log.Printf("collector: first poll upserted %d candle(s) across %d symbol(s)", n, len(c.symbols))
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			n, err := c.PollOnce(ctx)
			if err != nil {
				log.Printf("collector: poll error: %v", err)
				continue
			}
			log.Printf("collector: upserted %d candle(s)", n)
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/collector/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/collector/
git commit -m "feat: collector polls ticker and upserts all candles"
```

---

### Task 9: `collect` command (daemon wiring)

**Files:**
- Create: `internal/cli/collect.go`
- Modify: `internal/cli/root.go` (attach subcommand)

**Interfaces:**
- Consumes: `config.Load` (Task 2), `invx.New` (Task 7), `candle.NewStore` (Task 6), `collector.New` (Task 8).
- Produces: `func newCollectCmd() *cobra.Command`; attached in `NewRootCmd`.

This task has no unit test (it is thin wiring over tested units); it ends with a manual run against the dev API.

- [ ] **Step 1: Implement the command**

`internal/cli/collect.go`:
```go
package cli

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"snow-white/internal/candle"
	"snow-white/internal/collector"
	"snow-white/internal/config"
	"snow-white/internal/invx"
)

func newCollectCmd() *cobra.Command {
	var symbolsFlag []string
	var intervalFlag time.Duration

	cmd := &cobra.Command{
		Use:   "collect",
		Short: "Poll ticker candles into Postgres (daemon)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			symbols := cfg.Symbols
			if len(symbolsFlag) > 0 {
				symbols = symbolsFlag
			}
			if len(symbols) == 0 {
				return fmt.Errorf("no symbols: set INVX_SYMBOLS or --symbols")
			}
			interval := cfg.CollectInterval
			if intervalFlag > 0 {
				interval = intervalFlag
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			pool, err := pgxpool.New(ctx, cfg.PSQLURL)
			if err != nil {
				return fmt.Errorf("connect postgres: %w", err)
			}
			defer pool.Close()

			client := invx.New(cfg.APIKey, cfg.Secret, cfg.Host, nil)
			store := candle.NewStore(pool)
			col := collector.New(client, store, symbols, interval)

			if err := col.Run(ctx); err != nil && err != context.Canceled {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&symbolsFlag, "symbols", nil, "symbols to collect (overrides INVX_SYMBOLS)")
	cmd.Flags().DurationVar(&intervalFlag, "interval", 0, "poll interval (overrides INVX_COLLECT_INTERVAL)")
	return cmd
}
```

- [ ] **Step 2: Attach to root**

Modify `internal/cli/root.go` — add at the end of `NewRootCmd` before `return`:
```go
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "snow-white",
		Short:         "InnovestX market-data collection, analysis, and trading CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newCollectCmd())
	return root
}
```

- [ ] **Step 3: Build**

Run: `go build -o snow-white . && ./snow-white collect --help`
Expected: build OK; help shows `--symbols` and `--interval`.

- [ ] **Step 4: Manual verification (real dev API + DB)**

Prereq: `.env` filled with a Read-scoped dev key, `PSQL_URL` pointing at the migrated dev DB.
Run: `./snow-white collect --symbols BTCTHB --interval 60s` for ~2 minutes, then Ctrl-C.
Expected logs: "first poll upserted N candle(s)…". Then:
```bash
psql "$PSQL_URL" -c "SELECT symbol, open_time, close FROM candles ORDER BY open_time DESC LIMIT 5;"
```
Expected: rows present with sane satang `close` values. **Record N from the first-poll log** — that answers whether the endpoint backfills (N>1) or returns latest-only (N==1).

- [ ] **Step 5: Commit**

```bash
git add internal/cli/collect.go internal/cli/root.go
git commit -m "feat: collect daemon command"
```

---

### Task 10: Indicator package (SMA, EMA, RSI)

**Files:**
- Create: `internal/indicator/indicator.go`
- Test: `internal/indicator/indicator_test.go`

**Interfaces:**
- Produces (operate on satang `[]int64` close prices; output `[]float64` aligned to input, warm-up positions are `math.NaN()`):
  - `func SMA(values []int64, period int) []float64`
  - `func EMA(values []int64, period int) []float64`
  - `func RSI(values []int64, period int) []float64`
  - `func IsWarm(v float64) bool` — `!math.IsNaN(v)`.

- [ ] **Step 1: Write the failing test**

`internal/indicator/indicator_test.go`:
```go
package indicator

import (
	"math"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestSMA(t *testing.T) {
	in := []int64{10, 20, 30, 40}
	got := SMA(in, 2)
	if len(got) != 4 {
		t.Fatalf("len = %d", len(got))
	}
	if IsWarm(got[0]) {
		t.Errorf("got[0] should be warm-up NaN")
	}
	if !approx(got[1], 15) || !approx(got[2], 25) || !approx(got[3], 35) {
		t.Errorf("SMA = %v", got)
	}
}

func TestSMAConstantSeries(t *testing.T) {
	in := []int64{7, 7, 7, 7, 7}
	got := SMA(in, 3)
	for i := 2; i < len(got); i++ {
		if !approx(got[i], 7) {
			t.Errorf("SMA of constant = %v at %d", got[i], i)
		}
	}
}

func TestEMAFirstValueIsSMA(t *testing.T) {
	in := []int64{10, 20, 30, 40, 50}
	got := EMA(in, 3)
	// First defined EMA (index period-1) equals SMA of first `period` values.
	if !approx(got[2], 20) {
		t.Errorf("EMA[2] = %v, want 20 (SMA seed)", got[2])
	}
	if IsWarm(got[1]) {
		t.Errorf("EMA[1] must be NaN warm-up")
	}
}

func TestRSIAllGainsIs100(t *testing.T) {
	in := []int64{1, 2, 3, 4, 5, 6}
	got := RSI(in, 3)
	last := got[len(got)-1]
	if !approx(last, 100) {
		t.Errorf("RSI of monotonic rise = %v, want 100", last)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/indicator/ -v`
Expected: FAIL — `undefined: SMA`.

- [ ] **Step 3: Implement indicators**

`internal/indicator/indicator.go`:
```go
// Package indicator computes technical indicators over satang int64 close prices.
// Output slices align 1:1 with input; warm-up positions are math.NaN().
package indicator

import "math"

func nanSlice(n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	return out
}

// IsWarm reports whether an indicator value is defined (past warm-up).
func IsWarm(v float64) bool { return !math.IsNaN(v) }

// SMA is the simple moving average over `period` values.
func SMA(values []int64, period int) []float64 {
	out := nanSlice(len(values))
	if period <= 0 || len(values) < period {
		return out
	}
	var sum int64
	for i := 0; i < len(values); i++ {
		sum += values[i]
		if i >= period {
			sum -= values[i-period]
		}
		if i >= period-1 {
			out[i] = float64(sum) / float64(period)
		}
	}
	return out
}

// EMA seeds with the SMA of the first `period` values, then applies the standard
// multiplier 2/(period+1).
func EMA(values []int64, period int) []float64 {
	out := nanSlice(len(values))
	if period <= 0 || len(values) < period {
		return out
	}
	k := 2.0 / float64(period+1)
	var seed int64
	for i := 0; i < period; i++ {
		seed += values[i]
	}
	prev := float64(seed) / float64(period)
	out[period-1] = prev
	for i := period; i < len(values); i++ {
		prev = (float64(values[i])-prev)*k + prev
		out[i] = prev
	}
	return out
}

// RSI is Wilder's Relative Strength Index over `period`.
func RSI(values []int64, period int) []float64 {
	out := nanSlice(len(values))
	if period <= 0 || len(values) <= period {
		return out
	}
	var gain, loss float64
	for i := 1; i <= period; i++ {
		ch := float64(values[i] - values[i-1])
		if ch >= 0 {
			gain += ch
		} else {
			loss -= ch
		}
	}
	avgGain := gain / float64(period)
	avgLoss := loss / float64(period)
	out[period] = rsiFrom(avgGain, avgLoss)
	for i := period + 1; i < len(values); i++ {
		ch := float64(values[i] - values[i-1])
		g, l := 0.0, 0.0
		if ch >= 0 {
			g = ch
		} else {
			l = -ch
		}
		avgGain = (avgGain*float64(period-1) + g) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + l) / float64(period)
		out[i] = rsiFrom(avgGain, avgLoss)
	}
	return out
}

func rsiFrom(avgGain, avgLoss float64) float64 {
	if avgLoss == 0 {
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - (100 / (1 + rs))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/indicator/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/indicator/
git commit -m "feat: SMA/EMA/RSI indicators with NaN warm-up"
```

---

### Task 11: `analyze` command (read-only indicators)

**Files:**
- Create: `internal/cli/analyze.go`
- Create: `internal/analyze/analyze.go`
- Modify: `internal/cli/root.go` (attach subcommand)
- Test: `internal/analyze/analyze_test.go`

**Interfaces:**
- Consumes: `candle.Candle` (Task 6), `indicator` (Task 10), `pkg/scale` (Task 3).
- Produces:
  - `type Row struct { OpenTime time.Time; Close int64; SMA, EMA, RSI float64 }`
  - `func Compute(candles []candle.Candle, smaP, emaP, rsiP int) []Row`
  - `func FormatCSV(rows []Row) string`

- [ ] **Step 1: Write the failing test**

`internal/analyze/analyze_test.go`:
```go
package analyze

import (
	"strings"
	"testing"
	"time"

	"snow-white/internal/candle"
)

func mkCandles(closes ...int64) []candle.Candle {
	base := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	out := make([]candle.Candle, len(closes))
	for i, c := range closes {
		out[i] = candle.Candle{Symbol: "BTCTHB", OpenTime: base.Add(time.Duration(i) * time.Minute), Close: c}
	}
	return out
}

func TestComputeAlignsAndWarmsUp(t *testing.T) {
	rows := Compute(mkCandles(10, 20, 30, 40), 2, 0, 0)
	if len(rows) != 4 {
		t.Fatalf("len = %d", len(rows))
	}
	if rows[1].SMA != 15 {
		t.Errorf("SMA[1] = %v, want 15", rows[1].SMA)
	}
}

func TestFormatCSVHeader(t *testing.T) {
	out := FormatCSV(Compute(mkCandles(10, 20), 2, 0, 0))
	if !strings.HasPrefix(out, "open_time,close,sma,ema,rsi\n") {
		t.Fatalf("missing header: %q", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/analyze/ -v`
Expected: FAIL — `undefined: Compute`.

- [ ] **Step 3: Implement analyze core**

`internal/analyze/analyze.go`:
```go
package analyze

import (
	"fmt"
	"math"
	"strings"
	"time"

	"snow-white/internal/candle"
	"snow-white/internal/indicator"
	"snow-white/pkg/scale"
)

type Row struct {
	OpenTime time.Time
	Close    int64
	SMA      float64
	EMA      float64
	RSI      float64
}

// Compute returns one Row per candle. A period of 0 disables that indicator
// (its column stays NaN).
func Compute(candles []candle.Candle, smaP, emaP, rsiP int) []Row {
	closes := make([]int64, len(candles))
	for i, c := range candles {
		closes[i] = c.Close
	}
	var sma, ema, rsi []float64
	if smaP > 0 {
		sma = indicator.SMA(closes, smaP)
	}
	if emaP > 0 {
		ema = indicator.EMA(closes, emaP)
	}
	if rsiP > 0 {
		rsi = indicator.RSI(closes, rsiP)
	}
	rows := make([]Row, len(candles))
	for i, c := range candles {
		r := Row{OpenTime: c.OpenTime, Close: c.Close}
		r.SMA = at(sma, i)
		r.EMA = at(ema, i)
		r.RSI = at(rsi, i)
		rows[i] = r
	}
	return rows
}

func at(s []float64, i int) float64 {
	if s == nil {
		return math.NaN()
	}
	return s[i]
}

func FormatCSV(rows []Row) string {
	var b strings.Builder
	b.WriteString("open_time,close,sma,ema,rsi\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "%s,%s,%s,%s,%s\n",
			r.OpenTime.UTC().Format(time.RFC3339),
			scale.Format(r.Close, 2),
			fmtFloat(r.SMA), fmtFloat(r.EMA), fmtFloat(r.RSI),
		)
	}
	return b.String()
}

func fmtFloat(v float64) string {
	if v != v { // NaN
		return ""
	}
	return fmt.Sprintf("%.2f", v/100) // satang-scale indicator -> THB display
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/analyze/ -v`
Expected: PASS.

- [ ] **Step 5: Implement the command**

`internal/cli/analyze.go`:
```go
package cli

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"snow-white/internal/analyze"
	"snow-white/internal/candle"
	"snow-white/internal/config"
)

func newAnalyzeCmd() *cobra.Command {
	var symbol, out string
	var smaP, emaP, rsiP int
	var fromStr, toStr string

	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Compute indicators over stored candles (read-only)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if symbol == "" {
				return fmt.Errorf("--symbol required")
			}
			from, to, err := parseRange(fromStr, toStr)
			if err != nil {
				return err
			}
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

			candles, err := candle.NewStore(pool).List(ctx, symbol, from, to, 100000)
			if err != nil {
				return err
			}
			rows := analyze.Compute(candles, smaP, emaP, rsiP)
			// CSV is the implemented output; table/json formats are deferred (YAGNI).
			fmt.Print(analyze.FormatCSV(rows))
			return nil
		},
	}
	cmd.Flags().StringVar(&symbol, "symbol", "", "symbol, e.g. BTCTHB")
	cmd.Flags().IntVar(&smaP, "sma", 20, "SMA period (0 disables)")
	cmd.Flags().IntVar(&emaP, "ema", 0, "EMA period (0 disables)")
	cmd.Flags().IntVar(&rsiP, "rsi", 0, "RSI period (0 disables)")
	cmd.Flags().StringVar(&fromStr, "from", "", "start date YYYY-MM-DD (default: 30d ago)")
	cmd.Flags().StringVar(&toStr, "to", "", "end date YYYY-MM-DD (default: now)")
	cmd.Flags().StringVar(&out, "out", "csv", "output format: csv (table/json deferred)")
	return cmd
}

func parseRange(fromStr, toStr string) (time.Time, time.Time, error) {
	to := time.Now().UTC()
	from := to.AddDate(0, 0, -30)
	if fromStr != "" {
		t, err := time.Parse("2006-01-02", fromStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parse --from: %w", err)
		}
		from = t
	}
	if toStr != "" {
		t, err := time.Parse("2006-01-02", toStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parse --to: %w", err)
		}
		to = t.Add(24 * time.Hour)
	}
	return from, to, nil
}
```

- [ ] **Step 6: Attach to root + build**

Modify `internal/cli/root.go` — add `root.AddCommand(newAnalyzeCmd())`.
Run: `go build -o snow-white . && go test ./... && ./snow-white analyze --help`
Expected: build OK, tests PASS, help shows flags.

- [ ] **Step 7: Commit**

```bash
git add internal/cli/analyze.go internal/analyze/ internal/cli/root.go
git commit -m "feat: analyze command (indicators over stored candles)"
```

---

### Task 12: Strategy package (interface + MA-cross)

**Files:**
- Create: `internal/strategy/strategy.go`
- Create: `internal/strategy/macross.go`
- Test: `internal/strategy/macross_test.go`

**Interfaces:**
- Consumes: `candle.Candle` (Task 6), `indicator` (Task 10).
- Produces:
  - `type Action int` with `Hold`, `Buy`, `Sell`; `func (a Action) String() string`.
  - `type Signal struct { Action Action; Reason string }`
  - `type Strategy interface { Name() string; WarmupBars() int; Evaluate(candles []candle.Candle) Signal }`
  - `type MACross struct { Fast, Slow int }`; `func NewMACross(fast, slow int) MACross`.

- [ ] **Step 1: Write the failing test**

`internal/strategy/macross_test.go`:
```go
package strategy

import (
	"testing"
	"time"

	"snow-white/internal/candle"
)

func candles(closes ...int64) []candle.Candle {
	base := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	out := make([]candle.Candle, len(closes))
	for i, c := range closes {
		out[i] = candle.Candle{Symbol: "BTCTHB", OpenTime: base.Add(time.Duration(i) * time.Minute), Close: c}
	}
	return out
}

func TestMACrossBuyOnGoldenCross(t *testing.T) {
	// Fast(2) crosses above Slow(3) on the last bar.
	// closes: prior bar fast<=slow, last bar fast>slow.
	cs := candles(10, 10, 10, 30, 60) // rising tail pushes fast above slow at the end
	sig := NewMACross(2, 3).Evaluate(cs)
	if sig.Action != Buy {
		t.Fatalf("Action = %v, want Buy (%s)", sig.Action, sig.Reason)
	}
}

func TestMACrossSellOnDeathCross(t *testing.T) {
	cs := candles(60, 60, 60, 30, 5) // falling tail pushes fast below slow at the end
	sig := NewMACross(2, 3).Evaluate(cs)
	if sig.Action != Sell {
		t.Fatalf("Action = %v, want Sell (%s)", sig.Action, sig.Reason)
	}
}

func TestMACrossHoldWhenNotWarm(t *testing.T) {
	cs := candles(10, 20) // fewer than Slow bars
	sig := NewMACross(2, 3).Evaluate(cs)
	if sig.Action != Hold {
		t.Fatalf("Action = %v, want Hold", sig.Action)
	}
}

func TestMACrossHoldWhenNoCross(t *testing.T) {
	cs := candles(10, 11, 12, 13, 14) // steady trend, fast stays above slow, no fresh cross
	sig := NewMACross(2, 3).Evaluate(cs)
	if sig.Action != Hold {
		t.Fatalf("Action = %v, want Hold (no fresh cross)", sig.Action)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/strategy/ -v`
Expected: FAIL — `undefined: NewMACross`.

- [ ] **Step 3: Implement strategy interface**

`internal/strategy/strategy.go`:
```go
package strategy

import "snow-white/internal/candle"

type Action int

const (
	Hold Action = iota
	Buy
	Sell
)

func (a Action) String() string {
	switch a {
	case Buy:
		return "BUY"
	case Sell:
		return "SELL"
	default:
		return "HOLD"
	}
}

type Signal struct {
	Action Action
	Reason string
}

// Strategy evaluates a candle history and emits a signal for the most recent bar.
type Strategy interface {
	Name() string
	WarmupBars() int
	Evaluate(candles []candle.Candle) Signal
}
```

- [ ] **Step 4: Implement MA-cross**

`internal/strategy/macross.go`:
```go
package strategy

import (
	"fmt"

	"snow-white/internal/candle"
	"snow-white/internal/indicator"
)

// MACross emits Buy when the fast SMA crosses above the slow SMA on the latest
// bar, Sell when it crosses below, Hold otherwise.
type MACross struct {
	Fast int
	Slow int
}

func NewMACross(fast, slow int) MACross { return MACross{Fast: fast, Slow: slow} }

func (m MACross) Name() string { return fmt.Sprintf("macross(%d,%d)", m.Fast, m.Slow) }

func (m MACross) WarmupBars() int { return m.Slow + 1 }

func (m MACross) Evaluate(cs []candle.Candle) Signal {
	if len(cs) < m.WarmupBars() {
		return Signal{Action: Hold, Reason: "warming up"}
	}
	closes := make([]int64, len(cs))
	for i, c := range cs {
		closes[i] = c.Close
	}
	fast := indicator.SMA(closes, m.Fast)
	slow := indicator.SMA(closes, m.Slow)

	n := len(cs) - 1
	curFast, curSlow := fast[n], slow[n]
	prevFast, prevSlow := fast[n-1], slow[n-1]
	if !indicator.IsWarm(prevFast) || !indicator.IsWarm(prevSlow) {
		return Signal{Action: Hold, Reason: "warming up"}
	}

	switch {
	case prevFast <= prevSlow && curFast > curSlow:
		return Signal{Action: Buy, Reason: fmt.Sprintf("fast %.2f crossed above slow %.2f", curFast, curSlow)}
	case prevFast >= prevSlow && curFast < curSlow:
		return Signal{Action: Sell, Reason: fmt.Sprintf("fast %.2f crossed below slow %.2f", curFast, curSlow)}
	default:
		return Signal{Action: Hold, Reason: "no fresh cross"}
	}
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/strategy/ -v`
Expected: PASS. (If a directional test fails, adjust the test's close sequence so the fast/slow relationship actually flips on the last bar — verify by hand-computing SMA(2) and SMA(3) on the final two bars.)

- [ ] **Step 6: Commit**

```bash
git add internal/strategy/
git commit -m "feat: Strategy interface + MA-cross reference"
```

---

### Task 13: Backtest engine + `backtest` command

**Files:**
- Create: `internal/backtest/backtest.go`
- Create: `internal/cli/backtest.go`
- Modify: `internal/cli/root.go` (attach subcommand)
- Test: `internal/backtest/backtest_test.go`

**Interfaces:**
- Consumes: `candle.Candle` (Task 6), `strategy.Strategy` + `strategy.Buy/Sell/Hold` (Task 12), `pkg/scale` (Task 3).
- Produces:
  - `type Trade struct { Time time.Time; Action string; Price int64; Qty int64; CashAfter int64 }`
  - `type Result struct { Trades []Trade; StartCash, EndCash, PnL int64; NumTrades int; WinRate, MaxDrawdownPct float64 }`
  - `func Run(cs []candle.Candle, s strategy.Strategy, startCash int64, feeBps int) Result` — all-in/all-out: Buy spends all cash, Sell liquidates the position; fee in basis points of trade value.

- [ ] **Step 1: Write the failing test**

`internal/backtest/backtest_test.go`:
```go
package backtest

import (
	"testing"
	"time"

	"snow-white/internal/candle"
	"snow-white/internal/strategy"
)

// scriptStrategy emits a fixed action per bar index, for deterministic tests.
type scriptStrategy struct{ acts []strategy.Action }

func (s scriptStrategy) Name() string     { return "script" }
func (s scriptStrategy) WarmupBars() int   { return 0 }
func (s scriptStrategy) Evaluate(cs []candle.Candle) strategy.Signal {
	i := len(cs) - 1
	if i < len(s.acts) {
		return strategy.Signal{Action: s.acts[i]}
	}
	return strategy.Signal{Action: strategy.Hold}
}

func mk(closes ...int64) []candle.Candle {
	base := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	out := make([]candle.Candle, len(closes))
	for i, c := range closes {
		out[i] = candle.Candle{OpenTime: base.Add(time.Duration(i) * time.Minute), Close: c}
	}
	return out
}

func TestBuyLowSellHighProfit(t *testing.T) {
	// Buy at 100 satang, sell at 200 satang, no fee -> cash doubles.
	cs := mk(100, 200)
	acts := []strategy.Action{strategy.Buy, strategy.Sell}
	res := Run(cs, scriptStrategy{acts: acts}, 1000, 0)

	if res.NumTrades != 2 {
		t.Fatalf("NumTrades = %d, want 2", res.NumTrades)
	}
	if res.EndCash != 2000 {
		t.Fatalf("EndCash = %d, want 2000", res.EndCash)
	}
	if res.PnL != 1000 {
		t.Fatalf("PnL = %d, want 1000", res.PnL)
	}
}

func TestNoTradesKeepsCash(t *testing.T) {
	res := Run(mk(100, 110, 120), scriptStrategy{acts: nil}, 5000, 0)
	if res.EndCash != 5000 || res.NumTrades != 0 {
		t.Fatalf("EndCash=%d NumTrades=%d", res.EndCash, res.NumTrades)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/backtest/ -v`
Expected: FAIL — `undefined: Run`.

- [ ] **Step 3: Implement the engine**

`internal/backtest/backtest.go`:
```go
// Package backtest replays a strategy over historical candles. All-in/all-out:
// Buy converts all cash to position at the bar close; Sell liquidates fully.
// Money and position value are satang int64; the only float is the drawdown ratio.
package backtest

import (
	"time"

	"snow-white/internal/candle"
	"snow-white/internal/strategy"
)

type Trade struct {
	Time      time.Time
	Action    string
	Price     int64
	Qty       int64 // position units *1e8 bought/sold
	CashAfter int64
}

type Result struct {
	Trades         []Trade
	StartCash      int64
	EndCash        int64
	PnL            int64
	NumTrades      int
	WinRate        float64
	MaxDrawdownPct float64
}

func Run(cs []candle.Candle, s strategy.Strategy, startCash int64, feeBps int) Result {
	cash := startCash
	var posUnits int64 // *1e8
	var entryCash int64
	var wins, closed int

	res := Result{StartCash: startCash}
	peak := startCash

	for i := range cs {
		price := cs[i].Close // satang per 1 unit
		sig := s.Evaluate(cs[:i+1])

		switch sig.Action {
		case strategy.Buy:
			if posUnits == 0 && cash > 0 && price > 0 {
				gross := cash
				fee := gross * int64(feeBps) / 10000
				spend := gross - fee
				posUnits = spend * 1e8 / price // units *1e8
				entryCash = cash
				cash = 0
				res.Trades = append(res.Trades, Trade{cs[i].OpenTime, "BUY", price, posUnits, cash})
				res.NumTrades++
			}
		case strategy.Sell:
			if posUnits > 0 {
				gross := posUnits * price / 1e8
				fee := gross * int64(feeBps) / 10000
				cash += gross - fee
				if cash > entryCash {
					wins++
				}
				closed++
				res.Trades = append(res.Trades, Trade{cs[i].OpenTime, "SELL", price, posUnits, cash})
				res.NumTrades++
				posUnits = 0
			}
		}

		// Mark-to-market equity for drawdown.
		equity := cash + posUnits*price/1e8
		if equity > peak {
			peak = equity
		}
		if peak > 0 {
			dd := float64(peak-equity) / float64(peak)
			if dd > res.MaxDrawdownPct {
				res.MaxDrawdownPct = dd
			}
		}
	}

	// Liquidate any open position at the last close for final accounting.
	if posUnits > 0 && len(cs) > 0 {
		price := cs[len(cs)-1].Close
		cash += posUnits * price / 1e8
		posUnits = 0
	}
	res.EndCash = cash
	res.PnL = cash - startCash
	if closed > 0 {
		res.WinRate = float64(wins) / float64(closed)
	}
	return res
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/backtest/ -v`
Expected: PASS.

- [ ] **Step 5: Implement the command**

`internal/cli/backtest.go`:
```go
package cli

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"snow-white/internal/backtest"
	"snow-white/internal/candle"
	"snow-white/internal/config"
	"snow-white/internal/strategy"
	"snow-white/pkg/scale"
)

func newBacktestCmd() *cobra.Command {
	var symbol string
	var fast, slow, feeBps int
	var cashTHB float64
	var fromStr, toStr string

	cmd := &cobra.Command{
		Use:   "backtest",
		Short: "Replay the MA-cross strategy over stored candles",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if symbol == "" {
				return fmt.Errorf("--symbol required")
			}
			from, to, err := parseRange(fromStr, toStr)
			if err != nil {
				return err
			}
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

			cs, err := candle.NewStore(pool).List(ctx, symbol, from, to, 100000)
			if err != nil {
				return err
			}
			if len(cs) == 0 {
				return fmt.Errorf("no candles for %s in range", symbol)
			}
			startCash := int64(cashTHB * 100) // THB -> satang
			res := backtest.Run(cs, strategy.NewMACross(fast, slow), startCash, feeBps)

			fmt.Printf("strategy:   macross(%d,%d)\n", fast, slow)
			fmt.Printf("candles:    %d  (%s .. %s)\n", len(cs), cs[0].OpenTime.Format("2006-01-02 15:04"), cs[len(cs)-1].OpenTime.Format("2006-01-02 15:04"))
			fmt.Printf("start cash: %s THB\n", scale.Format(res.StartCash, 2))
			fmt.Printf("end cash:   %s THB\n", scale.Format(res.EndCash, 2))
			fmt.Printf("P&L:        %s THB\n", scale.Format(res.PnL, 2))
			fmt.Printf("trades:     %d  win rate: %.0f%%\n", res.NumTrades, res.WinRate*100)
			fmt.Printf("max draw:   %.1f%%\n", res.MaxDrawdownPct*100)
			return nil
		},
	}
	cmd.Flags().StringVar(&symbol, "symbol", "", "symbol, e.g. BTCTHB")
	cmd.Flags().IntVar(&fast, "fast", 20, "fast SMA period")
	cmd.Flags().IntVar(&slow, "slow", 50, "slow SMA period")
	cmd.Flags().IntVar(&feeBps, "fee-bps", 25, "fee in basis points per trade")
	cmd.Flags().Float64Var(&cashTHB, "cash", 100000, "starting cash in THB")
	cmd.Flags().StringVar(&fromStr, "from", "", "start date YYYY-MM-DD (default: 30d ago)")
	cmd.Flags().StringVar(&toStr, "to", "", "end date YYYY-MM-DD (default: now)")
	return cmd
}
```

- [ ] **Step 6: Attach to root + full build/test**

Modify `internal/cli/root.go` — add `root.AddCommand(newBacktestCmd())`.
Run: `go build -o snow-white . && go test ./... && ./snow-white backtest --help`
Expected: build OK, all tests PASS, help shows flags.

- [ ] **Step 7: Commit**

```bash
git add internal/cli/backtest.go internal/backtest/ internal/cli/root.go
git commit -m "feat: backtest engine + command"
```

---

## Phase 1 Completion Gate

Before declaring Phase 1 done (per engagement rules — "done" = verified):

- [ ] `go test ./...` is green (unit + testcontainers integration).
- [ ] `task migrate-dev` applied the schema to the dev DB; `\d candles` shows the unique index.
- [ ] `./snow-white collect` ran against the dev API and wrote real candle rows (verified via `psql`); the first-poll candle count is recorded (answers the backfill question).
- [ ] `./snow-white analyze --symbol BTCTHB --sma 20` printed indicator rows from stored data.
- [ ] `./snow-white backtest --symbol BTCTHB --fast 20 --slow 50` printed a P&L report.

Phase 2 (trading: orders/positions/risk_state stores, risk guards, paper/live order pipeline, `trade` daemon, manual `order` commands, `kill`/`resume`, startup reconcile) gets its own plan authored after Phase 1 lands, referencing the real signatures produced here.

## Notes on Generated-Code Alignment

sqlc generates exact Go type and field names from the SQL. Tasks 6 reference `pgtype.Timestamptz` and param names like `OpenTime`, `RowLimit`, `FromTime`, `ToTime`. After `task sqlcgen`, read `sqlc/models.go` and `sqlc/candles.sql.go` and align the mapper/store to the actual emitted names and nullability. This is expected, not a deviation — the test in Task 6 Step 4 surfaces mismatches immediately.
