# InnovestX Trading CLI — Design Spec

**Date:** 2026-06-19
**Module:** `snow-white` (Go 1.25)
**Status:** Approved design, pre-implementation

## Purpose

A Go CLI that (1) collects InnovestX digital-asset market data into Postgres for
analysis and (2) runs technical-analysis (TA) automated trading on that data, plus
(3) a manual human-in-loop order CLI. The exchange API moves real money; safety
guardrails are first-class, not optional.

## Build Phases

Data must be trusted before any auto-order fires.

- **Phase 1 — Data + analysis.** `invx` client, config, `candles` schema, `collect`
  daemon, `analyze`, `backtest`. No order placement. Provably collecting real
  candles and producing indicator/backtest output.
- **Phase 2 — Trading.** `strategy` engine, `trader` daemon (paper default), manual
  `order` commands, risk guards, `kill`/`resume`.

## Decisions

| Axis | Choice |
|---|---|
| Goal | Collect candles for analysis + automated TA trading + manual order CLI |
| Strategy engine | Pluggable `Strategy` interface; MA-cross reference; shared by live + backtest |
| Storage | Postgres via pgx + sqlc + Atlas (`schema.hcl`) |
| Run model | Separate `collect` daemon + `trade` daemon (collector standalone) |
| Safety | Paper-mode default; per-order + daily spend caps; kill switch + daily-loss stop; Read+Trading-only key (no Withdraw), IP-whitelisted |
| Config/secrets | Viper env + `.env` (gitignored); secrets never as CLI args |
| Stack | Go 1.25, cobra, go-money, HMAC-SHA256 signed client |

### Strategy engine — chosen approach

Pluggable `Strategy` interface + one reference strategy (MA-cross) + backtest
harness. `Strategy.Evaluate(candles) -> Signal{Action, Size}`. The SAME strategy
code runs live and in backtest over PG history, so backtest is a true preview of
live behavior and doubles as the "analyze before risking funds" gate. New
strategies (RSI, breakout) plug in without touching collector/trader.

Rejected: hardcoded MA-cross only (live/backtest logic would drift; "TA indicators"
is plural). Deferred: config/YAML rules-engine (overkill, risky to get subtly wrong
with real money).

## API Constraints (verified from api-docs.innovestxonline.com, 2026-06-19)

- **Auth:** every request carries `X-INVX-APIKEY`, `X-INVX-SIGNATURE`,
  `X-INVX-REQUEST-UID` (uuid v4), `X-INVX-TIMESTAMP` (UTC ms), `Content-Type:
  application/json`.
- **Signature:** `hex(HMAC_SHA256(string_to_sign, secret))` where
  `string_to_sign = APIKEY + METHOD(upper) + host(lower) + path + query +
  ContentType + REQUEST-UID + TIMESTAMP + body`. Body must be signed byte-exact
  with what is sent (re-marshal reorders keys -> `4005 Invalid signature`).
- **Timestamp skew:** > 150 s from server time -> `4011`, request rejected. Client
  enforces a tighter ~120 s local margin and refuses to send beyond it.
- **Hosts:** prod `api.innovestxonline.com`, dev `api-dev.innovestxonline.com`.
  Base path `/api/v1/digital-asset/{path}`.
- **Candle source:** `POST /ticker/subscribe` returns OHLCV in `data[]` —
  `dateTime, open, high, low, close, volume, insideBidPrice, insideAskPrice,
  symbol` at 1-minute interval. All numeric fields are decimal **strings**.
- **Backfill handling (verify on first dev call):** the `ticker/subscribe` request
  body carries only `symbol` — there is **no from/to range param**, so we cannot
  *request* arbitrary history. Backfill = however many candles a single call's
  `data[]` returns. The collector **consumes every element in `data[]`** and upserts
  them all (idempotent on `(symbol, open_time)`), so:
  - if the endpoint returns only the latest minute -> history accumulates over time
    by polling each minute (warm-up: e.g. MA-50 needs ~50 min before first signal);
  - if it returns a window -> that window is backfilled automatically on the first
    call, shortening or eliminating warm-up.
  On collector startup, log how many candles the first call returned per symbol so
  the actual behavior is observed, not assumed. No design change either way.
- **Error codes used:** `0000` success; `4002` bad key; `4005` bad signature;
  `4011` timestamp skew; `4019` insufficient balance; `4041/4042` not found.
- **Endpoints used:** `/ticker/subscribe`, `/order/send`, `/order/cancel`,
  `/order/open/inquiry`, `/order/history/inquiry`, `/account/balance/inquiry`,
  `/symbols`, `/products`.

## Architecture & Layout

