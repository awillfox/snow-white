# Phase 2 Final Review — Fix Wave Report

Date: 2026-06-19
Branch: feat/invx-trading-phase2

---

## FIX 1 (CRITICAL — caps unit: THB in env, satang in code)

**Files changed:**
- `internal/config/config.go`: After `v.Unmarshal`, added `cfg.MaxOrder *= 100; cfg.MaxDaily *= 100; cfg.MaxLoss *= 100` with doc comment that env vars are THB and stored as satang.
- `internal/config/config_test.go` `TestLoadRiskCaps`: Changed env vars to THB values (`INVX_MAX_ORDER=5000`, `INVX_MAX_DAILY=50000`, `INVX_MAX_LOSS=10000`) and asserts satang (`cfg.MaxOrder == 500000`, `cfg.MaxDaily == 5000000`, `cfg.MaxLoss == 1000000`).
- `internal/cli/trade.go`: Startup banner now formats caps in THB using `scale.Format(caps.MaxOrder, 2)` etc., displaying e.g. `caps[order=5000.00THB daily=50000.00THB loss=10000.00THB]`. Added `snow-white/pkg/scale` import.

---

## FIX 2 (HIGH — kill switch must bind on manual live send)

**File changed:** `internal/cli/order.go`

After the y/N confirm, before calling `client.SendOrder`:
1. Calls `trader.KillFileTripped(cfg.KillFile)` — returns error `"blocked: kill file present (<path>)"` if tripped.
2. Opens pgxpool, calls `order.NewStore(pool).RiskToday(ctx, time.Now())` — returns error `"blocked: trading halted (resume to clear)"` if `state.Halted`.
3. Only if both pass, proceeds to `client.SendOrder`.

Added `snow-white/internal/trader` import. Dry-run path is unchanged.

---

## FIX 3 (HIGH — zero-size order guard)

**File changed:** `internal/cli/order.go`

After parsing `qtyUnits` and `valueSatang`, before building `SendOrderInput` (applies to both dry-run and live paths): if `qtyUnits == 0 && valueSatang == 0` → returns error `"order size is zero: provide a non-zero --qty or --value"`.

---

## FIX 4 (Minor cleanups)

- `internal/cli/trade.go`: Changed `err != context.Canceled` to `errors.Is(err, context.Canceled)`. Added `"errors"` import.
- `internal/cli/order.go`: Removed the unreachable `""` case from the `--type` switch (flag default is `"LIMIT"`, so `typeStr` is never empty). Added comment explaining why.

---

## Config Test Assertion Change

`TestLoadRiskCaps` before:
```
INVX_MAX_ORDER=500000  // labeled satang
assert: cfg.MaxOrder == 500000
```

After:
```
INVX_MAX_ORDER=5000    // THB (human label matches .env)
assert: cfg.MaxOrder == 500000  // satang (5000 × 100)
```

---

## Build + Test Output

```
$ go build -o snow-white .
(no output — success)

$ go test ./...
?     snow-white                   [no test files]
ok    snow-white/internal/analyze  (cached)
ok    snow-white/internal/backtest (cached)
ok    snow-white/internal/candle   (cached)
ok    snow-white/internal/cli      0.006s
ok    snow-white/internal/collector (cached)
ok    snow-white/internal/config   0.005s
ok    snow-white/internal/indicator (cached)
ok    snow-white/internal/invx     (cached)
ok    snow-white/internal/order    (cached)
ok    snow-white/internal/strategy (cached)
ok    snow-white/internal/trader   (cached)
ok    snow-white/pkg/scale         (cached)
?     snow-white/sqlc              [no test files]
```

All packages green.

---

## Manual Safety Checks

**Check 1 — zero-size send errors:**
```
$ ./snow-white order send --symbol BTCTHB --side BUY --type LIMIT --price 1000000 --value 0
error: order size is zero: provide a non-zero --qty or --value
exit: 1
```
PASS.

**Check 2 — --value 1000 without --live prints DRY-RUN:**
```
$ ./snow-white order send --symbol BTCTHB --side BUY --type LIMIT --price 1000000 --value 1000
order: symbol=BTCTHB side=BUY type=LIMIT price=1000000.00 qty=0.00000000 value=1000.00
DRY-RUN — would send the above order (pass --live to place)
exit: 0
```
PASS.
