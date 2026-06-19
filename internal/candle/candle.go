package candle

import (
	"time"

	"snow-white/sqlc"
)

// Candle is one OHLCV bar. Prices are satang (THB*100); Volume is asset units *1e8.
type Candle struct {
	ID         int64     `json:"id"`
	Symbol     string    `json:"symbol"`
	OpenTime   time.Time `json:"open_time"`
	Open       int64     `json:"open"`
	High       int64     `json:"high"`
	Low        int64     `json:"low"`
	Close      int64     `json:"close"`
	Volume     int64     `json:"volume"`
	InsideBid  int64     `json:"inside_bid"`
	InsideAsk  int64     `json:"inside_ask"`
	Source     string    `json:"source"`
	IngestedAt time.Time `json:"ingested_at"`
}

func NewFromSQLC(c sqlc.Candle) Candle {
	return Candle{
		ID:         c.ID,
		Symbol:     c.Symbol,
		OpenTime:   c.OpenTime.Time,
		Open:       c.Open,
		High:       c.High,
		Low:        c.Low,
		Close:      c.Close,
		Volume:     c.Volume,
		InsideBid:  c.InsideBid,
		InsideAsk:  c.InsideAsk,
		Source:     c.Source,
		IngestedAt: c.IngestedAt.Time,
	}
}