```
snow-white/
├── main.go                  # only main(): cobra root + viper load, dispatch
├── cmd/                     # cobra subcommands (thin: parse -> call internal)
│   ├── collect.go   # daemon: poll ticker -> candles
│   ├── analyze.go   # indicators/stats over PG history -> stdout/CSV/JSON
│   ├── backtest.go  # run a Strategy over PG history -> P&L report
│   ├── trade.go     # daemon: PG -> Strategy -> guarded order send (paper default)
│   ├── order.go     # MANUAL: send/cancel/open/hist + balance + status (confirm prompt)
│   └── risk.go      # kill / resume
├── internal/
│   ├── invx/        # API client: signing, transport, typed req/resp (the contract)
│   ├── config/      # viper typed Config + .env load
│   ├── candle/      # domain Candle, NewFromSQLC, store queries
│   ├── indicator/   # pure funcs: SMA/EMA/RSI (no I/O, unit-tested)
│   ├── strategy/    # Strategy interface + macross reference + Signal
│   ├── collector/   # poll loop, dedupe, upsert candles
│   ├── trader/      # signal->order pipeline + risk guards (caps/kill/loss)
│   └── sql/         # hand-written *.sql for sqlc
├── sqlc/            # GENERATED — never hand-edit
├── schema.hcl       # Atlas source of truth
├── schema.sql       # inspected schema (generated)
├── Taskfile.yml     # migrate-dev, generate-sql-schema, sqlcgen, build
└── .env             # gitignored: INVX_APIKEY, INVX_SECRET, PSQL_URL, ...
```

**Data flow:** `invx.Ticker -> collector -> PG candles` then
`candle store -> indicator -> strategy -> trader(guards) -> invx.SendOrder`.

`main.go` holds only `main()`; all wiring inside it. Domain structs carry `json`
tags only, mapped from sqlc via `NewFromSQLC`.

## Data Model (Postgres / `schema.hcl`)

Every table has a surrogate `id bigserial primary key`; natural keys are `UNIQUE`
and serve as upsert conflict targets. Money is `bigint` subunits (satang, THB×100);
asset quantity/volume is scaled int (×1e8). Float never appears in domain or DB —
only at display formatting.

```
candles
  id          bigserial   primary key
  symbol      text         not null
  open_time   timestamptz  not null            -- candle interval start (1-min)
  open, high, low, close   bigint not null      -- satang
  volume                   bigint not null      -- ×1e8
  inside_bid, inside_ask   bigint not null      -- satang
  source      text         not null default 'ticker'
  ingested_at timestamptz  not null default now()
  UNIQUE (symbol, open_time)                     -- upsert dedupe target

orders
  id           bigserial   primary key
  client_uid   uuid        not null UNIQUE       -- == X-INVX-REQUEST-UID, idempotency
  symbol       text not null
  side         text not null                     -- BUY / SELL
  type         text not null                     -- LIMIT / MARKET
  limit_price  bigint                            -- satang, null = market
  quantity     bigint not null                   -- ×1e8
  mode         text not null                     -- paper / live
  strategy     text                              -- which Strategy fired it; null = manual
  status       text not null                     -- pending/accepted/rejected/canceled
  exchange_ref text                              -- broker order id on accept
  reason       text                              -- reject / guard message
  created_at   timestamptz not null default now()

positions
  id            bigserial  primary key
  symbol        text not null UNIQUE
  qty           bigint not null default 0         -- ×1e8
  avg_cost      bigint not null default 0          -- satang
  realized_pnl  bigint not null default 0          -- satang
  updated_at    timestamptz not null default now()

risk_state                                        -- one row PER trading day
  id            bigserial  primary key
  day           date not null UNIQUE               -- daily reset; guard reads today's row
  halted        bool not null default false
  halt_reason   text
  spent_today   bigint not null default 0          -- satang
  loss_today    bigint not null default 0          -- satang
  updated_at    timestamptz not null default now()
```

Upserts: `ON CONFLICT (symbol, open_time)`, `(client_uid)`, `(symbol)`, `(day)`.
`risk_state` per-day rows give automatic cap/kill reset at date rollover plus a full
audit trail.

## CLI Surface (`snow-white <cmd>`, cobra)

Secrets only via env/.env. Money flags accept THB decimals, parsed once to satang
int at the boundary; internal math is all int. `--live` absent on any order path =>
paper / no API call.

```
collect   --symbols BTCTHB,ETHTHB  [--interval 60s]
          Daemon. Poll ticker each symbol per interval -> upsert candles.

analyze   --symbol BTCTHB  [--indicator sma:20,ema:50,rsi:14]
          [--from DATE] [--to DATE] [--out table|csv|json]
          Read-only indicators/stats over PG history. No orders.

backtest  --symbol BTCTHB --strategy macross [--fast 20 --slow 50]
          [--from --to] [--cash 100000] [--fee-bps N]
          Replay Strategy over PG candles -> P&L, win rate, max drawdown, trades.
          Same Strategy code as live. The "analyze before risking" gate.

trade     --symbol BTCTHB --strategy macross [--fast 20 --slow 50]
          [--live]                 # default = PAPER
          --max-order 5000         # THB cap per order
          --max-daily 50000        # THB deployed/day cap
          --max-loss 10000         # THB daily realized-loss -> auto-halt
          [--kill-file PATH]
          Daemon. PG candles -> indicator -> Strategy -> risk guards -> SendOrder.

order send   --symbol --side BUY --type LIMIT --price 900000 --qty 0.001 [--live]
order cancel --ref <exchange_ref | client_uid>
order open   [--symbol]
order hist   --symbol [--from --to]
balance
status                       # risk_state today: halted?, spent, loss, positions

kill   [--reason "..."]      # set risk_state.halted=true
resume                       # clear halt (confirm prompt)
```

