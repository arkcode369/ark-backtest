package backtest

import (
	"testing"
	"time"
	"trading-backtest-bot/internal/data"
)

func makeOBRetestBars() []data.OHLCV {
	base := time.Date(2024, 6, 10, 10, 0, 0, 0, time.UTC)
	minute := func(m int) time.Time { return base.Add(time.Duration(m) * time.Minute) }

	bars := make([]data.OHLCV, 40)

	// Bars 0-14: ranging, used to build up ATR history
	for i := 0; i < 15; i++ {
		bars[i] = data.OHLCV{
			Time: minute(i), Open: 100.0, High: 100.5, Low: 99.5, Close: 100.0, Volume: 100,
		}
	}

	// Bars 15-37: continue ranging
	for i := 15; i < 38; i++ {
		bars[i] = data.OHLCV{
			Time: minute(i), Open: 100.0, High: 100.5, Low: 99.5, Close: 100.0, Volume: 100,
		}
	}

	// Bar 38: bearish candle — this becomes the bullish OB's preceding candle.
	// OB zone: Top = bars[38].Open = 100.5, Bottom = bars[38].Low = 99.0
	bars[38] = data.OHLCV{Time: minute(38), Open: 100.5, High: 100.6, Low: 99.0, Close: 99.2, Volume: 100}

	// Bar 39: large bullish impulse candle — creates bullish OB at bar 39.
	// Since this is the LAST bar, the mitigation scan (j=40..39) is empty,
	// so the OB remains unmitigated (Mitigated=false).
	// The signal bar (39) itself touches the OB zone:
	//   barLow (99.0) <= ob.Top (100.5) ✓
	//   barHigh (104.0) >= ob.Bottom (99.0) ✓
	//   price = Close (103.5) >= ob.Bottom (99.0) ✓
	bars[39] = data.OHLCV{Time: minute(39), Open: 99.2, High: 104.0, Low: 99.0, Close: 103.5, Volume: 300}

	return bars
}

func TestOBRetestBullish(t *testing.T) {
	bars := makeOBRetestBars()
	s := &OBRetestStrategy{}
	s.Init(bars, map[string]float64{
		"swing_period":  3,
		"atr_period":    5,
		"ob_impulse":    0.5,
		"min_ob_age":    0, // OB forms at last bar, so age is 0
		"pd_lookback":   50,
		"use_pd_filter": 0, // disable PD filter for simpler test
	})

	sig := s.Signal(len(bars) - 1)
	if sig != BuySignal {
		t.Errorf("expected BuySignal on OB retest, got %d", sig)
	}
}

func TestOBRetestNoSignalTooYoung(t *testing.T) {
	bars := makeOBRetestBars()
	s := &OBRetestStrategy{}
	s.Init(bars, map[string]float64{
		"swing_period":  3,
		"atr_period":    5,
		"ob_impulse":    0.5,
		"min_ob_age":    100, // OB too young
		"pd_lookback":   50,
		"use_pd_filter": 0,
	})

	sig := s.Signal(len(bars) - 1)
	if sig != NoSignal {
		t.Errorf("expected NoSignal when OB too young, got %d", sig)
	}
}

func TestOBRetestRegistry(t *testing.T) {
	meta, ok := StrategyRegistry["ob_retest"]
	if !ok {
		t.Fatal("ob_retest not in registry")
	}
	s := meta.Factory()
	if s.Name() != "OB Retest" {
		t.Errorf("expected name 'OB Retest', got '%s'", s.Name())
	}
}
