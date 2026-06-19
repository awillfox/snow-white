# notify-reconcile-fixes report
Date: 2026-06-19

## Changes made

### FIX A — Non-blocking Discord notify (`internal/trader/trader.go`)
`sendNotify` now fires the `t.notify.Send(...)` call inside a goroutine with a fresh
`context.WithTimeout(context.Background(), 10*time.Second)`. The nil-check stays
synchronous (no goroutine spawned for no-op case). The Tick ctx parameter is now
`_` — it is not passed to the goroutine (it may be cancelled before the goroutine
runs). `internal/cli/order.go` notify block is untouched but has a one-line comment
explaining it is intentionally synchronous (one-shot process must wait for delivery).

### FIX B1 — computeFill overflow guard (`internal/trader/fill.go`)
`computeFill` now returns 5 values: `(newQty, newAvgCost, newRealizedPnl, lossDelta int64, err error)`.
Before each `a * b` where a and b can be large, a guard checks
`if b != 0 && a > math.MaxInt64/b { return ..., fmt.Errorf(...) }`.
Four multiplication sites are guarded:
1. `pos.Qty * pos.AvgCost` (BUY costBefore)
2. `qtyExecuted * avgPrice` (BUY addCost / SELL proceeds)
3. `totalCost * 1e8` (BUY newAvgCost re-scale)
4. `qtyExecuted * pos.AvgCost` (SELL cost)

### FIX B2 — Partial-sell test (`internal/trader/fill_test.go`)
`TestComputeFill_PartialSell`: hold 0.002 BTC at 200_000_000_00 satang/coin,
sell 0.001 at 210_000_000_00 satang/coin → newQty=100_000, avgCost unchanged,
realized=1_000_000 satang, lossDelta=0.

### FIX B3 — Unit comment in fill.go
Doc comment on `computeFill` now explicitly states qtyExecuted/pos.Qty are ×1e8
scaled coin units while avgPrice/pos.AvgCost are satang per WHOLE coin, explaining
the /1e8 division that converts (scaled-units × satang-per-coin) → satang.

### FIX B4 — Single-instance assumption doc (`internal/trader/reconcile.go`)
Comment added at the top of `Reconcile`: GetPosition is read outside ApplyFill's
transaction; Reconcile assumes single trader process per symbol. Concurrent instances
would double-apply. No SQL changes.

### Reconcile overflow handling (`internal/trader/reconcile.go`)
In the FullyExecuted branch, if `computeFill` returns an error, Reconcile logs it
and `continue`s — the order stays PENDING. `ApplyFill` is NOT called so no
corrupted fill is written.

### Test updates (`internal/trader/trader_test.go`)
`fakeNotifier` changed from slice to `sent chan string` (buffered 4). Success tests
use `select { case msg := <-notif.sent ... case <-time.After(2s): t.Fatal }`.
No-notify cases (Hold / blocked Place / nil notifier) use
`select { case msg := <-notif.sent: t.Fatal ... case <-time.After(150ms): /* ok */ }`.

## Test run output

### `go test ./internal/trader/ -count=3 -race`
```
ok  	snow-white/internal/trader	1.939s
```
All 32 tests pass. No races detected across 3 runs. Zero flakes.

### `go test ./...`
```
ok  snow-white/internal/analyze    (cached)
ok  snow-white/internal/backtest   (cached)
ok  snow-white/internal/candle     (cached)
ok  snow-white/internal/cli        0.005s
ok  snow-white/internal/collector  (cached)
ok  snow-white/internal/config     (cached)
ok  snow-white/internal/discord    (cached)
ok  snow-white/internal/indicator  (cached)
ok  snow-white/internal/invx       (cached)
ok  snow-white/internal/order      (cached)
ok  snow-white/internal/strategy   (cached)
ok  snow-white/internal/trader     0.308s
ok  snow-white/pkg/scale           (cached)
```

### computeFill overflow test
`TestComputeFill_Overflow_ReturnsError`: passes — astronomically large pos.Qty ×
pos.AvgCost correctly returns a non-nil error instead of silent int64 wrap-around.

`TestReconcile_ComputeFillOverflow_LeavesOrderPending`: passes — Reconcile returns
n=0 (order left pending), 0 ApplyFill calls, no error returned to caller.

## Verification
- Tick no longer blocks on Discord: `sendNotify` spawns a goroutine; Tick returns
  immediately after `Place` regardless of webhook latency.
- computeFill returns error on overflow: confirmed by new overflow test.
- Reconcile leaves order pending on overflow: confirmed by new reconcile overflow test.
- Money math results for valid (non-overflow) inputs are unchanged: all 4 existing
  fill tests pass with identical expected values.
