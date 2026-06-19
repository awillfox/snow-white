# Discord Webhook Notifications Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Discord webhook notifications to snow-white so every buy/sell order (auto-trader and manual `order send`) pings Discord, plus a `notify` CLI command for sending arbitrary messages.

**Architecture:** A thin `internal/discord` package wraps a single POST call to a Discord incoming webhook; a `Notifier` interface in the `trader` package keeps the dependency direction clean (discord is an implementation detail, not a core dependency); all notification failures are non-fatal (log + continue) except the standalone `notify` command which fails loudly when the URL is missing.

**Tech Stack:** Go 1.25, `net/http` (stdlib), `net/http/httptest` (test stubs), cobra, viper/config.

## Global Constraints

- `go test ./...` must pass at every commit.
- Discord failures NEVER block or fail an order or trade — log + continue.
- Never log the webhook URL.
- Do NOT modify `pipeline.go`, `reconcile.go`, the guard, or money math.
- `context.Context` is always the first parameter; errors wrap with `%w`.
- Paper-mode orders also ping Discord (the message includes the mode so users know).

---

### Task 1: Config — add `DiscordWebhookURL` field

**Files:**
- Modify: `internal/config/config.go` (lines 11–22 struct; lines 40–46 BindEnv loop)
- Modify: `internal/config/config_test.go` (add one test case)

**Interfaces:**
- Produces: `cfg.DiscordWebhookURL string` — consumed by Tasks 4, 5, 6.

- [ ] **Step 1: Write the failing test**

Add this test to `internal/config/config_test.go` after the existing `TestLoadRiskCaps` function:

```go
func TestLoadDiscordWebhookURL(t *testing.T) {
	t.Setenv("INVX_APIKEY", "k")
	t.Setenv("INVX_SECRET", "s")
	t.Setenv("DISCORD_BOT_URL", "https://discord.com/api/webhooks/test/token")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DiscordWebhookURL != "https://discord.com/api/webhooks/test/token" {
		t.Fatalf("DiscordWebhookURL = %q", cfg.DiscordWebhookURL)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/nate/Dev/snow-white && go test ./internal/config/... -run TestLoadDiscordWebhookURL -v
```

Expected: FAIL — `cfg.DiscordWebhookURL` field does not exist.

- [ ] **Step 3: Add the field and BindEnv**

In `internal/config/config.go`:

Add the field to the struct after `KillFile`:
```go
KillFile           string        `mapstructure:"INVX_KILL_FILE"`
DiscordWebhookURL  string        `mapstructure:"DISCORD_BOT_URL"`
```

Add `"DISCORD_BOT_URL"` to the BindEnv loop:
```go
for _, k := range []string{
    "INVX_APIKEY", "INVX_SECRET", "INVX_HOST", "PSQL_URL",
    "INVX_SYMBOLS", "INVX_COLLECT_INTERVAL",
    "INVX_MAX_ORDER", "INVX_MAX_DAILY", "INVX_MAX_LOSS", "INVX_KILL_FILE",
    "DISCORD_BOT_URL",
} {
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /home/nate/Dev/snow-white && go test ./internal/config/... -v
```

Expected: All config tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/nate/Dev/snow-white && git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add DiscordWebhookURL field bound to DISCORD_BOT_URL"
```

---

### Task 2: `internal/discord` package

**Files:**
- Create: `internal/discord/discord.go`
- Create: `internal/discord/discord_test.go`

**Interfaces:**
- Produces: `discord.New(url string) *Client` and `(*Client).Send(ctx context.Context, content string) error`
- `*discord.Client` satisfies `trader.Notifier` (Task 3) — same Send signature.

- [ ] **Step 1: Write the failing tests**

Create `internal/discord/discord_test.go`:

```go
package discord_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"snow-white/internal/discord"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSend_PostsJSONAndSucceedsOn204: the client POSTs {"content":"hello"}
// to the webhook URL and returns nil on a 204 response.
func TestSend_PostsJSONAndSucceedsOn204(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		gotBody = body
		w.WriteHeader(http.StatusNoContent) // 204
	}))
	defer srv.Close()

	c := discord.New(srv.URL)
	err := c.Send(context.Background(), "hello")
	require.NoError(t, err)

	var payload map[string]string
	require.NoError(t, json.Unmarshal(gotBody, &payload))
	assert.Equal(t, "hello", payload["content"])
}

