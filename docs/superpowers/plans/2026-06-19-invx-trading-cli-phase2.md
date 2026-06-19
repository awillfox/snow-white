# InnovestX Trading CLI — Phase 2 (Trading) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add automated TA trading + a manual order CLI to `snow-white`, with paper-mode default and hard risk guards, on top of the Phase 1 data/analysis foundation.

**Architecture:** A separate `trade` daemon reads candles from Postgres, runs the Phase 1 `Strategy`, and routes Buy/Sell signals through a risk guard into an order pipeline. The pipeline runs in **paper mode by default** (logs intended orders, no API call); `--live` places real orders via new signed `invx` order methods. A manual `order` command group lets a human place/cancel/inspect orders. A kill switch (DB flag OR watched file) halts all trading instantly. Every multi-row write is one `pgx.Tx`. Money stays int64 satang; it is serialized to the wire as exact decimal `json.Number` (never float).

**Tech Stack:** Go 1.25, cobra, pgx/v5 + sqlc, go-money domain ints, the Phase 1 `invx`/`candle`/`strategy`/`indicator`/`scale`/`config` packages.

## Global Constraints

- Module `snow-white`, Go 1.25. `main.go` holds only `main()`.
- **Paper mode is the default.** Any order path places a real order ONLY when `--live` is explicitly passed. Absent `--live`, no order endpoint is called.
- Money is int64 **satang** (THB×100) in DB and domain; asset quantity is int64 **×1e8**. Float64 appears ONLY at display. To send decimals to the API, convert the int via `scale.Format` and emit as `encoding/json.Number` — never marshal a float64.
- The risk guard runs on EVERY order (paper and live) BEFORE the mode branch: per-order cap, daily-spend cap, daily-loss stop, kill switch. A blocked order is never sent.
- The API key is Read+Trading only (no Withdraw). The bot never calls deposit/withdraw endpoints.
- API enums: request uses ints (`side` 0=Buy/1=Sell, `orderType` 1=Market/2=Limit, `timeInForce` 1=GTC); responses return strings ("Buy"/"Sell", "Market"/"Limit", `orderState` "Working"/"Rejected"/"Canceled"/"Expired"/"FullyExecuted"/"Unknown"). Map both directions.
- `clientOrderId` (long int) sent to the exchange = our `orders.id` (bigserial). The exchange's returned `orderId` is stored in `orders.exchange_ref`.
- Idempotency: write a `pending` order row (status=pending) BEFORE the live API call; settle it after. A startup reconcile resolves any leftover `pending` rows against order history.
- Multi-row writes (order insert + position update + risk_state update) happen in ONE `pgx.Tx`.
- `context.Context` first param on I/O. Errors wrap with `%w`. Secrets never logged. `go test ./...` must pass before each commit.
- Never hand-edit `sqlc/`. Regenerate via `task sqlcgen`.

## API Contract (verified from api-docs.innovestxonline.com, 2026-06-19)

All under `/api/v1/digital-asset`, signed exactly as Phase 1 (exact-case X-INVX-* headers, HTTP/1.1 — already handled by `invx.Client`).

- **POST `/order/send`** (Trading). Body: `symbol` (str), `timeInForce` (int, 1=GTC), `side` (int 0/1), `orderType` (int 1/2), `limitPrice` (decimal), and one of `quantity` (decimal) or `value` (decimal THB), optional `clientOrderId` (long). Response: `data.orderId` (long).
- **POST `/order/cancel`** (Trading). Body: one of `clientOrderId` (long) or `orderId` (long). Response: `data.detail` on reject (e.g. code `1001`).
- **GET `/order/open/inquiry`** (Read,Trading). Response: `data[]` — `side`/`orderType`/`orderState` as STRINGS, `orderId`/`clientOrderId` (long), `price`/`quantity`/`origQuantity`/`quantityExecuted`/`avgPrice` (decimal strings), `symbol`, `receiveDateTime`.
- **POST `/order/history/inquiry`** (Read,Trading). Body: `symbol` (mandatory), optional `orderId`/`clientOrderId`/`startTimeStamp`/`endTimeStamp`/`depth`(default 200)/`startIndex`. Response: `data[]` like open orders.
- **GET `/account/balance/inquiry`** (Read,Trading). Response: `data[]` — `product` (e.g. "BTC"), `amount`/`hold` (decimal strings).
- Error codes: `0000` success; `1001` reject transaction; plus the Phase 1 auth codes.

## File Structure

```
internal/invx/
  client.go        # MODIFY: add a signed GET helper `get(ctx, path)`
  orders.go        # NEW: SendOrder/CancelOrder/OpenOrders/OrderHistory/AccountBalance + enum maps + decimal serialization
internal/config/config.go   # MODIFY: add MaxOrder/MaxDaily/MaxLoss (satang) + KillFile
internal/sql/orders.sql     # NEW: sqlc queries for orders/positions/risk_state
internal/order/
  order.go         # NEW: Order/Position/RiskState domain + enums + NewFromSQLC mappers
  store.go         # NEW: Store wrapping sqlc (InsertPending, MarkSettled, position + risk_state ops, reconcile query)
internal/trader/
  guard.go         # NEW: risk guard (pure logic over a RiskState + caps)
  killswitch.go    # NEW: kill-file + DB-halt check
  pipeline.go      # NEW: order pipeline (paper/live, idempotency, one tx)
  trader.go        # NEW: trade daemon loop (candles -> strategy -> guard -> pipeline)
internal/cli/
  trade.go         # NEW: `trade` daemon command
  order.go         # NEW: `order` manual command group + `balance` + `status`
  risk.go          # NEW: `kill` / `resume` commands
  root.go          # MODIFY: attach the new commands
```

---

### Task 1: Config additions + invx signed GET + decimal wire helper

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/invx/client.go`
- Create: `internal/invx/wire.go`
- Test: `internal/config/config_test.go` (add cases), `internal/invx/wire_test.go` (add cases — file exists)

**Interfaces:**
- Consumes: `pkg/scale.Format` (Phase 1).
- Produces:
  - `config.Config` gains `MaxOrder, MaxDaily, MaxLoss int64` (satang) and `KillFile string`.
  - `func (c *Client) get(ctx context.Context, path string) ([]byte, error)` — signed GET, empty body.
  - `func decimalNumber(scaledInt int64, decimals int) json.Number` (package invx) — `scale.Format` → `json.Number`, marshals as a bare number (no quotes, no float).

- [ ] **Step 1: Write failing config test**

Add to `internal/config/config_test.go`:
```go
func TestLoadRiskCaps(t *testing.T) {
	t.Setenv("INVX_APIKEY", "k")
	t.Setenv("INVX_SECRET", "s")
	t.Setenv("INVX_MAX_ORDER", "500000")  // satang
	t.Setenv("INVX_MAX_DAILY", "5000000")
	t.Setenv("INVX_MAX_LOSS", "1000000")
	t.Setenv("INVX_KILL_FILE", "./.halt")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxOrder != 500000 || cfg.MaxDaily != 5000000 || cfg.MaxLoss != 1000000 {
		t.Fatalf("caps not loaded: %+v", cfg)
	}
	if cfg.KillFile != "./.halt" {
		t.Fatalf("kill file = %q", cfg.KillFile)
	}
}
```

- [ ] **Step 2: Run test → fail**

Run: `go test ./internal/config/ -run TestLoadRiskCaps -v`
Expected: FAIL — `cfg.MaxOrder undefined`.

- [ ] **Step 3: Add fields + bindings to config**

In `internal/config/config.go`, add to the `Config` struct:
```go
	MaxOrder int64  `mapstructure:"INVX_MAX_ORDER"`
	MaxDaily int64  `mapstructure:"INVX_MAX_DAILY"`
	MaxLoss  int64  `mapstructure:"INVX_MAX_LOSS"`
	KillFile string `mapstructure:"INVX_KILL_FILE"`
```
And add these keys to the `BindEnv` loop:
```go
		"INVX_MAX_ORDER", "INVX_MAX_DAILY", "INVX_MAX_LOSS", "INVX_KILL_FILE",
```

- [ ] **Step 4: Run config test → pass**

Run: `go test ./internal/config/ -run TestLoadRiskCaps -v`
Expected: PASS.

- [ ] **Step 5: Write failing decimal-wire test**

Add to `internal/invx/wire_test.go`:
```go
func TestDecimalNumber(t *testing.T) {
	// 7000 satang -> "70.00" ; 10000000 (x1e8) -> "0.10000000"
	if got := decimalNumber(7000, 2); string(got) != "70.00" {
		t.Fatalf("price = %q, want 70.00", got)
	}
	if got := decimalNumber(10000000, 8); string(got) != "0.10000000" {
		t.Fatalf("qty = %q, want 0.10000000", got)
	}
	// Marshals as a bare JSON number, not a quoted string.
	b, err := json.Marshal(map[string]json.Number{"limitPrice": decimalNumber(7000, 2)})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"limitPrice":70.00}` {
		t.Fatalf("marshal = %s", b)
	}
}
```
Add imports `encoding/json` and `testing` if not present (they are in wire_test.go; add `encoding/json`).

