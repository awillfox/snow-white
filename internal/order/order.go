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

// NewFromSQLC maps a generated sqlc.Order to the domain Order.
// pgtype.UUID.String() returns the hyphenated form (e.g. "019d1bae-...") or "" when not valid.
// pgtype.Int8.Int64 holds the limit price (0 when NULL/not-set).
// pgtype.Text.String holds nullable text fields (empty string when NULL).
// pgtype.Timestamptz.Time holds the Go time.Time value.
func NewFromSQLC(o sqlc.Order) Order {
	return Order{
		ID:          o.ID,
		ClientUID:   o.ClientUid.String(), // pgtype.UUID.String() → hyphenated uuid string
		Symbol:      o.Symbol,
		Side:        o.Side,
		Type:        o.Type,
		LimitPrice:  o.LimitPrice.Int64, // pgtype.Int8 nullable → zero when NULL
		Quantity:    o.Quantity,
		Mode:        o.Mode,
		Strategy:    o.Strategy.String, // pgtype.Text nullable
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