// TestSend_EmptyURL_NoOp: New("").Send makes no HTTP call and returns nil.
func TestSend_EmptyURL_NoOp(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	// Use empty URL — must NOT call the test server.
	c := discord.New("")
	err := c.Send(context.Background(), "anything")
	require.NoError(t, err)
	assert.False(t, called, "no HTTP call expected when URL is empty")
}

// TestSend_NonOK_ReturnsError: a 400 response causes Send to return a non-nil error.
func TestSend_NonOK_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest) // 400
	}))
	defer srv.Close()

	c := discord.New(srv.URL)
	err := c.Send(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "400")
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/nate/Dev/snow-white && go test ./internal/discord/... -v
```

Expected: FAIL — package `discord` does not exist.

- [ ] **Step 3: Implement `internal/discord/discord.go`**

```go
package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client sends messages to a Discord incoming webhook.
type Client struct {
	url string
	hc  *http.Client
}

// New creates a Client. If url is empty, Send is a no-op.
func New(url string) *Client {
	return &Client{
		url: url,
		hc:  &http.Client{Timeout: 10 * time.Second},
	}
}

// Send posts content as a Discord webhook message.
// If the URL is empty, Send is a no-op and returns nil.
// Treats any 2xx response (including 204 No Content) as success.
// Returns a non-nil error for non-2xx responses or transport failures.
func (c *Client) Send(ctx context.Context, content string) error {
	if c.url == "" {
		return nil
	}

	body, err := json.Marshal(map[string]string{"content": content})
	if err != nil {
		return fmt.Errorf("discord: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("discord: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("discord: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord: unexpected status %d", resp.StatusCode)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /home/nate/Dev/snow-white && go test ./internal/discord/... -v
```

Expected: All three discord tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/nate/Dev/snow-white && git add internal/discord/
git commit -m "feat(discord): add webhook client (no-op on empty URL, 2xx=ok)"
```

---

### Task 3: `Notifier` interface + `SetNotifier` on `Trader`

**Files:**
- Modify: `internal/trader/trader.go`
- Modify: `internal/trader/trader_test.go`

**Interfaces:**
- Consumes: none from prior tasks directly (discord.Client satisfies this interface, but trader.go must NOT import the discord package — dependency flows discord→trader, not the reverse).
- Produces: `type Notifier interface { Send(ctx context.Context, content string) error }` and `(*Trader).SetNotifier(n Notifier)`.

- [ ] **Step 1: Write the failing tests**

Add these tests to `internal/trader/trader_test.go`:

```go
// fakeNotifier records calls to Send.
type fakeNotifier struct {
	calls   []string
	sendErr error
}

func (f *fakeNotifier) Send(_ context.Context, content string) error {
	f.calls = append(f.calls, content)
	return f.sendErr
}

// TestTick_Notify_BuySuccess: successful Buy Place calls notify.Send once
// with a message containing the symbol and "BUY".
func TestTick_Notify_BuySuccess(t *testing.T) {
	cs := makeCandles(100_00, 200_00, 300_00)
	src := &fakeSource{candles: cs}
	strat := &stubStrategy{action: strategy.Buy, name: "stub"}
	pl := &fakePlacer{}
	pos := &fakePosReader{qty: 0}
	notif := &fakeNotifier{}

	tr := newTestTrader(src, strat, pl, pos)
	tr.SetNotifier(notif)

	require.NoError(t, tr.Tick(context.Background()))

	require.Len(t, notif.calls, 1, "expected exactly one notify call on successful Buy")
	assert.Contains(t, notif.calls[0], testSymbol)
	assert.Contains(t, notif.calls[0], "BUY")
}

// TestTick_Notify_SellSuccess: successful Sell Place calls notify.Send once
// with a message containing the symbol and "SELL".
func TestTick_Notify_SellSuccess(t *testing.T) {
	const holdingQty = int64(5_000_000_00)
	cs := makeCandles(100_00, 200_00, 400_00)
	src := &fakeSource{candles: cs}
	strat := &stubStrategy{action: strategy.Sell, name: "stub"}
	pl := &fakePlacer{}
	pos := &fakePosReader{qty: holdingQty}
	notif := &fakeNotifier{}

	tr := newTestTrader(src, strat, pl, pos)
	tr.SetNotifier(notif)

	require.NoError(t, tr.Tick(context.Background()))

	require.Len(t, notif.calls, 1, "expected exactly one notify call on successful Sell")
	assert.Contains(t, notif.calls[0], testSymbol)
	assert.Contains(t, notif.calls[0], "SELL")
}

// TestTick_Notify_PlaceBlocked_NoNotify: when Place returns an error (blocked),
// notify.Send must NOT be called.
func TestTick_Notify_PlaceBlocked_NoNotify(t *testing.T) {
	cs := makeCandles(100_00, 200_00, 300_00)
	src := &fakeSource{candles: cs}
	strat := &stubStrategy{action: strategy.Buy, name: "stub"}

	// Placer that always errors (simulates guard block).
	pl := &errorPlacer{err: fmt.Errorf("blocked by guard")}
	pos := &fakePosReader{qty: 0}
	notif := &fakeNotifier{}

	tr := newTestTrader(src, strat, pl, pos)
	tr.SetNotifier(notif)

	require.NoError(t, tr.Tick(context.Background()))

	assert.Empty(t, notif.calls, "no notify on blocked Place")
}

// TestTick_Notify_NilNotifier_NoPanic: nil notifier must not panic.
func TestTick_Notify_NilNotifier_NoPanic(t *testing.T) {
	cs := makeCandles(100_00, 200_00, 300_00)
	src := &fakeSource{candles: cs}
	strat := &stubStrategy{action: strategy.Buy, name: "stub"}
	pl := &fakePlacer{}
	pos := &fakePosReader{qty: 0}

	tr := newTestTrader(src, strat, pl, pos)
	// No SetNotifier call — notify is nil.

	assert.NotPanics(t, func() {
		require.NoError(t, tr.Tick(context.Background()))
	})
}

// TestTick_Notify_ErrorIsNonFatal: when notify.Send returns an error,
// Tick must still return nil (order already placed successfully).
func TestTick_Notify_ErrorIsNonFatal(t *testing.T) {
	cs := makeCandles(100_00, 200_00, 300_00)
	src := &fakeSource{candles: cs}
	strat := &stubStrategy{action: strategy.Buy, name: "stub"}
	pl := &fakePlacer{}
	pos := &fakePosReader{qty: 0}
	notif := &fakeNotifier{sendErr: fmt.Errorf("discord down")}

	tr := newTestTrader(src, strat, pl, pos)
	tr.SetNotifier(notif)

	err := tr.Tick(context.Background())
	require.NoError(t, err, "notify error must be non-fatal — Tick must return nil")
	assert.Len(t, pl.placed, 1, "order must still have been placed")
}
```

Also add the `errorPlacer` fake near the other fakes:

```go
type errorPlacer struct {
	err error
}

func (e *errorPlacer) Place(_ context.Context, _ Intent) (order.Order, error) {
	return order.Order{}, e.err
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/nate/Dev/snow-white && go test ./internal/trader/... -run "TestTick_Notify" -v
```

Expected: FAIL — `SetNotifier` and `Notifier` not defined.

- [ ] **Step 3: Add `Notifier` interface and `notify` field to Trader**

In `internal/trader/trader.go`:

Add `Notifier` interface after the existing `positionReader` interface:

```go
// Notifier sends a notification message. Satisfied by *discord.Client.
// Defined here to keep the trader package free of the discord import.
type Notifier interface {
	Send(ctx context.Context, content string) error
}
```

Add `notify Notifier` field to the `Trader` struct:

```go
type Trader struct {
	src        CandleSource
	strat      strategy.Strategy
	pipe       placer
	pos        positionReader
	symbol     string
	buyValue   int64
	interval   time.Duration
	now        func() time.Time
	reconcile  func(ctx context.Context) error
	notify     Notifier
}
```

Add `SetNotifier` method after `SetReconcile`:

```go
// SetNotifier registers a Notifier that Tick pings after every successful order placement.
// Pass nil to clear (no notifications). Notify errors are logged but never returned.
func (t *Trader) SetNotifier(n Notifier) {
	t.notify = n
}
```

Add a helper method that fires the notification (keeps Tick readable):

```go
// sendNotify sends a Discord notification after a successful Place.
// It logs and ignores any error so Discord failures never affect trading.
func (t *Trader) sendNotify(ctx context.Context, side, symbol, stratName, mode string, orderID int64) {
	if t.notify == nil {
		return
	}
	emoji := "🟢"
	sideLabel := "BUY"
	if side == "SELL" {
		emoji = "🔴"
		sideLabel = "SELL"
	}
	msg := fmt.Sprintf("%s %s %s via %s (order %d, %s)", emoji, sideLabel, symbol, stratName, orderID, mode)
	if err := t.notify.Send(ctx, msg); err != nil {
		log.Printf("trader: notify error (non-fatal): %v", err)
	}
}
```

Modify the `Tick` method to call `sendNotify` after successful Place. Replace the switch block:

```go
var placed order.Order
var placeErr error
switch sig.Action {
case strategy.Buy:
    if pos.Qty > 0 {
        return nil
    }
    placed, placeErr = t.pipe.Place(ctx, Intent{
        Symbol:   t.symbol,
        Side:     invx.Buy,
        RefPrice: last,
        ValueTHB: t.buyValue,
        Strategy: t.strat.Name(),
    })
    if placeErr == nil {
        t.sendNotify(ctx, "BUY", t.symbol, t.strat.Name(), placed.Mode, placed.ID)
    }
case strategy.Sell:
    if pos.Qty <= 0 {
        return nil
    }
    placed, placeErr = t.pipe.Place(ctx, Intent{
        Symbol:   t.symbol,
        Side:     invx.Sell,
        RefPrice: last,
        Quantity: pos.Qty,
        Strategy: t.strat.Name(),
    })
    if placeErr == nil {
        t.sendNotify(ctx, "SELL", t.symbol, t.strat.Name(), placed.Mode, placed.ID)
    }
}

if placeErr != nil {
    log.Printf("trader: place blocked/failed: %v", placeErr)
}
return nil
```

Note: `order.Order` has `ID int64` and `Mode string` fields — verify by checking `internal/order/order.go` before implementing (if they differ, adapt the sendNotify call accordingly).

- [ ] **Step 4: Verify `order.Order` struct fields**

```bash
cd /home/nate/Dev/snow-white && grep -n "type Order struct" -A 20 internal/order/order.go
```

Adapt `sendNotify` to use the actual field names if they differ from `ID` and `Mode`.

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd /home/nate/Dev/snow-white && go test ./internal/trader/... -v
```

Expected: All trader tests PASS.

- [ ] **Step 6: Commit**

```bash
cd /home/nate/Dev/snow-white && git add internal/trader/trader.go internal/trader/trader_test.go
git commit -m "feat(trader): add Notifier interface and SetNotifier; ping Discord after successful Place"
```

---

### Task 4: Notify on manual `order send`

**Files:**
- Modify: `internal/cli/order.go` (lines 192–198 — after `client.SendOrder`)

**Interfaces:**
- Consumes: `cfg.DiscordWebhookURL` (Task 1); `discord.New` + `discord.Client.Send` (Task 2).
- Produces: nothing new — extends existing manual order flow.

**Note:** No unit test is added here because `newOrderSendCmd` exercises real stdin and a live exchange client. The non-fatal behavior is verified by code inspection and the discord package tests.

- [ ] **Step 1: Add the discord notification after `client.SendOrder` in `order.go`**

Import `snow-white/internal/discord` at the top of the file. Then locate the successful `SendOrder` block (after `fmt.Printf("order placed: orderId=%d\n", orderID)`) and add:

```go
orderID, err := client.SendOrder(ctx, in)
if err != nil {
    return err
}
fmt.Printf("order placed: orderId=%d\n", orderID)

// Non-fatal Discord notification.
dc := discord.New(cfg.DiscordWebhookURL)
notifyMsg := fmt.Sprintf("📝 manual LIVE %s %s qty=%s price=%s (orderId=%d)",
    sideLabel,
    symbol,
    scale.Format(in.Quantity, 8),
    scale.Format(in.LimitPrice, 2),
    orderID,
)
if err := dc.Send(ctx, notifyMsg); err != nil {
    log.Printf("order: discord notify error (non-fatal): %v", err)
}
return nil
```

Also add `"log"` to the import block if not already present.

- [ ] **Step 2: Build to verify it compiles**

```bash
cd /home/nate/Dev/snow-white && go build ./...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd /home/nate/Dev/snow-white && git add internal/cli/order.go
git commit -m "feat(cli/order): ping Discord after successful live order send (non-fatal)"
```

---

### Task 5: `notify` CLI command

**Files:**
- Create: `internal/cli/notify.go`
- Modify: `internal/cli/root.go`

**Interfaces:**
- Consumes: `cfg.DiscordWebhookURL` (Task 1); `discord.New` + `discord.Client.Send` (Task 2).
- Produces: `newNotifyCmd() *cobra.Command` — consumed by root.go.

- [ ] **Step 1: Create `internal/cli/notify.go`**

```go
package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"snow-white/internal/config"
	"snow-white/internal/discord"
)

func newNotifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "notify <message...>",
		Short: "Send a message to Discord (DISCORD_BOT_URL must be set)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.DiscordWebhookURL == "" {
				return fmt.Errorf("DISCORD_BOT_URL not set")
			}
			message := strings.Join(args, " ")
			dc := discord.New(cfg.DiscordWebhookURL)
			if err := dc.Send(cmd.Context(), message); err != nil {
				return fmt.Errorf("discord notify: %w", err)
			}
			fmt.Println("sent")
			return nil
		},
	}
}
```

- [ ] **Step 2: Attach `newNotifyCmd` in `root.go`**

Add to `NewRootCmd()` in `internal/cli/root.go`:

```go
root.AddCommand(newNotifyCmd())
```

(Add it after `root.AddCommand(newResumeCmd())`.)

- [ ] **Step 3: Build and smoke-test the help output**

```bash
cd /home/nate/Dev/snow-white && go build -o /tmp/snow-white . && /tmp/snow-white notify --help
```

Expected output includes: `Send a message to Discord` and `Usage: snow-white notify <message...>`.

- [ ] **Step 4: Run full test suite**

```bash
cd /home/nate/Dev/snow-white && go test ./...
```

Expected: All tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/nate/Dev/snow-white && git add internal/cli/notify.go internal/cli/root.go
git commit -m "feat(cli): add notify command to send Discord messages via DISCORD_BOT_URL"
```