- [ ] **Step 6: Run test → fail**

Run: `go test ./internal/invx/ -run TestDecimalNumber -v`
Expected: FAIL — `undefined: decimalNumber`.

- [ ] **Step 7: Implement wire helper + signed GET**

Create `internal/invx/wire.go`:
```go
package invx

import (
	"encoding/json"

	"snow-white/pkg/scale"
)

// decimalNumber renders a scaled int64 as an exact-decimal json.Number, so it
// marshals as a bare number (e.g. 70.00, 0.10000000) with no float64 rounding.
func decimalNumber(scaledInt int64, decimals int) json.Number {
	return json.Number(scale.Format(scaledInt, decimals))
}
```

In `internal/invx/client.go`, add a signed GET method (the canonical string uses an empty body for GET):
```go
// get signs and sends a GET to basePath+path (empty body in the signature).
func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	fullPath := basePath + path
	uid := uuid.NewString()
	ts := strconv.FormatInt(c.now().UnixMilli(), 10)

	sts := buildStringToSign(c.apikey, "GET", c.host, fullPath, "", contentType, uid, ts, "")
	signature := sign(c.secret, sts)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+fullPath, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header["Content-Type"] = []string{contentType}
	req.Header["X-INVX-APIKEY"] = []string{c.apikey}
	req.Header["X-INVX-SIGNATURE"] = []string{signature}
	req.Header["X-INVX-REQUEST-UID"] = []string{uid}
	req.Header["X-INVX-TIMESTAMP"] = []string{ts}

	res, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do GET %s: %w", path, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("read GET %s: %w", path, err)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d for %s: %s", res.StatusCode, path, string(raw))
	}
	return raw, nil
}
```

- [ ] **Step 8: Run tests → pass**

Run: `go test ./internal/invx/ ./internal/config/ -v 2>&1 | tail -20`
Expected: all PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/config/ internal/invx/
git commit -m "feat(phase2): config risk caps + invx signed GET + decimal wire helper"
```

---

### Task 2: invx order methods (send/cancel/open/history/balance)

**Files:**
- Create: `internal/invx/orders.go`
- Test: `internal/invx/orders_test.go`

**Interfaces:**
- Consumes: `decimalNumber` (Task 1), `c.post`/`c.get` (Phase 1 + Task 1).
- Produces (package invx):
  - `type Side int` (`Buy Side = 0`, `Sell Side = 1`); `type OrderType int` (`Market = 1`, `Limit = 2`).
  - `type SendOrderInput struct { Symbol string; Side Side; Type OrderType; LimitPrice int64; Quantity int64; Value int64; ClientOrderID int64 }` (LimitPrice satang; Quantity ×1e8; Value satang; set Quantity XOR Value).
  - `func (c *Client) SendOrder(ctx context.Context, in SendOrderInput) (int64, error)` — returns exchange `orderId`.
  - `func (c *Client) CancelOrder(ctx context.Context, clientOrderID, orderID int64) error` (pass one; 0 means omit).
  - `type OrderInfo struct { OrderID, ClientOrderID int64; Symbol string; Side Side; Type OrderType; State string; Price, OrigQuantity, QuantityExecuted, AvgPrice int64; ReceiveDateTime time.Time }` (prices satang, quantities ×1e8).
  - `func (c *Client) OpenOrders(ctx context.Context) ([]OrderInfo, error)`
  - `func (c *Client) OrderHistory(ctx context.Context, symbol string, depth int) ([]OrderInfo, error)`
  - `type Balance struct { Product string; Amount, Hold int64 }` (×1e8)
  - `func (c *Client) AccountBalance(ctx context.Context) ([]Balance, error)`

- [ ] **Step 1: Write the failing tests (httptest stubs)**

`internal/invx/orders_test.go`:
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

func stubClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "http://")
	c := New("pub", "sec", host, srv.Client())
	c.baseURL = srv.URL
	return c
}

func TestSendOrderSerializesDecimalsAndReturnsOrderID(t *testing.T) {
	var body map[string]json.RawMessage
	c := stubClient(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		_, _ = w.Write([]byte(`{"code":"0000","message":"SUCCESS","data":{"orderId":12345}}`))
	})
	id, err := c.SendOrder(context.Background(), SendOrderInput{
		Symbol: "BTCTHB", Side: Buy, Type: Limit,
		LimitPrice: 7000, Value: 500000, ClientOrderID: 42,
	})
	require.NoError(t, err)
	require.Equal(t, int64(12345), id)
	// decimals serialized as bare numbers, side/orderType/timeInForce as ints
	require.JSONEq(t, `70.00`, string(body["limitPrice"]))
	require.JSONEq(t, `5000.00`, string(body["value"]))
	require.JSONEq(t, `0`, string(body["side"]))
	require.JSONEq(t, `2`, string(body["orderType"]))
	require.JSONEq(t, `1`, string(body["timeInForce"]))
	require.JSONEq(t, `42`, string(body["clientOrderId"]))
}

func TestSendOrderAPIError(t *testing.T) {
	c := stubClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"1001","message":"Reject transaction"}`))
	})
	_, err := c.SendOrder(context.Background(), SendOrderInput{Symbol: "BTCTHB", Side: Buy, Type: Market, Value: 100})
	require.Error(t, err)
	require.Contains(t, err.Error(), "1001")
}

func TestOpenOrdersParsesStringEnums(t *testing.T) {
	c := stubClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"0000","message":"SUCCESS","data":[{
			"side":"Buy","orderId":1,"price":"1000.00000000","quantity":"0.01000000",
			"symbol":"BTCTHB","orderType":"Limit","clientOrderId":42,"orderState":"Working",
			"receiveDateTime":"2023-05-03T00:00:00.646Z","origQuantity":"0.01000000",
			"quantityExecuted":"0.00000000","avgPrice":"0.00000000"}]}`))
	})
	got, err := c.OpenOrders(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, Buy, got[0].Side)
	require.Equal(t, Limit, got[0].Type)
	require.Equal(t, "Working", got[0].State)
	require.Equal(t, int64(100000), got[0].Price)       // 1000.00 -> satang
	require.Equal(t, int64(1000000), got[0].OrigQuantity) // 0.01 -> x1e8
	require.Equal(t, int64(42), got[0].ClientOrderID)
}

func TestAccountBalanceParses(t *testing.T) {
	c := stubClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"0000","message":"SUCCESS","data":[{"product":"BTC","amount":"1.50000000","hold":"0.25000000"}]}`))
	})
	got, err := c.AccountBalance(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "BTC", got[0].Product)
	require.Equal(t, int64(150000000), got[0].Amount) // 1.5 -> x1e8
	require.Equal(t, int64(25000000), got[0].Hold)
}
```

- [ ] **Step 2: Run tests → fail**

Run: `go test ./internal/invx/ -run 'TestSendOrder|TestOpenOrders|TestAccountBalance' -v`
Expected: FAIL — `undefined: SendOrderInput` etc.

- [ ] **Step 3: Implement order methods**

`internal/invx/orders.go`:
```go
package invx

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"snow-white/pkg/scale"
)

type Side int

const (
	Buy  Side = 0
	Sell Side = 1
)

type OrderType int

const (
	Market OrderType = 1
	Limit  OrderType = 2
)

const timeInForceGTC = 1

type SendOrderInput struct {
	Symbol        string
	Side          Side
	Type          OrderType
	LimitPrice    int64 // satang
	Quantity      int64 // x1e8 (set Quantity XOR Value)
	Value         int64 // satang THB (set Quantity XOR Value)
	ClientOrderID int64
}

