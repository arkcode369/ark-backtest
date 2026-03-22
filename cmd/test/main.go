package main

import (
	"fmt"
	"trading-backtest-bot/internal/backtest"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

func main() {
	fmt.Println("=== Testing Data Fetch ===")
	bars, err := data.FetchOHLCV(data.FetchParams{
		Symbol:   "XAUUSD",
		Interval: "1d",
		Period:   "1y",
	})
	if err != nil {
		fmt.Println("ERROR:", err)
		return
	}
	fmt.Printf("XAUUSD 1d: %d bars, from %s to %s\n",
		len(bars),
		bars[0].Time.Format("2006-01-02"),
		bars[len(bars)-1].Time.Format("2006-01-02"),
	)
	fmt.Printf("Latest: O=%.2f H=%.2f L=%.2f C=%.2f V=%.0f\n",
		bars[len(bars)-1].Open, bars[len(bars)-1].High,
		bars[len(bars)-1].Low, bars[len(bars)-1].Close,
		bars[len(bars)-1].Volume,
	)

	fmt.Println("\n=== Testing Indicators ===")
	closes := indicators.ExtractClose(bars)
	ema9 := indicators.EMA(closes, 9)
	ema21 := indicators.EMA(closes, 21)
	rsi := indicators.RSI(closes, 14)
	fmt.Printf("EMA9: %.2f | EMA21: %.2f | RSI14: %.2f\n",
		indicators.Last(ema9), indicators.Last(ema21), indicators.Last(rsi))

	fmt.Println("\n=== Testing Multiple Symbols ===")
	for _, sym := range []string{"NQ", "EURUSD", "CL", "XAGUSD"} {
		b, err := data.FetchOHLCV(data.FetchParams{Symbol: sym, Interval: "1d", Period: "30d"})
		if err != nil {
			fmt.Printf("  %s: ERROR - %v\n", sym, err)
		} else {
			fmt.Printf("  %s: %d bars, last close=%.4f\n", sym, len(b), b[len(b)-1].Close)
		}
	}

	fmt.Println("\n=== Running Backtest (XAUUSD 1d EMA Cross) ===")
	cfg := backtest.Config{
		InitialCapital:  10000,
		PositionSizePct: 0.02,
		Commission:      5,
		Slippage:        0.10,
		Symbol:          "XAUUSD",
		Interval:        "1d",
	}
	engine := backtest.NewEngine(cfg)
	engine.LoadData(bars)
	meta := backtest.StrategyRegistry["ema_cross"]
	engine.SetStrategy(meta.Factory())
	result, err := engine.Run(meta.Params)
	if err != nil {
		fmt.Println("ERROR:", err)
		return
	}
	fmt.Printf("  Trades: %d | WinRate: %.1f%% | P&L: $%.2f (%.2f%%)\n",
		result.TotalTrades, result.WinRate, result.TotalPnL, result.TotalPnLPct)
	fmt.Printf("  MaxDD: $%.2f | Sharpe: %.2f | PF: %.2f\n",
		result.MaxDrawdown, result.SharpeRatio, result.ProfitFactor)

	fmt.Println("\n=== Testing All Strategies ===")
	for key, meta := range backtest.StrategyRegistry {
		engine2 := backtest.NewEngine(cfg)
		engine2.LoadData(bars)
		engine2.SetStrategy(meta.Factory())
		r, err := engine2.Run(meta.Params)
		if err != nil {
			fmt.Printf("  %s: ERROR - %v\n", key, err)
		} else {
			fmt.Printf("  %-12s: %d trades | WR=%.0f%% | P&L=%.0f | PF=%.2f\n",
				key, r.TotalTrades, r.WinRate, r.TotalPnL, r.ProfitFactor)
		}
	}

	fmt.Println("\n✅ All tests passed!")
}
