package collector

import (
	"context"
	"fmt"
	"log"
	"time"

	"snow-white/internal/candle"
	"snow-white/internal/invx"
)

type TickerFetcher interface {
	Ticker(ctx context.Context, symbol string) ([]invx.TickerCandle, error)
}

type CandleUpserter interface {
	Upsert(ctx context.Context, c candle.Candle) error
}

type Collector struct {
	fetcher  TickerFetcher
	store    CandleUpserter
	symbols  []string
	interval time.Duration
}

func New(f TickerFetcher, u CandleUpserter, symbols []string, interval time.Duration) *Collector {
	return &Collector{fetcher: f, store: u, symbols: symbols, interval: interval}
}

// PollOnce fetches each symbol once and upserts every returned candle.
func (c *Collector) PollOnce(ctx context.Context) (int, error) {
	total := 0
	for _, sym := range c.symbols {
		tcs, err := c.fetcher.Ticker(ctx, sym)
		if err != nil {
			return total, fmt.Errorf("fetch ticker %s: %w", sym, err)
		}
		for _, tc := range tcs {
			cd := candle.Candle{
				Symbol:    tc.Symbol,
				OpenTime:  tc.DateTime,
				Open:      tc.Open,
				High:      tc.High,
				Low:       tc.Low,
				Close:     tc.Close,
				Volume:    tc.Volume,
				InsideBid: tc.InsideBid,
				InsideAsk: tc.InsideAsk,
				Source:    "ticker",
			}
			if err := c.store.Upsert(ctx, cd); err != nil {
				return total, fmt.Errorf("upsert %s: %w", sym, err)
			}
			total++
		}
	}
	return total, nil
}

// Run polls every interval until ctx is cancelled. The first poll logs how many
// candles each call returned per symbol so backfill behavior is observed.
func (c *Collector) Run(ctx context.Context) error {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	n, err := c.PollOnce(ctx)
	if err != nil {
		log.Printf("collector: first poll error: %v", err)
	} else {
		log.Printf("collector: first poll upserted %d candle(s) across %d symbol(s)", n, len(c.symbols))
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			n, err := c.PollOnce(ctx)
			if err != nil {
				log.Printf("collector: poll error: %v", err)
				continue
			}
			log.Printf("collector: upserted %d candle(s)", n)
		}
	}
}