func (c *Client) SendOrder(ctx context.Context, in SendOrderInput) (int64, error) {
	body := map[string]any{
		"symbol":        in.Symbol,
		"timeInForce":   timeInForceGTC,
		"side":          int(in.Side),
		"orderType":     int(in.Type),
		"limitPrice":    decimalNumber(in.LimitPrice, 2),
		"clientOrderId": in.ClientOrderID,
	}
	if in.Quantity > 0 {
		body["quantity"] = decimalNumber(in.Quantity, 8)
	}
	if in.Value > 0 {
		body["value"] = decimalNumber(in.Value, 2)
	}
	raw, err := c.postJSON(ctx, "/order/send", body)
	if err != nil {
		return 0, err
	}
	var resp struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Data    struct {
			OrderID int64 `json:"orderId"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return 0, fmt.Errorf("decode send order: %w", err)
	}
	if resp.Code != "0000" {
		return 0, fmt.Errorf("send order: api error %s: %s", resp.Code, resp.Message)
	}
	return resp.Data.OrderID, nil
}

func (c *Client) CancelOrder(ctx context.Context, clientOrderID, orderID int64) error {
	body := map[string]any{}
	if clientOrderID > 0 {
		body["clientOrderId"] = clientOrderID
	}
	if orderID > 0 {
		body["orderId"] = orderID
	}
	raw, err := c.postJSON(ctx, "/order/cancel", body)
	if err != nil {
		return err
	}
	var resp struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fmt.Errorf("decode cancel: %w", err)
	}
	if resp.Code != "0000" {
		return fmt.Errorf("cancel order: api error %s: %s", resp.Code, resp.Message)
	}
	return nil
}

type OrderInfo struct {
	OrderID          int64
	ClientOrderID    int64
	Symbol           string
	Side             Side
	Type             OrderType
	State            string
	Price            int64 // satang
	OrigQuantity     int64 // x1e8
	QuantityExecuted int64 // x1e8
	AvgPrice         int64 // satang
	ReceiveDateTime  time.Time
}

type orderRaw struct {
	Side             string `json:"side"`
	OrderID          int64  `json:"orderId"`
	ClientOrderID    int64  `json:"clientOrderId"`
	Symbol           string `json:"symbol"`
	OrderType        string `json:"orderType"`
	OrderState       string `json:"orderState"`
	Price            string `json:"price"`
	OrigQuantity     string `json:"origQuantity"`
	Quantity         string `json:"quantity"`
	QuantityExecuted string `json:"quantityExecuted"`
	AvgPrice         string `json:"avgPrice"`
	ReceiveDateTime  string `json:"receiveDateTime"`
}

func (r orderRaw) toInfo() (OrderInfo, error) {
	price, err := scale.Parse(zeroIfEmpty(r.Price), 2)
	if err != nil {
		return OrderInfo{}, err
	}
	// origQuantity may be absent on open orders that use "quantity"; fall back.
	origQty := r.OrigQuantity
	if origQty == "" {
		origQty = r.Quantity
	}
	oq, err := scale.Parse(zeroIfEmpty(origQty), 8)
	if err != nil {
		return OrderInfo{}, err
	}
	qe, err := scale.Parse(zeroIfEmpty(r.QuantityExecuted), 8)
	if err != nil {
		return OrderInfo{}, err
	}
	ap, err := scale.Parse(zeroIfEmpty(r.AvgPrice), 2)
	if err != nil {
		return OrderInfo{}, err
	}
	var dt time.Time
	if r.ReceiveDateTime != "" {
		dt, _ = time.Parse(time.RFC3339Nano, r.ReceiveDateTime)
	}
	return OrderInfo{
		OrderID: r.OrderID, ClientOrderID: r.ClientOrderID, Symbol: r.Symbol,
		Side: sideFromString(r.Side), Type: typeFromString(r.OrderType), State: r.OrderState,
		Price: price, OrigQuantity: oq, QuantityExecuted: qe, AvgPrice: ap, ReceiveDateTime: dt,
	}, nil
}

func zeroIfEmpty(s string) string {
	if s == "" {
		return "0"
	}
	return s
}

func sideFromString(s string) Side {
	if s == "Sell" || s == "1" {
		return Sell
	}
	return Buy
}

func typeFromString(s string) OrderType {
	if s == "Market" || s == "1" {
		return Market
	}
	return Limit
}

func (c *Client) OpenOrders(ctx context.Context) ([]OrderInfo, error) {
	raw, err := c.get(ctx, "/order/open/inquiry")
	if err != nil {
		return nil, err
	}
	return parseOrderList(raw, "open orders")
}

func (c *Client) OrderHistory(ctx context.Context, symbol string, depth int) ([]OrderInfo, error) {
	if depth <= 0 {
		depth = 200
	}
	raw, err := c.postJSON(ctx, "/order/history/inquiry", map[string]any{"symbol": symbol, "depth": depth})
	if err != nil {
		return nil, err
	}
	return parseOrderList(raw, "order history")
}

func parseOrderList(raw []byte, what string) ([]OrderInfo, error) {
	var resp struct {
		Code    string     `json:"code"`
		Message string     `json:"message"`
		Data    []orderRaw `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode %s: %w", what, err)
	}
	if resp.Code != "0000" {
		return nil, fmt.Errorf("%s: api error %s: %s", what, resp.Code, resp.Message)
	}
	out := make([]OrderInfo, 0, len(resp.Data))
	for _, r := range resp.Data {
		oi, err := r.toInfo()
		if err != nil {
			return nil, fmt.Errorf("parse %s row: %w", what, err)
		}
		out = append(out, oi)
	}
	return out, nil
}

type Balance struct {
	Product string
	Amount  int64 // x1e8
	Hold    int64 // x1e8
}