---

### Task 6: Wire notifier into the trade daemon

**Files:**
- Modify: `internal/cli/trade.go` (after `tr := trader.NewTrader(...)`)

**Interfaces:**
- Consumes: `cfg.DiscordWebhookURL` (Task 1); `discord.New` (Task 2); `(*Trader).SetNotifier` (Task 3).

- [ ] **Step 1: Add notifier wiring in `trade.go`**

Import `snow-white/internal/discord` at the top of the file.

After the `tr := trader.NewTrader(...)` line, add:

```go
tr.SetNotifier(discord.New(cfg.DiscordWebhookURL))
```

- [ ] **Step 2: Build to verify it compiles**

```bash
cd /home/nate/Dev/snow-white && go build ./...
```

Expected: no errors.

- [ ] **Step 3: Run full test suite**

```bash
cd /home/nate/Dev/snow-white && go test ./...
```

Expected: All tests PASS (discord.New("") is a no-op, so tests without a real URL are unaffected).

- [ ] **Step 4: Commit**

```bash
cd /home/nate/Dev/snow-white && git add internal/cli/trade.go
git commit -m "feat(cli/trade): wire Discord notifier into trade daemon via SetNotifier"
```

---

### Task 7: Final build, test, and report

**Files:**
- Create: `.superpowers/sdd/discord-report.md`

