package backtest

import (
	"testing"
	"time"
	"trading-backtest-bot/internal/data"
)

func makeWeeklyBars() []data.OHLCV {
	// Create bars spanning two weeks
	bars := make([]data.OHLCV, 0)

	// Week 1: Mon-Fri, hourly bars, range 95-105
	for d := 0; d < 5; d++ {
		for h := 9; h < 17; h++ {
			t := time.Date(2024, 6, 3+d, h, 0, 0, 0, time.UTC)
			bars = append(bars, data.OHLCV{
				Time: t, Open: 100.0, High: 105.0, Low: 95.0, Close: 100.0, Volume: 100,
			})
		}
	}

	// Week 2: Mon-Wed, price opens at 100 (weekly open)
	// PWH=105, PWL=95 from week 1
	for d := 0; d < 3; d++ {
		for h := 9; h < 17; h++ {
			t := time.Date(2024, 6, 10+d, h, 0, 0, 0, time.UTC)
			bars = append(bars, data.OHLCV{
				Time: t, Open: 100.5, High: 101.0, Low: 100.0, Close: 100.5, Volume: 100,
			})
		}
	}

	// Last bar: bullish bounce off weekly open
	// Price drops to touch weekly open (100.0) then displacement up
	t := time.Date(2024, 6, 12, 15, 0, 0, 0, time.UTC)
	bars = append(bars, data.OHLCV{
		Time: t, Open: 100.2, High: 103.0, Low: 100.0, Close: 102.8, Volume: 300,
	})

	return bars
}

func TestWeeklyProfileBullish(t *testing.T) {
	bars := makeWeeklyBars()
	s := &WeeklyProfileStrategy{}
	s.Init(bars, map[string]float64{
		"swing_period": 3,
		"atr_period":   5,
		"disp_mult":    0.5,
		"body_ratio":   0.3,
		"touch_atr":    2.0,
	})

	sig := s.Signal(len(bars) - 1)
	if sig != BuySignal {
		t.Errorf("expected BuySignal on weekly open bounce, got %d", sig)
	}
}

func TestWeeklyProfileRegistry(t *testing.T) {
	meta, ok := StrategyRegistry["weekly_profile"]
	if !ok {
		t.Fatal("weekly_profile not in registry")
	}
	s := meta.Factory()
	if s.Name() != "Weekly Profile" {
		t.Errorf("expected name 'Weekly Profile', got '%s'", s.Name())
	}
}
