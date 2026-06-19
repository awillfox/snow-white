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
