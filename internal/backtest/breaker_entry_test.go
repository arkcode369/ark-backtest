package backtest

import (
	"testing"
	"time"
	"trading-backtest-bot/internal/data"
)

func makeBreakerBars() []data.OHLCV {
	base := time.Date(2024, 6, 10, 10, 0, 0, 0, time.UTC)
	minute := func(m int) time.Time { return base.Add(time.Duration(m) * time.Minute) }

	bars := make([]data.OHLCV, 40)

	// Bars 0-9: ranging
	for i := 0; i < 10; i++ {
		bars[i] = data.OHLCV{
			Time: minute(i), Open: 100.0, High: 100.5, Low: 99.5, Close: 100.0, Volume: 100,
		}
	}

	// Bar 10: bullish candle (preceding candle for bearish OB)
	bars[10] = data.OHLCV{Time: minute(10), Open: 99.5, High: 101.0, Low: 99.3, Close: 100.8, Volume: 100}

	// Bar 11: large bearish impulse (creates bearish OB at bar 10)
	bars[11] = data.OHLCV{Time: minute(11), Open: 100.8, High: 101.0, Low: 96.0, Close: 96.5, Volume: 300}

	// Bars 12-19: price stays low
	for i := 12; i < 20; i++ {
		bars[i] = data.OHLCV{
			Time: minute(i), Open: 96.5, High: 97.0, Low: 96.0, Close: 96.5, Volume: 100,
		}
	}

	// Bar 20: price breaks ABOVE the bearish OB top (101.0) — OB becomes bullish breaker
	// Breaker zone = [OB bottom, OB top] = [99.5, 101.0]
	bars[20] = data.OHLCV{Time: minute(20), Open: 97.0, High: 102.0, Low: 96.5, Close: 101.5, Volume: 200}

	// Bars 21-35: price stays STRICTLY above breaker top (101.0) so breaker is NOT mitigated yet.
	// Breaker mitigation check: bars[k].Low <= z.Top (101.0), so Low must be > 101.0.
	for i := 21; i < 36; i++ {
		bars[i] = data.OHLCV{
			Time: minute(i), Open: 102.0, High: 102.5, Low: 101.1, Close: 102.0, Volume: 100,
		}
	}

	// Bars 36-38: retrace toward breaker zone but Low stays above 101.0
	bars[36] = data.OHLCV{Time: minute(36), Open: 102.0, High: 102.0, Low: 101.2, Close: 101.5, Volume: 100}
	bars[37] = data.OHLCV{Time: minute(37), Open: 101.5, High: 101.6, Low: 101.1, Close: 101.3, Volume: 100}
	bars[38] = data.OHLCV{Time: minute(38), Open: 101.3, High: 101.4, Low: 101.05, Close: 101.1, Volume: 100}

	// Bar 39: touches breaker zone [99.5, 101.0] — first bar with Low <= 101.0
	// Close >= Bottom (99.5) satisfies the bullish signal condition
	bars[39] = data.OHLCV{Time: minute(39), Open: 101.0, High: 101.2, Low: 100.5, Close: 100.8, Volume: 100}

	return bars
}

func TestBreakerEntryBullish(t *testing.T) {
	bars := makeBreakerBars()
	s := &BreakerEntryStrategy{}
	s.Init(bars, map[string]float64{
		"swing_period":  3,
		"atr_period":    5,
		"ob_impulse":    0.5,
		"min_age":       3,
		"pd_lookback":   50,
		"use_pd_filter": 0,
	})

	sig := s.Signal(len(bars) - 1)
	if sig != BuySignal {
		t.Errorf("expected BuySignal on breaker retest, got %d", sig)
	}
}

func TestBreakerEntryRegistry(t *testing.T) {
	meta, ok := StrategyRegistry["breaker_entry"]
	if !ok {
		t.Fatal("breaker_entry not in registry")
	}
	s := meta.Factory()
	if s.Name() != "Breaker Entry" {
		t.Errorf("expected name 'Breaker Entry', got '%s'", s.Name())
	}
}