`order send --live` and `resume` prompt y/N. `collect` and `trade` are the only
daemons; all else one-shot.

## Safety Internals

**Risk guard** — every order (paper or live) passes `trader.guard` before the mode
branch, so caps are validated even in dry-run:

```
guard(order):
  load risk_state for today (create row if date rolled over -> caps reset)
  1. halted?                              -> reject "kill switch active"
  2. order.thb > max-order                -> reject "exceeds per-order cap"
  3. spent_today + order.thb > max-daily  -> reject "exceeds daily cap"
  4. loss_today >= max-loss               -> set halted=true; reject "daily loss stop"
  pass -> proceed; on live accept: spent_today += thb (same tx as order/position write)
```

**Kill switch** = `risk_state.halted` (DB) OR a watched `--kill-file`; either trips
it. `kill`/`resume` flip the DB flag. Trader checks both at the top of each cycle.

**Order pipeline** (shared by `trader` and manual `order send`):

```
build order -> guard
  paper: insert orders(mode=paper, status=accepted), log, STOP (no API call)
  live:  client_uid = uuid v4
         insert orders(mode=live, status=pending)        # idempotency record FIRST
         invx.SendOrder(signed)  # client_uid == X-INVX-REQUEST-UID
         on 0000:  status=accepted, exchange_ref; update position + spent_today (1 tx)
         on error: status=rejected, reason=code/msg
```

`client_uid` persisted before the API call: a crash mid-send leaves a `pending`
row; startup reconcile queries `order hist` to settle pending -> accepted/rejected.
No lost or duplicate orders. All multi-row writes use one `pgx.Tx`.

**Signing** (`invx`, the only place crypto happens): body marshaled once to `[]byte`,
that exact slice both signed and sent. `TS = time.Now().UnixMilli()`; refuse if
local skew would exceed ~120 s. Secret from viper, never logged, never an arg.

## Blast Radius / Reversibility

- **Worst case:** a live order at wrong price/size. Bounded by per-order + daily
  caps + daily-loss stop + kill switch. Key has no Withdraw permission, so funds
  cannot leave the exchange.
- **Reversibility:** paper is the default and side-effect-free; live orders
  cancelable via `order cancel`; halt is instant; all state in PG (auditable,
  replayable). `client_uid` idempotency prevents accidental duplicates on retry.

## Testing Strategy

**Pure units (table-driven, no I/O — primary confidence):**
- `indicator/` — SMA/EMA/RSI vs hand-computed fixtures; insufficient lookback ->
  no value (not zero); SMA of constant series == that constant.
- `strategy/macross` — Buy on fast-crosses-above-slow, Sell on below, Hold else; no
  signal until both windows warm.
- `trader/guard` — every branch: under/over per-order, daily rollover resets caps,
  loss stop sets halt, halted rejects. Fake clock + fake `risk_state`.
- `invx` signing — golden test: the doc's NodeJS example (known key/secret/body/ts)
  must reproduce a known signature; assert body-signed == body-sent.
- money/scale parsing — API decimal strings -> satang / ×1e8 int round-trip, no
  float drift.

**Integration (testcontainers Postgres):**
- candle upsert idempotency — same `(symbol, open_time)` twice -> one row, latest
  values.
- order pipeline in one tx — paper insert; live `pending`->`accepted` updates
  position + `spent_today` atomically; forced error -> `rejected`, no position/spend
  change.
- migration check — `schema.hcl` applies clean; sqlc queries compile against it.

**Transport (httptest stub, no live API in CI):** recorded fixtures for `0000`,
`4011` skew, `4005` bad sig, `4019` insufficient balance.

**Live API:** manual only, paper-first, then one tiny live order behind `--live` on
dev host `api-dev.innovestxonline.com`.

**Completion gate ("done" = verified):** `go test ./...` green + `task migrate-dev`
clean + collector observed writing real candles + backtest produces a P&L report +
paper trade logs intended orders — before any `--live`.

## Out of Scope (YAGNI)

- Deposit/withdraw endpoints (key has no Withdraw; treasury tooling deferred).
- Multiple strategies beyond the MA-cross reference (interface supports them; ship one).
- Web dashboard / TUI (Postgres is SQL-queryable for analysis now).
- Config/YAML rules-engine strategy authoring.
- Level-2 orderbook ingestion (ticker OHLCV is the TA source for MVP).
```
