---
name: snow-white-cli
description: Use when operating the snow-white InnovestX trading CLI binary or making trading decisions with it — building it, setting up .env/Postgres, collecting candles, analyzing/backtesting, OR placing orders (manual order send/cancel, the trade daemon, kill/resume, balance, status). Establishes a disciplined expert-trader mindset (discipline, analysis, risk management, mental toughness) and covers the paper-default safety model, --live gate, risk caps, and known live-API quirks (500 THB minimum, clientOrderId, value-vs-quantity orders).
---

# snow-white CLI

## Overview

`snow-white` is a Go CLI for the InnovestX digital-asset exchange (all markets THB-quoted). It does three things: **collect** 1-minute OHLCV candles into Postgres, **analyze/backtest** TA over them, and **trade** (manual orders + an MA-cross auto-trader). Trading is **paper by default**; real orders require an explicit `--live` flag (and a y/N confirm on manual sends).

Money is integer **satang** (THB×100) internally; quantities are integer **×1e8**. Float appears only at display.

## Trader Mindset

Operate this tool as a skilled, disciplined crypto trader. Each trait maps to how you use the binary — the CLI exists to *enforce* the mindset, not replace it.

- **Discipline & self-control.** Follow the strategy; do not deviate even on losing trades. Don't revenge-trade, don't widen caps mid-session, don't override the risk guard. The guard, caps, and kill switch are mechanical discipline — respect them. Trade the plan you backtested, not the feeling.
- **Analytical skills.** Decide from data, not gut. Use `analyze` (SMA/EMA/RSI) and `backtest` over real collected candles to validate a strategy *before* going live. Read price and volume; size positions from the numbers.
- **Flexibility & responsiveness.** Markets move fast — the `trade` daemon re-evaluates every interval. Adapt `--fast/--slow`, symbol, and sizing to conditions, but change parameters *deliberately* (re-backtest), not impulsively mid-trade.
- **Risk management.** Have a plan before the order. Spread capital across assets/trades; cap exposure with `INVX_MAX_ORDER` / `INVX_MAX_DAILY` / `INVX_MAX_LOSS` (THB) and per-trade `--buy-thb` / `--qty`; keep the kill-file ready. Never risk more than the plan allows. Paper-trade first.
- **Mental toughness.** Don't let emotion or panic drive decisions. Let the daily loss-stop and caps act for you; if you must stop, `kill` — don't disable the guard. Stay calm in drawdowns; the discipline is in the rules, not the moment.

## Build

```bash
task build          # or: go build -o snow-white .
```

## Configure (.env)

Copy `.env.example` → `.env` (gitignored). Loaded via viper. Required keys:

| Key | Notes |
|-----|-------|
| `INVX_APIKEY` / `INVX_SECRET` | Read+Trading key (no Withdraw), single-line. |
| `INVX_HOST` | `api.innovestxonline.com` (prod, resolves). The `api-dev` host is VPN-gated (NXDOMAIN off-VPN). |
| `PSQL_URL` | Runtime DB DSN. Quote it if it contains `&`. |
| `PSQL_DEV_URL` | `task migrate-dev` target. |
| `INVX_SYMBOLS`, `INVX_COLLECT_INTERVAL` | collector defaults. |
| `INVX_MAX_ORDER`, `INVX_MAX_DAILY`, `INVX_MAX_LOSS` | risk caps **in THB** (config converts to satang). |
| `INVX_KILL_FILE` | path; if this file exists, all trading halts. |

DB setup (once): `task migrate-dev && task migrate-run` applies `schema.hcl` to the dev and runtime DBs. The orders/positions/risk_state tables already exist.

## Commands

