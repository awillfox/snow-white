---
name: snow-white-cli
description: Use when operating the snow-white InnovestX CLI binary — building it, setting up .env/Postgres, collecting ticker candles, analyzing indicators, or backtesting a strategy. Covers the collect/analyze/backtest commands and their flags. Note: live order placement (trade/order/kill) is Phase 2 and NOT yet implemented.
---

# snow-white CLI

## Overview

`snow-white` is a Go CLI for the InnovestX digital-asset exchange. **Phase 1 (current)** does three read-only things: collect 1-minute OHLCV candles into Postgres, compute TA indicators over them, and backtest a moving-average-cross strategy. **It does NOT place orders** — `trade`, `order`, `kill`/`resume` are Phase 2 and not built. `analyze`/`backtest`/`collect` only read market data; they never touch funds.

Money is integer **satang** (THB×100) everywhere; it is divided to THB only at display. Asset volume is integer ×1e8.

## Prerequisites

- Go 1.25+ (build), a reachable Postgres, `go-task` (Taskfile). `atlas` + `sqlc` only needed to change schema/queries.
- An InnovestX API key from trade.innovestxonline.com → API Keys, scoped **Read + Trading, NO Withdraw**, IP-whitelisted.

## Build

```bash
task build          # or: go build -o snow-white .
```
Produces `./snow-white`.

## Configure (.env)

Copy `.env.example` → `.env` (gitignored) and fill in. Loaded via viper (env vars override the file). Required keys:

| Key | Notes |
|-----|-------|
| `INVX_APIKEY` / `INVX_SECRET` | The Read+Trading key. Single-line, no surrounding quotes/newlines. Secret signs every request. |
| `INVX_HOST` | `api.innovestxonline.com` (prod, resolves publicly). The dev host `api-dev.innovestxonline.com` is **VPN-gated and will NXDOMAIN** off-VPN. |
| `PSQL_URL` | DSN the binary reads/writes at runtime. Quote it if it contains `&` (e.g. Neon `...&channel_binding=require`). |
| `PSQL_DEV_URL` | DSN that `task migrate-dev` applies the schema to. |
| `INVX_SYMBOLS` | Default collect list, e.g. `BTCTHB,ETHTHB`. |
| `INVX_COLLECT_INTERVAL` | Default poll interval, e.g. `60s`. |

## Database setup (once, before `collect`)

The schema must exist in the DB the binary uses (`PSQL_URL`). The Taskfile applies `schema.hcl` via Atlas:

```bash
task migrate-dev    # applies schema to PSQL_DEV_URL
task migrate-run    # applies schema to PSQL_URL (the runtime DB) — needed before collect writes there
```
Verify: `psql "$PSQL_URL" -c "\d candles"` shows the `candles` table with a unique index on `(symbol, open_time)`.

## Commands (verified flags)

| Command | What it does | Key flags |
|---|---|---|
| `collect` | **Daemon.** Polls the ticker each interval, upserts every returned candle (the endpoint backfills ~100 one-minute candles per call). Ctrl-C / SIGTERM exits cleanly. | `--symbols BTCTHB,ETHTHB` (overrides `INVX_SYMBOLS`), `--interval 60s` |
| `analyze` | Read-only. Prints indicator CSV over stored candles. | `--symbol BTCTHB` (required), `--sma 20`, `--ema 0`, `--rsi 0` (0 disables), `--from/--to YYYY-MM-DD`, `--out csv` |
| `backtest` | Replays MA-cross over stored candles → P&L, win rate, max drawdown. | `--symbol BTCTHB` (required), `--fast 20`, `--slow 50`, `--fee-bps 25`, `--cash 100000` (THB), `--from/--to` |

Run any command with `--help` for the authoritative flag list.

## Typical workflow

```bash
# 1. Collect candles (leave running, or run periodically). First poll backfills ~100 min.
./snow-white collect --symbols BTCTHB,ETHTHB --interval 60s

# 2. Inspect indicators over what was collected (THB, 2 decimals; warm-up rows blank).
./snow-white analyze --symbol BTCTHB --sma 20 --rsi 14

# 3. Backtest a strategy on the collected history before trusting it.
./snow-white backtest --symbol BTCTHB --fast 5 --slow 20 --fee-bps 25 --cash 100000
```
`backtest` is the gate: validate a strategy on real collected data before Phase 2 trading is ever wired up.

## Output notes

- `analyze` CSV columns: `open_time,close,sma,ema,rsi`. `close`/`sma`/`ema` are THB; `rsi` is the raw 0–100 oscillator. Disabled/warm-up cells are empty.
- `backtest` prints start/end cash, P&L (THB), trade count, win rate, max drawdown %.

## Common mistakes

| Symptom | Cause / fix |
|---|---|
| `collect` errors with DNS/NXDOMAIN | `INVX_HOST` is the dev host off-VPN. Use `api.innovestxonline.com` or connect the VPN. |
| `analyze`/`backtest` returns "no candles" | Nothing collected yet, or schema not applied to `PSQL_URL`. Run `collect` first and `task migrate-run`. |
| API error `4002/4005/4011/4008/4003` | `4002` bad key, `4005` bad signature, `4011` clock >150s skew, `4008` request-uid rejected, `4003` IP not whitelisted. Check key scope, system clock, and the key's IP allowlist. |
| psql can't read `$PSQL_URL` via `source .env` | The `&` in the DSN backgrounds the assignment in bash. Pass the quoted DSN directly to `psql`, or rely on the binary (viper handles it). |

## Phase 2 — NOT yet implemented

Live/paper order placement, the `trade` daemon, manual `order send/cancel/open/hist`, `balance`, `status`, and `kill`/`resume` do **not** exist in this binary yet. The `orders`/`positions`/`risk_state` tables and the `.env` risk caps (`INVX_MAX_ORDER`, `INVX_MAX_DAILY`, `INVX_MAX_LOSS`, `INVX_KILL_FILE`) are scaffolding for that phase. Do not assume any command can move funds — none can.
