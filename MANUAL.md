# snow-white — User Manual

A command-line trader for the **InnovestX** digital-asset exchange (all markets quoted in THB). It collects market data, analyzes/backtests strategies, and places orders — manually or via an automated MA-cross trader. **Trading is paper (dry-run) by default; real orders require an explicit `--live` flag.**

> ⚠️ **Real money.** Read [Trading Safety](#trading-safety) before using `--live`. The defaults are designed to make accidental real orders impossible.

---

## Table of contents
1. [Concepts](#concepts)
2. [Install & build](#install--build)
3. [Configuration (.env)](#configuration-env)
4. [Database setup](#database-setup)
5. [Quick start](#quick-start)
6. [Commands](#commands)
7. [Trading safety](#trading-safety)
8. [Money model](#money-model)
9. [Notifications & session tracking](#notifications--session-tracking)
10. [Known live-API quirks](#known-live-api-quirks)
11. [Limitations](#limitations)
12. [Troubleshooting](#troubleshooting)

---

## Concepts

- **Candles** — 1-minute OHLCV bars pulled from the exchange ticker and stored in Postgres. The ticker call *backfills ~100 candles per request*, so history accumulates fast.
- **Strategy** — currently one: **MA-cross** (fast SMA crosses slow SMA → Buy / Sell, else Hold). Pluggable interface for adding more.
- **Paper vs live** — paper mode logs/simulates orders and calls **no** order API; `--live` places real orders.
- **Risk guard** — every automated order passes kill-switch → daily loss-stop → per-order cap → daily cap before it can be sent.
- **All markets are THB-quoted** (BTCTHB, ETHTHB, USDTTHB, …). There is no BTC/USDT pair — to swap coins you sell one to THB and buy the other.

---

## Install & build

Requirements: Go 1.25+, a reachable Postgres, [`go-task`](https://taskfile.dev). `atlas` + `sqlc` only needed if you change the schema/queries.

```bash
task build          # or: go build -o snow-white .
./snow-white --help
```

---

## Configuration (.env)

Copy `.env.example` → `.env` (gitignored) and fill in. Config is loaded by viper; environment variables override the file. Keys:

| Key | Required | Notes |
|-----|:--:|-------|
| `INVX_APIKEY` / `INVX_SECRET` | ✓ | API key from trade.innovestxonline.com → API Keys. **Scope: Read + Trading, no Withdraw.** IP-whitelist it. Secret is shown once. Single line, no quotes. |
| `INVX_HOST` | ✓ | `api.innovestxonline.com` (prod). The dev host `api-dev.innovestxonline.com` is VPN-gated and will not resolve off-VPN. |
| `PSQL_URL` | ✓ | Runtime Postgres DSN (collect/analyze/trade/status read & write here). Quote the value if it contains `&`. |
| `PSQL_DEV_URL` | for migrations | Target of `task migrate-dev`. |
| `INVX_SYMBOLS` | | Default collect list, e.g. `BTCTHB,ETHTHB`. |
| `INVX_COLLECT_INTERVAL` | | Default poll/eval interval, e.g. `60s`. |
| `INVX_MAX_ORDER` | | Per-order cap **in THB** (config converts to satang). |
| `INVX_MAX_DAILY` | | Daily deployed cap **in THB**. |
| `INVX_MAX_LOSS` | | Daily realized-loss cap **in THB** → auto-halt when hit. |
| `INVX_KILL_FILE` | | Path; if this file exists, all trading halts. e.g. `./.halt`. |
| `DISCORD_BOT_URL` | | Discord webhook URL for order/notify messages (optional). |

> The risk caps are entered in **THB** (human-friendly). Internally they are satang.

---

## Database setup

The schema lives in `schema.hcl` (Atlas). Apply it once to both the dev DB and the runtime DB:

```bash
task migrate-dev     # applies schema to PSQL_DEV_URL
task migrate-run     # applies schema to PSQL_URL (the DB the binary uses at runtime)
```

Tables: `candles`, `orders`, `positions`, `risk_state`, `session_tracks`.

---

## Quick start

```bash
# 1. Collect candles (leave running; first poll backfills ~100 min of history)
./snow-white collect --symbols BTCTHB,ETHTHB --interval 60s

# 2. Inspect indicators over what was collected (read-only)
./snow-white analyze --symbol BTCTHB --sma 20 --rsi 14

# 3. Backtest a strategy on the collected history BEFORE trusting it
./snow-white backtest --symbol BTCTHB --fast 5 --slow 20 --fee-bps 25 --cash 100000

# 4. Paper-trade the strategy (no real orders)
./snow-white trade --symbol BTCTHB --fast 5 --slow 20 --buy-thb 1000 --interval 60s

# 5. Go live only when you trust it (real money — caps + kill switch enforced)
./snow-white trade --symbol BTCTHB --fast 5 --slow 20 --buy-thb 1000 --live
```

---

## Commands

Run any command with `--help` for the authoritative flag list.

### `collect` — candle daemon
Polls the ticker each interval and upserts candles into Postgres (idempotent). Runs until Ctrl-C / SIGTERM.
```bash
./snow-white collect --symbols BTCTHB,ETHTHB --interval 60s
```

### `analyze` — indicators (read-only)
Prints a CSV of indicators over stored candles. `0` disables an indicator.
```bash
./snow-white analyze --symbol BTCTHB --sma 20 --ema 50 --rsi 14 --from 2026-06-01
```
Columns: `open_time,close,sma,ema,rsi` (close/sma/ema in THB; rsi is the raw 0–100 oscillator).

### `backtest` — replay a strategy
```bash
./snow-white backtest --symbol BTCTHB --fast 20 --slow 50 --fee-bps 25 --cash 100000
```
Reports start/end cash, P&L (THB), trade count, win rate, max drawdown.

### `trade` — automated MA-cross trader (PAPER default)
```bash
./snow-white trade --symbol BTCTHB --fast 5 --slow 20 --buy-thb 1000 --interval 60s   # paper
./snow-white trade --symbol BTCTHB --fast 5 --slow 20 --buy-thb 1000 --live            # REAL
```
Each interval: reconcile prior live fills (live only) → evaluate the strategy → on Buy (when flat) deploy `--buy-thb`, on Sell (when holding) sell the position. Every order passes the risk guard. Records a `session_tracks` snapshot at start and stop.

### `order` — manual orders
`order send` is **dry-run unless `--live`** (and `--live` prompts `y/N`).
```bash
# dry-run (prints intent, sends nothing):
./snow-white order send --symbol BTCTHB --side BUY --type LIMIT --price 2080000 --value 500
# real order (≈500 THB of BTC, marketable limit):
./snow-white order send --symbol BTCTHB --side BUY --type LIMIT --price 2080000 --value 500 --live
```
- BUY: pass `--value` (THB to spend) — the CLI converts it to a quantity via `--price`. SELL: pass `--qty` (coin amount).
- `order cancel --order-id <id>` (or `--client-order-id`)
- `order open` — list resting orders (live API)
- `order hist --symbol BTCTHB [--depth 200]` — order history (live API)

### `balance` — account balances (live API)
```bash
./snow-white balance
```

### `status` — today's risk state + position (reads Postgres)
```bash
./snow-white status --symbol BTCTHB
```

### `kill` / `resume` — emergency halt
```bash
./snow-white kill --reason "stepping away"   # halts all trading now (no confirm)
./snow-white resume                          # clears the halt (y/N confirm)
```
`touch`-ing the `INVX_KILL_FILE` also halts trading.

### `session start` / `session end` — net-worth snapshot
Records combined net worth (THB + crypto valued at market) to `session_tracks`. Auto-invoked by the `trade` daemon; also callable manually.
```bash
./snow-white session start
./snow-white session end
```

### `notify` — send a Discord message
```bash
./snow-white notify "daily report: +1.2% on BTCTHB, 3 trades"
```
Requires `DISCORD_BOT_URL`.

---

## Trading safety

- **Paper is the default.** `trade` and `order send` without `--live` make **zero order-API calls**. `--live` is the only switch that places real orders; `order send --live` also requires a `y/N` confirmation.
- **Risk guard** (automated trader, every order, before sending):
  1. kill switch — DB halt flag OR the kill-file
  2. daily loss-stop — halts when realized loss ≥ `INVX_MAX_LOSS`
  3. per-order cap — `INVX_MAX_ORDER`
  4. daily cap — `INVX_MAX_DAILY`
- **Kill switch** binds the auto-trader **and** manual `order send --live`. `kill` to halt, `resume` to clear.
- **Key scope**: Read + Trading only — the bot can never withdraw funds.

---

## Money model

- Money is integer **satang** (THB × 100) everywhere internally; asset quantity is integer **×1e8**. Floating point appears only at display.
- Decimal values sent to the API are exact (no float rounding).
- Minimum order on the exchange is **500 THB** (see quirks).

---

## Notifications & session tracking

- **Discord:** if `DISCORD_BOT_URL` is set, every order the bot places (paper or live) pings Discord, and you can send ad-hoc messages with `notify`. In the `trade` daemon the send is fire-and-forget (never blocks trading); a Discord outage never affects an order.
- **Session tracking:** the `trade` daemon writes a `session_tracks` row (combined net worth in satang) at start and stop, so you can measure per-session P&L. `session start`/`end` do the same manually.

---

## Known live-API quirks

Discovered against the real exchange and handled by the CLI:

| Quirk | Handling |
|-------|----------|
| **Minimum order = 500 THB** (below → `4017`). `/symbols` exposes only the lot increment, not this floor. | Size orders ≥ 500 THB. |
| **`clientOrderId` must be > 0** (docs say "optional"; 0 → `4022`). | CLI sets it automatically. |
| **Raw `value`-based limit orders are misinterpreted** (the exchange built a 4,161-BTC order from `value:500`). | CLI always sends **quantity**; `--value` is converted via `--price`. |
| **Header names are case-sensitive; HTTP/2 lowercases them.** | Client pins HTTP/1.1 + exact-case `X-INVX-*` headers. |
| **Timestamp skew > 150 s → `4011`.** | Keep the host clock correct. |

---

## Limitations

- **Manual live orders are not in the automated risk ledger** — they don't increment `spent_today`, aren't reconciled, and don't appear in `status`. Track them via `order open` / `order hist`.
- **Partial fills** of a resting limit order stay `pending` until fully executed; the fill-reconcile applies only fully-executed orders.
- **Single trader instance per symbol** — running two trader processes against the same symbol concurrently can double-apply fills (position read is outside the fill transaction).
- One strategy (MA-cross) and THB-quoted markets only.

---

## Troubleshooting

| Symptom | Cause / fix |
|---------|-------------|
| DNS / NXDOMAIN on API calls | `INVX_HOST` is the dev host off-VPN — use `api.innovestxonline.com`. |
| `analyze`/`backtest` "no candles" | Nothing collected yet, or schema not applied to `PSQL_URL`. Run `collect` and `task migrate-run`. |
| `4017 Below minimum amount` | Order < 500 THB. |
| `4001 Not_Enough_Funds` on a small order | A raw `value` order (use the CLI, which converts to qty), or insufficient cleared THB. |
| `4002` / `4005` / `4011` / `4022` | Bad key / bad signature / clock skew > 150s / clientOrderId ≤ 0. |
| `order send` did nothing | No `--live` (dry-run), or kill-file present, or `risk_state.halted` (run `resume`). |
| `psql` can't read `$PSQL_URL` via `source .env` | The `&` in the DSN backgrounds the assignment in bash — pass the quoted DSN directly to `psql`, or rely on the binary. |

---

*Operate with discipline: think before action, always read real data, and remember that Buy/Sell is optional — Hold unless a trade has a real edge.*