func (c *Client) AccountBalance(ctx context.Context) ([]Balance, error) {
	raw, err := c.get(ctx, "/account/balance/inquiry")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Data    []struct {
			Product string `json:"product"`
			Amount  string `json:"amount"`
			Hold    string `json:"hold"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode balance: %w", err)
	}
	if resp.Code != "0000" {
		return nil, fmt.Errorf("balance: api error %s: %s", resp.Code, resp.Message)
	}
	out := make([]Balance, 0, len(resp.Data))
	for _, d := range resp.Data {
		amt, err := scale.Parse(zeroIfEmpty(d.Amount), 8)
		if err != nil {
			return nil, err
		}
		hold, err := scale.Parse(zeroIfEmpty(d.Hold), 8)
		if err != nil {
			return nil, err
		}
		out = append(out, Balance{Product: d.Product, Amount: amt, Hold: hold})
	}
	return out, nil
}
```

- [ ] **Step 4: Refactor `post` to expose `postJSON` (marshal-once)**

The existing `post(ctx, path, body []byte)` marshals nothing — callers pass bytes. Add a sibling that marshals a value once and reuses `post` so body-signed == body-sent. In `internal/invx/client.go`, add:
```go
// postJSON marshals v once and posts it (the exact bytes are signed and sent).
func (c *Client) postJSON(ctx context.Context, path string, v any) ([]byte, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal %s body: %w", path, err)
	}
	return c.post(ctx, path, body)
}
```
(The `Ticker` method may be refactored to use `postJSON` too, but that is optional — leave it if it complicates the diff.)

- [ ] **Step 5: Run tests → pass**

Run: `go test ./internal/invx/ -v 2>&1 | tail -25`
Expected: all PASS (new order tests + existing signing/ticker/wire tests).

- [ ] **Step 6: Commit**

```bash
git add internal/invx/
git commit -m "feat(phase2): invx order + balance methods with decimal serialization and enum mapping"
```

---

### Task 3: sqlc queries for orders/positions/risk_state

**Files:**
- Create: `internal/sql/orders.sql`
- Regenerate: `sqlc/` (`task sqlcgen`)

**Interfaces:**
- Produces (generated `sqlc.Queries`): `InsertOrder`, `SettleOrder`, `ListPendingOrders`, `GetPosition`, `UpsertPosition`, `GetRiskState`, `InsertRiskState`, `UpdateRiskSpentLoss`, `SetRiskHalted`. Exact field names from sqlc; Task 4 maps them.

- [ ] **Step 1: Write the queries**

`internal/sql/orders.sql`:
```sql
-- name: InsertOrder :one
INSERT INTO orders (client_uid, symbol, side, type, limit_price, quantity, mode, strategy, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'pending')
RETURNING *;

-- name: SettleOrder :exec
UPDATE orders
SET status = $2, exchange_ref = $3, reason = $4
WHERE id = $1;

-- name: ListPendingOrders :many
SELECT * FROM orders WHERE status = 'pending' AND mode = 'live' ORDER BY id ASC;

-- name: GetPosition :one
SELECT * FROM positions WHERE symbol = $1;

-- name: UpsertPosition :exec
INSERT INTO positions (symbol, qty, avg_cost, realized_pnl, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (symbol) DO UPDATE SET
    qty = EXCLUDED.qty,
    avg_cost = EXCLUDED.avg_cost,
    realized_pnl = EXCLUDED.realized_pnl,
    updated_at = now();

-- name: GetRiskState :one
SELECT * FROM risk_state WHERE day = $1;

-- name: InsertRiskState :one
INSERT INTO risk_state (day) VALUES ($1)
ON CONFLICT (day) DO UPDATE SET updated_at = now()
RETURNING *;

-- name: UpdateRiskSpentLoss :exec
UPDATE risk_state
SET spent_today = spent_today + $2, loss_today = loss_today + $3, updated_at = now()
WHERE day = $1;

-- name: SetRiskHalted :exec
UPDATE risk_state SET halted = $2, halt_reason = $3, updated_at = now() WHERE day = $1;
```

- [ ] **Step 2: Generate**

Run: `task sqlcgen`
Expected: `sqlc/orders.sql.go` created; no errors.

- [ ] **Step 3: Verify it compiles**

Run: `go build ./sqlc/...`
Expected: clean.

- [ ] **Step 4: Record generated names**

Read `sqlc/orders.sql.go` and note the exact `InsertOrderParams` field names and the pgtype used for `limit_price` (nullable bigint → `pgtype.Int8`) and `strategy` (nullable text → `pgtype.Text`). Task 4 must match these.

- [ ] **Step 5: Commit**

```bash
git add internal/sql/orders.sql sqlc/
git commit -m "feat(phase2): sqlc queries for orders/positions/risk_state"
```

---

### Task 4: order domain + store (integration-tested tx)

**Files:**
- Create: `internal/order/order.go`
- Create: `internal/order/store.go`
- Test: `internal/order/store_test.go`

**Interfaces:**
- Consumes: generated sqlc (Task 3), `pgxpool.Pool`.
- Produces:
  - `type Mode string` (`Paper Mode = "paper"`, `Live Mode = "live"`); `type Status string` (`Pending="pending"`, `Accepted="accepted"`, `Rejected="rejected"`).
  - `type Order struct { ID int64; ClientUID string; Symbol, Side, Type string; LimitPrice int64; Quantity int64; Mode, Strategy, Status, ExchangeRef, Reason string; CreatedAt time.Time }` + `NewFromSQLC`.
  - `type Position struct { Symbol string; Qty, AvgCost, RealizedPnl int64 }`.
  - `type RiskState struct { Day time.Time; Halted bool; HaltReason string; SpentToday, LossToday int64 }`.
  - `type Store struct{ ... }`; `func NewStore(pool *pgxpool.Pool) *Store`.
  - `func (s *Store) InsertPending(ctx, in InsertPendingInput) (Order, error)` where `InsertPendingInput struct { ClientUID, Symbol, Side, Type string; LimitPrice int64; Quantity int64; Mode, Strategy string }`.
  - `func (s *Store) Settle(ctx, id int64, status Status, exchangeRef, reason string) error`.
  - `func (s *Store) RiskToday(ctx, day time.Time) (RiskState, error)` (creates the row if absent).
  - `func (s *Store) ApplyFill(ctx, in FillInput) error` — ONE tx: settle order accepted + upsert position + update risk spent/loss. `FillInput struct { OrderID int64; Symbol string; Day time.Time; NewQty, NewAvgCost, NewRealizedPnl, SpentDelta, LossDelta int64; ExchangeRef string }`.
  - `func (s *Store) SetHalted(ctx, day time.Time, halted bool, reason string) error`.
  - `func (s *Store) ListPendingLive(ctx) ([]Order, error)`.

- [ ] **Step 1: Write domain + mappers** (`internal/order/order.go`)

```go
package order

import (
	"time"

	"snow-white/sqlc"
)

type Mode string
type Status string

const (
	Paper Mode = "paper"
	Live  Mode = "live"

	Pending  Status = "pending"
	Accepted Status = "accepted"
	Rejected Status = "rejected"
)

type Order struct {
	ID          int64     `json:"id"`
	ClientUID   string    `json:"client_uid"`
	Symbol      string    `json:"symbol"`
	Side        string    `json:"side"`
	Type        string    `json:"type"`
	LimitPrice  int64     `json:"limit_price"`
	Quantity    int64     `json:"quantity"`
	Mode        string    `json:"mode"`
	Strategy    string    `json:"strategy"`
	Status      string    `json:"status"`
	ExchangeRef string    `json:"exchange_ref"`
	Reason      string    `json:"reason"`
	CreatedAt   time.Time `json:"created_at"`
}

func NewFromSQLC(o sqlc.Order) Order {
	return Order{
		ID:          o.ID,
		ClientUID:   o.ClientUid.String(), // pgtype.UUID -> string
		Symbol:      o.Symbol,
		Side:        o.Side,
		Type:        o.Type,
		LimitPrice:  o.LimitPrice.Int64, // pgtype.Int8 nullable
		Quantity:    o.Quantity,
		Mode:        o.Mode,
		Strategy:    o.Strategy.String, // pgtype.Text
		Status:      o.Status,
		ExchangeRef: o.ExchangeRef.String,
		Reason:      o.Reason.String,
		CreatedAt:   o.CreatedAt.Time,
	}
}

type Position struct {
	Symbol      string `json:"symbol"`
	Qty         int64  `json:"qty"`
	AvgCost     int64  `json:"avg_cost"`
	RealizedPnl int64  `json:"realized_pnl"`
}

type RiskState struct {
	Day        time.Time `json:"day"`
	Halted     bool      `json:"halted"`
	HaltReason string    `json:"halt_reason"`
	SpentToday int64     `json:"spent_today"`
	LossToday  int64     `json:"loss_today"`
}
```
> NOTE: align `.String()`/`.Int64`/`.String`/`.Time` unwrapping to the ACTUAL pgtype the generator emitted (read `sqlc/models.go`). `client_uid` is `uuid` → likely `pgtype.UUID` (use its `.String()` helper or format the bytes); if the generated type differs, adjust and note it.

- [ ] **Step 2: Write the store** (`internal/order/store.go`)

```go
package order

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"snow-white/sqlc"
)

type Store struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, q: sqlc.New(pool)}
}

type InsertPendingInput struct {
	ClientUID  string
	Symbol     string
	Side       string
	Type       string
	LimitPrice int64
	Quantity   int64
	Mode       string
	Strategy   string
}

func (s *Store) InsertPending(ctx context.Context, in InsertPendingInput) (Order, error) {
	var uid pgtype.UUID
	if err := uid.Scan(in.ClientUID); err != nil {
		return Order{}, fmt.Errorf("parse client uid: %w", err)
	}
	row, err := s.q.InsertOrder(ctx, sqlc.InsertOrderParams{
		ClientUid:  uid,
		Symbol:     in.Symbol,
		Side:       in.Side,
		Type:       in.Type,
		LimitPrice: pgtype.Int8{Int64: in.LimitPrice, Valid: in.LimitPrice != 0},
		Quantity:   in.Quantity,
		Mode:       in.Mode,
		Strategy:   pgtype.Text{String: in.Strategy, Valid: in.Strategy != ""},
	})
	if err != nil {
		return Order{}, fmt.Errorf("insert pending order: %w", err)
	}
	return NewFromSQLC(row), nil
}

func (s *Store) Settle(ctx context.Context, id int64, status Status, exchangeRef, reason string) error {
	err := s.q.SettleOrder(ctx, sqlc.SettleOrderParams{
		ID:          id,
		Status:      string(status),
		ExchangeRef: pgtype.Text{String: exchangeRef, Valid: exchangeRef != ""},
		Reason:      pgtype.Text{String: reason, Valid: reason != ""},
	})
	if err != nil {
		return fmt.Errorf("settle order %d: %w", id, err)
	}
	return nil
}

func dayDate(day time.Time) pgtype.Date {
	return pgtype.Date{Time: time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
}

func (s *Store) RiskToday(ctx context.Context, day time.Time) (RiskState, error) {
	row, err := s.q.InsertRiskState(ctx, dayDate(day))
	if err != nil {
		return RiskState{}, fmt.Errorf("get/create risk_state: %w", err)
	}
	return RiskState{
		Day:        row.Day.Time,
		Halted:     row.Halted,
		HaltReason: row.HaltReason.String,
		SpentToday: row.SpentToday,
		LossToday:  row.LossToday,
	}, nil
}

type FillInput struct {
	OrderID        int64
	Symbol         string
	Day            time.Time
	NewQty         int64
	NewAvgCost     int64
	NewRealizedPnl int64
	SpentDelta     int64
	LossDelta      int64
	ExchangeRef    string
}

// ApplyFill settles the order accepted, upserts the position, and bumps the
// day's spent/loss counters — all in one transaction.
func (s *Store) ApplyFill(ctx context.Context, in FillInput) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	if err := q.SettleOrder(ctx, sqlc.SettleOrderParams{
		ID:          in.OrderID,
		Status:      string(Accepted),
		ExchangeRef: pgtype.Text{String: in.ExchangeRef, Valid: in.ExchangeRef != ""},
	}); err != nil {
		return fmt.Errorf("settle in tx: %w", err)
	}
	if err := q.UpsertPosition(ctx, sqlc.UpsertPositionParams{
		Symbol:      in.Symbol,
		Qty:         in.NewQty,
		AvgCost:     in.NewAvgCost,
		RealizedPnl: in.NewRealizedPnl,
	}); err != nil {
		return fmt.Errorf("upsert position in tx: %w", err)
	}
	if err := q.UpdateRiskSpentLoss(ctx, sqlc.UpdateRiskSpentLossParams{
		Day:        dayDate(in.Day),
		SpentToday: in.SpentDelta,
		LossToday:  in.LossDelta,
	}); err != nil {
		return fmt.Errorf("update risk in tx: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit fill: %w", err)
	}
	return nil
}

func (s *Store) SetHalted(ctx context.Context, day time.Time, halted bool, reason string) error {
	if _, err := s.q.InsertRiskState(ctx, dayDate(day)); err != nil {
		return fmt.Errorf("ensure risk row: %w", err)
	}
	err := s.q.SetRiskHalted(ctx, sqlc.SetRiskHaltedParams{
		Day:        dayDate(day),
		Halted:     halted,
		HaltReason: pgtype.Text{String: reason, Valid: reason != ""},
	})
	if err != nil {
		return fmt.Errorf("set halted: %w", err)
	}
	return nil
}

func (s *Store) GetPosition(ctx context.Context, symbol string) (Position, error) {
	row, err := s.q.GetPosition(ctx, symbol)
	if err == pgx.ErrNoRows {
		return Position{Symbol: symbol}, nil
	}
	if err != nil {
		return Position{}, fmt.Errorf("get position: %w", err)
	}
	return Position{Symbol: row.Symbol, Qty: row.Qty, AvgCost: row.AvgCost, RealizedPnl: row.RealizedPnl}, nil
}

func (s *Store) ListPendingLive(ctx context.Context) ([]Order, error) {
	rows, err := s.q.ListPendingOrders(ctx)
	if err != nil {
		return nil, fmt.Errorf("list pending: %w", err)
	}
	out := make([]Order, 0, len(rows))
	for _, r := range rows {
		out = append(out, NewFromSQLC(r))
	}
	return out, nil
}
```
> NOTE: `InsertOrderParams`/`SettleOrderParams`/etc. field names and pgtypes MUST match the generated code from Task 3. Read `sqlc/orders.sql.go` and align. `pgtype.UUID.Scan(string)` accepts a hyphenated uuid.

- [ ] **Step 3: Write integration test (one-tx fill)** (`internal/order/store_test.go`)

Use a testcontainers Postgres (mirror Phase 1 `candle/store_test.go` setup), creating the `orders`, `positions`, `risk_state` tables with the same DDL as `schema.hcl`. Then:
```go
func TestApplyFillIsAtomic(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t) // creates orders/positions/risk_state tables
	store := NewStore(pool)
	day := time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC)

	o, err := store.InsertPending(ctx, InsertPendingInput{
		ClientUID: "019d1bae-e2f1-42d9-b9e8-23d495dbe9f9",
		Symbol:    "BTCTHB", Side: "BUY", Type: "LIMIT",
		LimitPrice: 100000, Quantity: 1000000, Mode: "live", Strategy: "macross(5,20)",
	})
	require.NoError(t, err)
	require.Equal(t, "pending", o.Status)

	_, err = store.RiskToday(ctx, day)
	require.NoError(t, err)

	require.NoError(t, store.ApplyFill(ctx, FillInput{
		OrderID: o.ID, Symbol: "BTCTHB", Day: day,
		NewQty: 1000000, NewAvgCost: 100000, NewRealizedPnl: 0,
		SpentDelta: 1000, LossDelta: 0, ExchangeRef: "55",
	}))

	pos, err := store.GetPosition(ctx, "BTCTHB")
	require.NoError(t, err)
	require.Equal(t, int64(1000000), pos.Qty)

	risk, err := store.RiskToday(ctx, day)
	require.NoError(t, err)
	require.Equal(t, int64(1000), risk.SpentToday)
}
```
(Write the `newTestPool` helper with the orders/positions/risk_state DDL matching `schema.hcl`. Keep it in this test file.)

- [ ] **Step 4: Run → align generated names → pass**

Run: `go test ./internal/order/ -v`
Expected: PASS (fix any generated-name mismatch surfaced).

- [ ] **Step 5: Commit**

```bash
git add internal/order/
git commit -m "feat(phase2): order/position/risk_state domain + store with atomic fill tx"
```

---

### Task 5: Risk guard

**Files:**
- Create: `internal/trader/guard.go`
- Test: `internal/trader/guard_test.go`

**Interfaces:**
- Consumes: `order.RiskState` (Task 4).
- Produces:
  - `type Caps struct { MaxOrder, MaxDaily, MaxLoss int64 }` (satang).
  - `type Decision struct { Allowed bool; Reason string; TripHalt bool }`.
  - `func Check(state order.RiskState, caps Caps, orderValueTHB int64) Decision` — pure function, no I/O.

- [ ] **Step 1: Write the failing test**

`internal/trader/guard_test.go`:
```go
package trader

import (
	"testing"

	"snow-white/internal/order"
)

func TestGuard(t *testing.T) {
	caps := Caps{MaxOrder: 5000_00, MaxDaily: 50000_00, MaxLoss: 10000_00}
	tests := []struct {
		name      string
		state     order.RiskState
		value     int64
		wantOK    bool
		wantHalt  bool
	}{
		{"ok", order.RiskState{}, 1000_00, true, false},
		{"halted", order.RiskState{Halted: true}, 100, false, false},
		{"over per-order", order.RiskState{}, 6000_00, false, false},
		{"over daily", order.RiskState{SpentToday: 48000_00}, 3000_00, false, false},
		{"loss stop trips halt", order.RiskState{LossToday: 10000_00}, 100, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := Check(tc.state, caps, tc.value)
			if d.Allowed != tc.wantOK {
				t.Fatalf("Allowed = %v, want %v (%s)", d.Allowed, tc.wantOK, d.Reason)
			}
			if d.TripHalt != tc.wantHalt {
				t.Fatalf("TripHalt = %v, want %v", d.TripHalt, tc.wantHalt)
			}
		})
	}
}
```

- [ ] **Step 2: Run → fail**

Run: `go test ./internal/trader/ -run TestGuard -v`
Expected: FAIL — `undefined: Check`.

- [ ] **Step 3: Implement the guard**

`internal/trader/guard.go`:
```go
package trader

import (
	"fmt"

	"snow-white/internal/order"
)

type Caps struct {
	MaxOrder int64 // satang per order
	MaxDaily int64 // satang deployed per day
	MaxLoss  int64 // satang daily realized loss before halt
}

type Decision struct {
	Allowed  bool
	Reason   string
	TripHalt bool // caller should persist halted=true
}

// Check evaluates an order of orderValueTHB satang against the day's risk state
// and caps. Order matters: kill switch, loss stop, per-order, daily.
func Check(state order.RiskState, caps Caps, orderValueTHB int64) Decision {
	if state.Halted {
		return Decision{Allowed: false, Reason: "kill switch active"}
	}
	if caps.MaxLoss > 0 && state.LossToday >= caps.MaxLoss {
		return Decision{Allowed: false, Reason: "daily loss stop", TripHalt: true}
	}
	if caps.MaxOrder > 0 && orderValueTHB > caps.MaxOrder {
		return Decision{Allowed: false, Reason: fmt.Sprintf("exceeds per-order cap (%d > %d)", orderValueTHB, caps.MaxOrder)}
	}
	if caps.MaxDaily > 0 && state.SpentToday+orderValueTHB > caps.MaxDaily {
		return Decision{Allowed: false, Reason: "exceeds daily cap"}
	}
	return Decision{Allowed: true}
}
```

- [ ] **Step 4: Run → pass**

Run: `go test ./internal/trader/ -run TestGuard -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/trader/guard.go internal/trader/guard_test.go
git commit -m "feat(phase2): risk guard (kill switch, loss stop, per-order + daily caps)"
```

---

### Task 6: Kill switch (file OR DB)

**Files:**
- Create: `internal/trader/killswitch.go`
- Test: `internal/trader/killswitch_test.go`

**Interfaces:**
- Produces:
  - `func KillFileTripped(path string) bool` — true if the file exists (empty path → never).

- [ ] **Step 1: Write the failing test**

`internal/trader/killswitch_test.go`:
```go
package trader

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKillFileTripped(t *testing.T) {
	if KillFileTripped("") {
		t.Fatal("empty path must be false")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, ".halt")
	if KillFileTripped(p) {
		t.Fatal("absent file must be false")
	}
	if err := os.WriteFile(p, []byte("stop"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !KillFileTripped(p) {
		t.Fatal("present file must be true")
	}
}
```

- [ ] **Step 2: Run → fail**

Run: `go test ./internal/trader/ -run TestKillFile -v`
Expected: FAIL — `undefined: KillFileTripped`.

- [ ] **Step 3: Implement**

`internal/trader/killswitch.go`:
```go
package trader

import "os"

// KillFileTripped reports whether the kill-switch file exists. An empty path
// disables the file switch (DB halt flag still applies separately).
func KillFileTripped(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}
```

- [ ] **Step 4: Run → pass**

Run: `go test ./internal/trader/ -run TestKillFile -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/trader/killswitch.go internal/trader/killswitch_test.go
git commit -m "feat(phase2): kill-switch file check"
```

---

### Task 7: Order pipeline (paper/live, idempotent, one tx)

**Files:**
- Create: `internal/trader/pipeline.go`
- Test: `internal/trader/pipeline_test.go`

**Interfaces:**
- Consumes: `invx` order methods (Task 2), `order.Store` (Task 4), `Check`/`Caps` (Task 5).
- Produces:
  - `type Broker interface { SendOrder(ctx context.Context, in invx.SendOrderInput) (int64, error) }` (satisfied by `*invx.Client`).
  - `type OrderStore interface { InsertPending(...); Settle(...); RiskToday(...); ApplyFill(...); SetHalted(...); GetPosition(...) }` (satisfied by `*order.Store`; list exact methods used).
  - `type Pipeline struct { ... }`; `func NewPipeline(b Broker, s OrderStore, caps Caps, live bool, killFile string, now func() time.Time) *Pipeline`.
  - `type Intent struct { Symbol string; Side invx.Side; RefPrice int64; ValueTHB int64; Quantity int64; Strategy string }` (Buy uses ValueTHB; Sell uses Quantity; RefPrice = latest close satang for the limit price).
  - `func (p *Pipeline) Place(ctx context.Context, in Intent) (order.Order, error)` — guard → paper(log+insert accepted, no API) | live(insert pending → SendOrder → ApplyFill or Settle rejected). Returns the resulting order row.

- [ ] **Step 1: Write the failing tests (fakes)**

`internal/trader/pipeline_test.go` — define a `fakeBroker` (records SendOrder calls, returns a configurable orderId/err) and a `fakeStore` (records InsertPending/Settle/ApplyFill, serves a configurable RiskState). Tests:
```go
// 1. Paper mode: no SendOrder call; order row inserted with mode=paper, status=accepted.
// 2. Guard block (over per-order cap): no insert, no SendOrder; returns error mentioning the cap.
// 3. Live accepted: InsertPending(mode=live,pending) THEN SendOrder THEN ApplyFill(accepted, exchangeRef set).
// 4. Live rejected (broker returns error): pending row settled rejected with the error reason; no ApplyFill.
// 5. Loss-stop intent: guard trips halt → SetHalted called, order not placed.
```
Write each as a focused subtest with assertions on the fake's recorded calls. (The fakes make this pure-unit — no DB, no network.)

- [ ] **Step 2: Run → fail**

Run: `go test ./internal/trader/ -run TestPipeline -v`
Expected: FAIL — `undefined: NewPipeline`.

- [ ] **Step 3: Implement the pipeline**

`internal/trader/pipeline.go`:
```go
package trader

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"snow-white/internal/invx"
	"snow-white/internal/order"
)

type Broker interface {
	SendOrder(ctx context.Context, in invx.SendOrderInput) (int64, error)
}

type OrderStore interface {
	InsertPending(ctx context.Context, in order.InsertPendingInput) (order.Order, error)
	Settle(ctx context.Context, id int64, status order.Status, exchangeRef, reason string) error
	RiskToday(ctx context.Context, day time.Time) (order.RiskState, error)
	ApplyFill(ctx context.Context, in order.FillInput) error
	SetHalted(ctx context.Context, day time.Time, halted bool, reason string) error
	GetPosition(ctx context.Context, symbol string) (order.Position, error)
}

type Pipeline struct {
	broker   Broker
	store    OrderStore
	caps     Caps
	live     bool
	killFile string
	now      func() time.Time
}

func NewPipeline(b Broker, s OrderStore, caps Caps, live bool, killFile string, now func() time.Time) *Pipeline {
	if now == nil {
		now = time.Now
	}
	return &Pipeline{broker: b, store: s, caps: caps, live: live, killFile: killFile, now: now}
}

type Intent struct {
	Symbol   string
	Side     invx.Side
	RefPrice int64 // satang, used as the limit price
	ValueTHB int64 // satang, for Buy
	Quantity int64 // x1e8, for Sell
	Strategy string
}

// orderValue returns the satang exposure of the intent for guard purposes.
func (in Intent) orderValue() int64 {
	if in.Side == invx.Buy {
		return in.ValueTHB
	}
	// Sell exposure ≈ quantity * refPrice / 1e8 (satang).
	return in.Quantity * in.RefPrice / 1e8
}

func sideStr(s invx.Side) string {
	if s == invx.Sell {
		return "SELL"
	}
	return "BUY"
}

func (p *Pipeline) Place(ctx context.Context, in Intent) (order.Order, error) {
	day := p.now().UTC()

	// Kill-file forces halt before anything else.
	if KillFileTripped(p.killFile) {
		_ = p.store.SetHalted(ctx, day, true, "kill file present")
		return order.Order{}, fmt.Errorf("blocked: kill file present")
	}

	state, err := p.store.RiskToday(ctx, day)
	if err != nil {
		return order.Order{}, err
	}

	dec := Check(state, p.caps, in.orderValue())
	if !dec.Allowed {
		if dec.TripHalt {
			_ = p.store.SetHalted(ctx, day, true, dec.Reason)
		}
		return order.Order{}, fmt.Errorf("blocked: %s", dec.Reason)
	}

	mode := order.Paper
	if p.live {
		mode = order.Live
	}

	pending, err := p.store.InsertPending(ctx, order.InsertPendingInput{
		ClientUID:  uuid.NewString(),
		Symbol:     in.Symbol,
		Side:       sideStr(in.Side),
		Type:       "LIMIT",
		LimitPrice: in.RefPrice,
		Quantity:   in.Quantity,
		Mode:       string(mode),
		Strategy:   in.Strategy,
	})
	if err != nil {
		return order.Order{}, err
	}

	if !p.live {
		// Paper: simulate an immediate fill at RefPrice so the position updates
		// and Buy/Sell alternation works (no API call). Update spend + realized loss.
		pos, err := p.store.GetPosition(ctx, in.Symbol)
		if err != nil {
			return order.Order{}, err
		}
		newQty, newAvg, newPnl, spent := simulatePaperFill(pos, in)
		lossDelta := int64(0)
		if d := newPnl - pos.RealizedPnl; d < 0 {
			lossDelta = -d
		}
		if err := p.store.ApplyFill(ctx, order.FillInput{
			OrderID: pending.ID, Symbol: in.Symbol, Day: day,
			NewQty: newQty, NewAvgCost: newAvg, NewRealizedPnl: newPnl,
			SpentDelta: spent, LossDelta: lossDelta, ExchangeRef: "paper",
		}); err != nil {
			return order.Order{}, err
		}
		pending.Status = string(order.Accepted)
		pending.Mode = string(order.Paper)
		return pending, nil
	}

	// Live: send the order using our orders.id as clientOrderId.
	send := invx.SendOrderInput{
		Symbol: in.Symbol, Side: in.Side, Type: invx.Limit,
		LimitPrice: in.RefPrice, ClientOrderID: pending.ID,
	}
	if in.Side == invx.Buy {
		send.Value = in.ValueTHB
	} else {
		send.Quantity = in.Quantity
	}
	orderID, err := p.broker.SendOrder(ctx, send)
	if err != nil {
		_ = p.store.Settle(ctx, pending.ID, order.Rejected, "", err.Error())
		return order.Order{}, fmt.Errorf("send order: %w", err)
	}
	if err := p.store.ApplyFill(ctx, order.FillInput{
		OrderID: pending.ID, Symbol: in.Symbol, Day: day,
		NewQty: 0, NewAvgCost: 0, NewRealizedPnl: 0,
		SpentDelta: in.orderValue(), LossDelta: 0,
		ExchangeRef: fmt.Sprintf("%d", orderID),
	}); err != nil {
		return order.Order{}, err
	}
	pending.Status = string(order.Accepted)
	pending.ExchangeRef = fmt.Sprintf("%d", orderID)
	return pending, nil
}

// simulatePaperFill computes the position after an immediate fill at RefPrice.
// Buy deploys ValueTHB (satang) into units; Sell liquidates Quantity and realizes PnL.
func simulatePaperFill(pos order.Position, in Intent) (qty, avg, pnl, spent int64) {
	if in.Side == invx.Buy {
		units := in.ValueTHB * 1e8 / in.RefPrice
		return pos.Qty + units, in.RefPrice, pos.RealizedPnl, in.ValueTHB
	}
	proceeds := in.Quantity * in.RefPrice / 1e8 // satang
	cost := in.Quantity * pos.AvgCost / 1e8     // satang
	return pos.Qty - in.Quantity, pos.AvgCost, pos.RealizedPnl + (proceeds - cost), 0
}
```
> NOTE on live vs paper position tracking: **paper** simulates the fill immediately (above), so paper mode trades realistically. **Live** GTC-limit orders may not fill instantly, so the live path counts `SpentDelta` for the daily cap but leaves true `qty`/`avg_cost`/`realized_pnl` to the fill-reconcile (Task 11 reconciles order STATE; a follow-up should also read `quantityExecuted`/`avgPrice` to set live position/PnL). This divergence is intentional and flagged as a watch-item, not silently skipped.

- [ ] **Step 4: Run → pass**

Run: `go test ./internal/trader/ -run TestPipeline -v`
Expected: PASS — all five subtests.

- [ ] **Step 5: Commit**

```bash
git add internal/trader/pipeline.go internal/trader/pipeline_test.go
git commit -m "feat(phase2): order pipeline (paper default, guarded, idempotent live path)"
```

---

### Task 8: Trade daemon loop

**Files:**
- Create: `internal/trader/trader.go`
- Test: `internal/trader/trader_test.go`

**Interfaces:**
- Consumes: `candle.Candle`, `strategy.Strategy`/`Signal`/`Buy`/`Sell`, `Pipeline.Place`, `order.Position`.
- Produces:
  - `type CandleSource interface { List(ctx, symbol string, from, to time.Time, limit int32) ([]candle.Candle, error) }` (satisfied by `*candle.Store`).
  - `type Trader struct { ... }`; `func NewTrader(src CandleSource, strat strategy.Strategy, pipe *Pipeline, store OrderStore, symbol string, buyValueTHB int64, interval time.Duration) *Trader`.
  - `func (t *Trader) Tick(ctx context.Context) error` — load recent candles → Evaluate → on Buy place a Buy intent (value=buyValueTHB) when flat; on Sell place a Sell intent (quantity=position qty) when holding; Hold does nothing.
  - `func (t *Trader) Run(ctx context.Context) error` — Tick every interval until ctx done.

- [ ] **Step 1: Write the failing test (fakes)**

`internal/trader/trader_test.go`: fake CandleSource returns a fixed candle slice; a stub strategy returns Buy; a fake pipeline (record Place intents) — assert `Tick` places a Buy intent with `ValueTHB == buyValueTHB` and `RefPrice == lastClose` when position is flat, and does NOT place when the strategy returns Hold. Use the position store fake returning qty 0.
(Define a small `placer` interface the Trader calls so the test can inject a fake — or have Trader hold `*Pipeline` and inject a fake Broker+Store; prefer a `placer interface { Place(ctx, Intent) (order.Order, error) }` field for testability.)

- [ ] **Step 2: Run → fail**

Run: `go test ./internal/trader/ -run TestTrader -v`
Expected: FAIL — `undefined: NewTrader`.

- [ ] **Step 3: Implement the trader**

`internal/trader/trader.go`:
```go
package trader

import (
	"context"
	"fmt"
	"log"
	"time"

	"snow-white/internal/candle"
	"snow-white/internal/invx"
	"snow-white/internal/order"
	"snow-white/internal/strategy"
)

type CandleSource interface {
	List(ctx context.Context, symbol string, from, to time.Time, limit int32) ([]candle.Candle, error)
}

type placer interface {
	Place(ctx context.Context, in Intent) (order.Order, error)
}

type positionReader interface {
	GetPosition(ctx context.Context, symbol string) (order.Position, error)
}

type Trader struct {
	src      CandleSource
	strat    strategy.Strategy
	pipe     placer
	pos      positionReader
	symbol   string
	buyValue int64 // satang to deploy per Buy
	interval time.Duration
	now      func() time.Time
}

func NewTrader(src CandleSource, strat strategy.Strategy, pipe placer, pos positionReader, symbol string, buyValueTHB int64, interval time.Duration) *Trader {
	return &Trader{src: src, strat: strat, pipe: pipe, pos: pos, symbol: symbol, buyValue: buyValueTHB, interval: interval, now: time.Now}
}

func (t *Trader) Tick(ctx context.Context) error {
	to := t.now().UTC()
	from := to.AddDate(0, 0, -1) // last day of 1-min candles is plenty for warm-up
	cs, err := t.src.List(ctx, t.symbol, from, to, 100000)
	if err != nil {
		return fmt.Errorf("load candles: %w", err)
	}
	if len(cs) == 0 {
		return nil
	}
	sig := t.strat.Evaluate(cs)
	if sig.Action == strategy.Hold {
		return nil
	}
	last := cs[len(cs)-1].Close
	pos, err := t.pos.GetPosition(ctx, t.symbol)
	if err != nil {
		return err
	}
	switch sig.Action {
	case strategy.Buy:
		if pos.Qty > 0 {
			return nil // already long
		}
		_, err = t.pipe.Place(ctx, Intent{Symbol: t.symbol, Side: invx.Buy, RefPrice: last, ValueTHB: t.buyValue, Strategy: t.strat.Name()})
	case strategy.Sell:
		if pos.Qty <= 0 {
			return nil // nothing to sell
		}
		_, err = t.pipe.Place(ctx, Intent{Symbol: t.symbol, Side: invx.Sell, RefPrice: last, Quantity: pos.Qty, Strategy: t.strat.Name()})
	}
	if err != nil {
		log.Printf("trader: place blocked/failed: %v", err)
	}
	return nil
}

func (t *Trader) Run(ctx context.Context) error {
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := t.Tick(ctx); err != nil {
				log.Printf("trader: tick error: %v", err)
			}
		}
	}
}
```

- [ ] **Step 4: Run → pass**

Run: `go test ./internal/trader/ -run TestTrader -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/trader/trader.go internal/trader/trader_test.go
git commit -m "feat(phase2): trade daemon loop (signal -> guarded intent)"
```

---

### Task 9: `trade` command (daemon wiring, paper default)

**Files:**
- Create: `internal/cli/trade.go`
- Modify: `internal/cli/root.go`

**Interfaces:**
- Consumes: `config.Load`, `invx.New`, `candle.NewStore`, `order.NewStore`, `trader.{NewPipeline,NewTrader,Caps}`, `strategy.NewMACross`.
- Produces: `func newTradeCmd() *cobra.Command`; attached to root.

This is thin wiring; it ends with a build + a PAPER-mode manual run (no `--live`). No unit test (units are tested).

- [ ] **Step 1: Implement the command**

`internal/cli/trade.go`:
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
	"snow-white/internal/config"
	"snow-white/internal/invx"
	"snow-white/internal/order"
	"snow-white/internal/strategy"
	"snow-white/internal/trader"
)

func newTradeCmd() *cobra.Command {
	var symbol string
	var fast, slow int
	var live bool
	var buyTHB float64
	var interval time.Duration

	cmd := &cobra.Command{
		Use:   "trade",
		Short: "Run the MA-cross trader (PAPER by default; --live places real orders)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if symbol == "" {
				return fmt.Errorf("--symbol required")
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if interval == 0 {
				interval = cfg.CollectInterval
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			pool, err := pgxpool.New(ctx, cfg.PSQLURL)
			if err != nil {
				return fmt.Errorf("connect postgres: %w", err)
			}
			defer pool.Close()

			client := invx.New(cfg.APIKey, cfg.Secret, cfg.Host, nil)
			candleStore := candle.NewStore(pool)
			orderStore := order.NewStore(pool)
			caps := trader.Caps{MaxOrder: cfg.MaxOrder, MaxDaily: cfg.MaxDaily, MaxLoss: cfg.MaxLoss}
			pipe := trader.NewPipeline(client, orderStore, caps, live, cfg.KillFile, nil)
			strat := strategy.NewMACross(fast, slow)
			tr := trader.NewTrader(candleStore, strat, pipe, orderStore, symbol, int64(buyTHB*100), interval)

			mode := "PAPER"
			if live {
				mode = "LIVE"
			}
			fmt.Printf("trader starting: %s %s caps[order=%d daily=%d loss=%d] kill=%q\n",
				strat.Name(), mode, caps.MaxOrder, caps.MaxDaily, caps.MaxLoss, cfg.KillFile)

			if err := tr.Run(ctx); err != nil && err != context.Canceled {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&symbol, "symbol", "", "symbol, e.g. BTCTHB")
	cmd.Flags().IntVar(&fast, "fast", 20, "fast SMA period")
	cmd.Flags().IntVar(&slow, "slow", 50, "slow SMA period")
	cmd.Flags().BoolVar(&live, "live", false, "place REAL orders (default: paper/dry-run)")
	cmd.Flags().Float64Var(&buyTHB, "buy-thb", 1000, "THB to deploy per Buy signal")
	cmd.Flags().DurationVar(&interval, "interval", 0, "evaluation interval (default: INVX_COLLECT_INTERVAL)")
	return cmd
}
```

- [ ] **Step 2: Attach + build**

Add `root.AddCommand(newTradeCmd())` in `internal/cli/root.go`.
Run: `go build -o snow-white . && go test ./... && ./snow-white trade --help`
Expected: build OK, tests PASS, help shows `--live` defaulting false.

- [ ] **Step 3: Manual PAPER verification (no real orders)**

Prereq: candles in the DB (from `collect`). Run for ~2 min:
```bash
timeout 130 ./snow-white trade --symbol BTCTHB --fast 5 --slow 20 --buy-thb 1000 --interval 60s ; echo "exit=$?"
```
Expected: starts in PAPER mode; on a Buy/Sell signal logs/inserts a paper order. Verify:
```bash
psql "$PSQL_URL" -c "SELECT id, symbol, side, mode, status, strategy FROM orders ORDER BY id DESC LIMIT 5;"
```
Expected: any orders are `mode=paper`. **Do NOT pass `--live`.**

- [ ] **Step 4: Commit**

```bash
git add internal/cli/trade.go internal/cli/root.go
git commit -m "feat(phase2): trade daemon command (paper default)"
```

---

### Task 10: Manual `order` command group + `balance` + `status`

**Files:**
- Create: `internal/cli/order.go`
- Modify: `internal/cli/root.go`

**Interfaces:**
- Consumes: `invx` order/balance methods, `order.Store` (positions + risk), `config.Load`.
- Produces: `func newOrderCmd() *cobra.Command` (subcommands `send`, `cancel`, `open`, `hist`), `func newBalanceCmd()`, `func newStatusCmd()`; attached to root.

- [ ] **Step 1: Implement the commands**

`internal/cli/order.go` — `order send` requires `--live` to actually send (otherwise prints what it WOULD send) and prompts for `y/N` confirmation before a live send; `order cancel/open/hist` and `balance` are read/cancel ops that hit the API; `status` prints the day's risk_state + positions from PG. Money flags accept THB/coin decimals parsed once to int via `scale.Parse`. Build the `invx` client and `pgxpool` from config exactly as `trade.go` does.

```go
// Sketch of the send subcommand body (full code follows the trade.go wiring pattern):
//   parse --side (BUY/SELL) --type (LIMIT/MARKET) --price (THB) --qty (coin) or --value (THB)
//   priceSatang, _ := scale.Parse(priceStr, 2); qtyUnits, _ := scale.Parse(qtyStr, 8)
//   if !live { fmt.Printf("DRY-RUN would send: %+v\n", in); return nil }
//   confirm y/N; then client.SendOrder(ctx, in); print returned orderId
```
Implement each subcommand fully (no placeholders) following the established `cmd/*.go` pattern: parse flags → build client/pool → call the `invx`/`order.Store` method → print result via `render`-free `fmt`. `status` reads `order.NewStore(pool).RiskToday(ctx, time.Now())` and `GetPosition` for the requested symbol.

- [ ] **Step 2: Attach + build + help**

Add `root.AddCommand(newOrderCmd(), newBalanceCmd(), newStatusCmd())`.
Run: `go build -o snow-white . && go test ./... && ./snow-white order send --help && ./snow-white balance --help && ./snow-white status --help`
Expected: build OK, tests PASS, help shows the flags; `order send` shows `--live` defaulting false.

- [ ] **Step 3: Manual read-only verification**

```bash
./snow-white balance              # lists account products/amounts (read-only)
./snow-white order open           # lists open orders (read-only)
./snow-white status --symbol BTCTHB  # day risk_state + position
./snow-white order send --side BUY --type LIMIT --price 1000000 --value 1000   # DRY-RUN (no --live): prints intended order, sends nothing
```
Expected: read commands return real data; `order send` without `--live` sends nothing.

- [ ] **Step 4: Commit**

```bash
git add internal/cli/order.go internal/cli/root.go
git commit -m "feat(phase2): manual order/balance/status commands (send needs --live + confirm)"
```

---

### Task 11: `kill` / `resume` + startup reconcile

**Files:**
- Create: `internal/cli/risk.go`
- Modify: `internal/cli/trade.go` (call reconcile before `Run`)
- Create: `internal/trader/reconcile.go`
- Test: `internal/trader/reconcile_test.go`

**Interfaces:**
- Produces:
  - `func newKillCmd()` (sets `risk_state.halted=true` via `order.Store.SetHalted`), `func newResumeCmd()` (clears halt; prompts y/N), attached to root.
  - `func Reconcile(ctx context.Context, store OrderStore, hist OrderHistorySource, symbol string) (int, error)` — for each `pending` live order, look it up in order history by `clientOrderId` (== our order id) and settle accepted/rejected; returns count reconciled. `type OrderHistorySource interface { OrderHistory(ctx, symbol string, depth int) ([]invx.OrderInfo, error) }`.

- [ ] **Step 1: Write the failing reconcile test**

Fakes: a store returning one `pending` live order (id=5), and a fake history source returning an `OrderInfo{ClientOrderID:5, State:"FullyExecuted", OrderID:99}`. Assert `Reconcile` settles order 5 → accepted with exchangeRef "99". Add a case where history shows `State:"Rejected"` → settled rejected.

- [ ] **Step 2: Run → fail**

Run: `go test ./internal/trader/ -run TestReconcile -v`
Expected: FAIL — `undefined: Reconcile`.

- [ ] **Step 3: Implement reconcile**

`internal/trader/reconcile.go`:
```go
package trader

import (
	"context"
	"fmt"

	"snow-white/internal/invx"
	"snow-white/internal/order"
)

type OrderHistorySource interface {
	OrderHistory(ctx context.Context, symbol string, depth int) ([]invx.OrderInfo, error)
}

type reconcileStore interface {
	ListPendingLive(ctx context.Context) ([]order.Order, error)
	Settle(ctx context.Context, id int64, status order.Status, exchangeRef, reason string) error
}

// Reconcile settles leftover pending live orders against exchange history,
// matching on clientOrderId == our order id. Run at trader startup so a crash
// mid-send never leaves a phantom pending row.
func Reconcile(ctx context.Context, store reconcileStore, hist OrderHistorySource, symbol string) (int, error) {
	pending, err := store.ListPendingLive(ctx)
	if err != nil {
		return 0, err
	}
	if len(pending) == 0 {
		return 0, nil
	}
	infos, err := hist.OrderHistory(ctx, symbol, 200)
	if err != nil {
		return 0, err
	}
	byClient := make(map[int64]invx.OrderInfo, len(infos))
	for _, oi := range infos {
		byClient[oi.ClientOrderID] = oi
	}
	n := 0
	for _, o := range pending {
		oi, ok := byClient[o.ID]
		if !ok {
			continue // not found yet; leave pending
		}
		status := order.Accepted
		reason := ""
		if oi.State == "Rejected" || oi.State == "Canceled" || oi.State == "Expired" {
			status = order.Rejected
			reason = oi.State
		}
		if err := store.Settle(ctx, o.ID, status, fmt.Sprintf("%d", oi.OrderID), reason); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
```

- [ ] **Step 4: Run → pass**

Run: `go test ./internal/trader/ -run TestReconcile -v`
Expected: PASS.

- [ ] **Step 5: Implement kill/resume commands**

`internal/cli/risk.go`: `kill --reason "..."` calls `order.NewStore(pool).SetHalted(ctx, time.Now(), true, reason)`; `resume` prompts y/N then `SetHalted(..., false, "")`. Build pool from config as in other commands. Attach both to root.

- [ ] **Step 6: Wire reconcile into trade startup**

In `internal/cli/trade.go`, before `tr.Run(ctx)` and only when `live`:
```go
			if live {
				if n, err := trader.Reconcile(ctx, orderStore, client, symbol); err != nil {
					return fmt.Errorf("startup reconcile: %w", err)
				} else if n > 0 {
					fmt.Printf("reconciled %d pending live order(s)\n", n)
				}
			}
```

- [ ] **Step 7: Build + test + help**

Run: `go build -o snow-white . && go test ./... && ./snow-white kill --help && ./snow-white resume --help`
Expected: build OK, all tests PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/cli/risk.go internal/cli/trade.go internal/trader/reconcile.go internal/trader/reconcile_test.go
git commit -m "feat(phase2): kill/resume commands + startup reconcile of pending live orders"
```

---

## Phase 2 Completion Gate

Before declaring Phase 2 done (real money — verify, don't infer):

- [ ] `go test ./...` green (unit + testcontainers).
- [ ] `trade` ran in PAPER mode against real candles and wrote `mode=paper` orders only (verified via `psql`).
- [ ] `balance`, `order open`, `status` returned real data from the API/DB.
- [ ] `order send` WITHOUT `--live` sent nothing (dry-run).
- [ ] Guard tested: a `--buy-thb` above `INVX_MAX_ORDER` is blocked; a kill-file halts trading.
- [ ] **ONE tiny live order** (e.g. `order send --live` for the minimum allowed size) placed and then canceled — ONLY after explicit user authorization, on the smallest possible size, with `INVX_MAX_ORDER` set low. Verify it appears via `order open` and cancels via `order cancel`.

## Known Phase-2 Watch-items (carry into the work)

- **Position/PnL from fills is conservative.** The pipeline counts `spent_today` for the daily cap but leaves true `qty`/`avg_cost`/`realized_pnl` for a fill-reconcile follow-up (GTC-limit orders may not fill instantly). Build the fill-reconcile (read `quantityExecuted`/`avgPrice` from history → update position + realized PnL) before trusting `status` P&L.
- **Restore `--dev-url`** in `Taskfile.yml` migrate tasks IF any future schema change is needed (Phase 2 needs none).
- **`scale.Parse` silent-zero** on malformed input (Phase-1 carryover) — order amount parsing should reject empty/malformed explicitly; add a guard in `order send` flag parsing.
- **Trade timestamp / clock** — the guard keys on `now().UTC()` date; ensure the host clock is correct (the API also enforces a 150s skew on requests).
