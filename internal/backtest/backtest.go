// Package backtest replays a strategy over historical candles. All-in/all-out:
// Buy converts all cash to position at the bar close; Sell liquidates fully.
// Money and position value are satang int64; the only float is the drawdown ratio.
package backtest

import (
	"time"

	"snow-white/internal/candle"
	"snow-white/internal/strategy"
)

type Trade struct {
	Time      time.Time
	Action    string
	Price     int64
	Qty       int64 // position units *1e8 bought/sold
	CashAfter int64
}

type Result struct {
	Trades         []Trade
	StartCash      int64
	EndCash        int64
	PnL            int64
	NumTrades      int
	WinRate        float64
	MaxDrawdownPct float64
}

func Run(cs []candle.Candle, s strategy.Strategy, startCash int64, feeBps int) Result {
	cash := startCash
	var posUnits int64 // *1e8
	var entryCash int64
	var wins, closed int

	res := Result{StartCash: startCash}
	peak := startCash

	for i := range cs {
		price := cs[i].Close // satang per 1 unit
		sig := s.Evaluate(cs[:i+1])

		switch sig.Action {
		case strategy.Buy:
			if posUnits == 0 && cash > 0 && price > 0 {
				gross := cash
				fee := gross * int64(feeBps) / 10000
				spend := gross - fee
				posUnits = spend * 1e8 / price // units *1e8
				entryCash = cash
				cash = 0
				res.Trades = append(res.Trades, Trade{cs[i].OpenTime, "BUY", price, posUnits, cash})
				res.NumTrades++
			}
		case strategy.Sell:
			if posUnits > 0 {
				gross := posUnits * price / 1e8
				fee := gross * int64(feeBps) / 10000
				cash += gross - fee
				if cash > entryCash {
					wins++
				}
				closed++
				res.Trades = append(res.Trades, Trade{cs[i].OpenTime, "SELL", price, posUnits, cash})
				res.NumTrades++
				posUnits = 0
			}
		}

		// Mark-to-market equity for drawdown.
		equity := cash + posUnits*price/1e8
		if equity > peak {
			peak = equity
		}
		if peak > 0 {
			dd := float64(peak-equity) / float64(peak)
			if dd > res.MaxDrawdownPct {
				res.MaxDrawdownPct = dd
			}
		}
	}

	// Liquidate any open position at the last close for final accounting.
	if posUnits > 0 && len(cs) > 0 {
		price := cs[len(cs)-1].Close
		cash += posUnits * price / 1e8
		posUnits = 0
	}
	res.EndCash = cash
	res.PnL = cash - startCash
	if closed > 0 {
		res.WinRate = float64(wins) / float64(closed)
	}
	return res
}