- [ ] **Step 1: Build the binary**

```bash
cd /home/nate/Dev/snow-white && go build -o snow-white .
```

Expected: `./snow-white` produced with no errors.

- [ ] **Step 2: Verify `notify --help`**

```bash
cd /home/nate/Dev/snow-white && ./snow-white notify --help
```

Expected: Shows `notify <message...>` usage and `DISCORD_BOT_URL must be set` description.

- [ ] **Step 3: Run full test suite**

```bash
cd /home/nate/Dev/snow-white && go test ./...
```

Expected: `ok` for all packages.

- [ ] **Step 4: Write the report**

Create `.superpowers/sdd/discord-report.md` with:
- Which packages were added/modified.
- The test names that cover each feature.
- The build + suite output (copy from steps above).
- Confirmation: Send no-op on empty URL (from `TestSend_EmptyURL_NoOp`).
- Confirmation: order-notify is non-fatal (log + continue; discord error never returned).
- Confirmation: `notify` cmd requires `DISCORD_BOT_URL` (fails loudly on empty).
- Confirmation: `notify --help` works.

- [ ] **Step 5: Final commit**

```bash
cd /home/nate/Dev/snow-white && git add snow-white .superpowers/sdd/discord-report.md
git commit -m "chore: build binary and write discord webhook feature report"
```