| Command | What | Key flags |
|---|---|---|
| `collect` | daemon: poll ticker → upsert candles | `--symbols`, `--interval` |
| `analyze` | read-only indicator CSV | `--symbol`, `--sma/--ema/--rsi`, `--from/--to` |
| `backtest` | replay MA-cross → P&L | `--symbol`, `--fast/--slow`, `--fee-bps`, `--cash` |
| `trade` | **auto-trader daemon (PAPER default)** | `--symbol`, `--fast/--slow`, `--buy-thb`, **`--live`**, `--interval` |
| `order send` | manual order (**dry-run unless `--live` + confirm**) | `--symbol`, `--side BUY\|SELL`, `--type LIMIT\|MARKET`, `--price`(THB), `--qty`(coin) or `--value`(THB), `--live` |
| `order cancel` | cancel an open order | `--order-id` or `--client-order-id` |
| `order open` / `order hist` | list open / historical orders (read API) | hist: `--symbol`, `--depth` |
| `balance` | account balances (read API) | — |
| `status` | today's risk state + position (reads PG) | `--symbol` |
| `kill` | halt all trading now (sets `risk_state.halted`) | `--reason` |
| `resume` | clear the halt (y/N confirm) | — |

Run any command with `--help` for the authoritative flags.

## Trading safety model (real money — read this)

- **PAPER is the default.** `trade` without `--live` and `order send` without `--live` make **zero order-API calls**. `--live` is the only switch that places real orders; manual `order send --live` also requires a `y/N` confirm.
- **Risk guard** (auto-trader): every order passes kill-switch (DB halt OR `--kill-file`) → daily loss-stop → per-order cap → daily cap, before being sent. Caps come from the `INVX_MAX_*` env vars (THB).
- **Kill switch:** `kill` (or `touch`-ing the kill-file) halts the auto-trader AND blocks manual `order send --live`. `resume` clears it.
- **Order sizing:** for a BUY, pass `--value` (THB to spend) — the CLI converts it to a quantity using `--price` (the API misinterprets raw value-based orders). For a SELL, pass `--qty`.

## Known live-API quirks (discovered against the real exchange)

| Quirk | Handling |
|---|---|
| **Minimum order = 500 THB.** Orders below it → `4017 Below minimum amount`. `/symbols` only exposes the lot *increment*, not this floor. | Size orders ≥ 500 THB. |
| **`clientOrderId` must be > 0** (docs say "optional"); 0 → `4022`. | CLI sets it automatically. |
| **`value`-based limit orders are misinterpreted** (the exchange built a 4,161 BTC order from `value:500`). | CLI always sends **quantity**; `--value` is converted to qty via `--price`. Never send raw value. |
| Header names are case-sensitive; HTTP/2 lowercases them. | Client pins HTTP/1.1 + exact-case `X-INVX-*` headers. |
| Timestamp skew > 150s → `4011`. | Keep the host clock correct. |

## Verified working example

Buy ~500 THB of BTC (marketable limit, real fill):
```bash
# price the limit ~1% above the inside ask so it fills; --value caps spend
./snow-white order send --symbol BTCTHB --side BUY --type LIMIT --price 2080000 --value 500 --live
./snow-white order hist --symbol BTCTHB   # confirm FullyExecuted
./snow-white balance                      # confirm BTC up, THB down
```

## Common mistakes

| Symptom | Cause / fix |
|---|---|
| `collect`/order calls fail DNS | `INVX_HOST` is the dev host off-VPN — use the prod host. |
| `4017 Below minimum amount` | order < 500 THB. |
| `4001 Not_Enough_Funds` on a small order | likely a raw `value` order (use `--value` via the CLI, which converts to qty) OR insufficient cleared THB. |
| `order send` did nothing | no `--live` (dry-run), or kill-file present, or `risk_state.halted`. |
| `status` doesn't show a manual live order | manual sends aren't yet written to the DB ledger (known limitation); track via `order hist`. |

## Known limitations (Phase-2 follow-ups)

- Manual `order send --live` is **not** in the automated risk ledger: it doesn't increment `spent_today`, isn't reconciled, and isn't in `status` — track it via `order open`/`order hist`.
- Live position/PnL is deferred: the auto-trader counts `spent_today` for the cap but does not yet record live fill quantity/PnL, so the daily **loss-stop does not auto-fire on live fills** and `status` P&L is paper-only until a live fill-reconcile lands.
