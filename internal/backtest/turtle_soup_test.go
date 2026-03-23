package backtest

import (
	"testing"
	"time"
	"trading-backtest-bot/internal/data"
)

// makeTSBars creates test bars for Turtle Soup.
// Setup: swing low at bar 5 → sweep below at bar 15 → bullish displacement at bar i (last bar).
func makeTSBars() []data.OHLCV {
	base := time.Date(2024, 6, 10, 10, 0, 0, 0, time.UTC)
	minute := func(m int) time.Time { return base.Add(time.Duration(m) * time.Minute) }

	bars := make([]data.OHLCV, 25)

	// Bars 0-9: range with swing low at bar 5
	for i := 0; i < 10; i++ {
		bars[i] = data.OHLCV{
			Time: minute(i), Open: 100.5, High: 101.0, Low: 100.0, Close: 100.5, Volume: 100,
		}
	}
	// Clear swing low at bar 5
	bars[5] = data.OHLCV{Time: minute(5), Open: 100.0, High: 100.2, Low: 98.5, Close: 100.0, Volume: 100}

	// Bars 10-14: higher range
	for i := 10; i < 15; i++ {
		bars[i] = data.OHLCV{
			Time: minute(i), Open: 101.0, High: 101.5, Low: 100.5, Close: 101.0, Volume: 100,
		}
	}

	// Bar 15: sweep below swing low (low < 98.5)
	bars[15] = data.OHLCV{Time: minute(15), Open: 100.0, High: 100.2, Low: 97.5, Close: 99.0, Volume: 200}

	// Bars 16-22: recovery
	for i := 16; i < 23; i++ {
		bars[i] = data.OHLCV{
			Time: minute(i), Open: 99.5, High: 100.5, Low: 99.0, Close: 100.0, Volume: 100,
		}
	}

	// Bar 23: small bar
	bars[23] = data.OHLCV{Time: minute(23), Open: 100.0, High: 100.5, Low: 99.8, Close: 100.2, Volume: 100}

	// Bar 24: bullish displacement (large body)
	bars[24] = data.OHLCV{Time: minute(24), Open: 100.0, High: 104.0, Low: 99.8, Close: 103.5, Volume: 400}

	return bars
}

func TestTurtleSoupBullish(t *testing.T) {
	bars := makeTSBars()
	s := &TurtleSoupStrategy{}
	s.Init(bars, map[string]float64{
		"swing_period": 3,
		"atr_period":   5,
		"disp_mult":    0.5,
		"body_ratio":   0.3,
		"ts_lookback":  20,
		"equal_tol":    0.1,
	})

	sig := s.Signal(len(bars) - 1)
	if sig != BuySignal {
		t.Errorf("expected BuySignal on displacement after sweep, got %d", sig)
	}
}

func TestTurtleSoupBearish(t *testing.T) {
	base := time.Date(2024, 6, 10, 10, 0, 0, 0, time.UTC)
	minute := func(m int) time.Time { return base.Add(time.Duration(m) * time.Minute) }

	bars := make([]data.OHLCV, 25)

	// Range with swing high at bar 5
	for i := 0; i < 10; i++ {
		bars[i] = data.OHLCV{
			Time: minute(i), Open: 100.5, High: 101.0, Low: 100.0, Close: 100.5, Volume: 100,
		}
	}
	// Swing high at bar 5
	bars[5] = data.OHLCV{Time: minute(5), Open: 101.0, High: 103.0, Low: 100.5, Close: 101.0, Volume: 100}

	// Bars 10-14: lower range
	for i := 10; i < 15; i++ {
		bars[i] = data.OHLCV{
			Time: minute(i), Open: 100.0, High: 100.5, Low: 99.5, Close: 100.0, Volume: 100,
		}
	}

	// Bar 15: sweep above swing high (high > 103.0)
	bars[15] = data.OHLCV{Time: minute(15), Open: 102.0, High: 103.5, Low: 101.5, Close: 102.0, Volume: 200}

	// Bars 16-22: drift
	for i := 16; i < 23; i++ {
		bars[i] = data.OHLCV{
			Time: minute(i), Open: 101.0, High: 101.5, Low: 100.5, Close: 101.0, Volume: 100,
		}
	}

	// Bar 23: small
	bars[23] = data.OHLCV{Time: minute(23), Open: 101.0, High: 101.2, Low: 100.8, Close: 101.0, Volume: 100}

	// Bar 24: bearish displacement
	bars[24] = data.OHLCV{Time: minute(24), Open: 101.0, High: 101.2, Low: 97.0, Close: 97.5, Volume: 400}

	s := &TurtleSoupStrategy{}
	s.Init(bars, map[string]float64{
		"swing_period": 3,
		"atr_period":   5,
		"disp_mult":    0.5,
		"body_ratio":   0.3,
		"ts_lookback":  20,
		"equal_tol":    0.1,
	})

	sig := s.Signal(len(bars) - 1)
	if sig != SellSignal {
		t.Errorf("expected SellSignal on bearish displacement after sweep, got %d", sig)
	}
}

func TestTurtleSoupNoSignalNoSweep(t *testing.T) {
	base := time.Date(2024, 6, 10, 10, 0, 0, 0, time.UTC)
	minute := func(m int) time.Time { return base.Add(time.Duration(m) * time.Minute) }

	// Flat bars with no sweep
	bars := make([]data.OHLCV, 25)
	for i := 0; i < 25; i++ {
		bars[i] = data.OHLCV{
			Time: minute(i), Open: 100.0, High: 100.5, Low: 99.5, Close: 100.0, Volume: 100,
		}
	}

	s := &TurtleSoupStrategy{}
	s.Init(bars, map[string]float64{
		"swing_period": 3,
		"atr_period":   5,
		"disp_mult":    0.5,
		"body_ratio":   0.3,
		"ts_lookback":  20,
	})

	sig := s.Signal(len(bars) - 1)
	if sig != NoSignal {
		t.Errorf("expected NoSignal with no sweep, got %d", sig)
	}
}

func TestTurtleSoupRegistry(t *testing.T) {
	meta, ok := StrategyRegistry["turtle_soup"]
	if !ok {
		t.Fatal("turtle_soup not in registry")
	}
	s := meta.Factory()
	if s.Name() != "Turtle Soup" {
		t.Errorf("expected name 'Turtle Soup', got '%s'", s.Name())
	}
}

func TestTurtleSoupEqualLows(t *testing.T) {
	base := time.Date(2024, 6, 10, 10, 0, 0, 0, time.UTC)
	minute := func(m int) time.Time { return base.Add(time.Duration(m) * time.Minute) }

	bars := make([]data.OHLCV, 30)

	// Range
	for i := 0; i < 30; i++ {
		bars[i] = data.OHLCV{
			Time: minute(i), Open: 100.5, High: 101.0, Low: 100.0, Close: 100.5, Volume: 100,
		}
	}

	// Two "equal" swing lows at bars 5 and 12 (within tolerance)
	bars[5] = data.OHLCV{Time: minute(5), Open: 100.0, High: 100.2, Low: 98.5, Close: 100.0, Volume: 100}
	bars[12] = data.OHLCV{Time: minute(12), Open: 100.0, High: 100.2, Low: 98.55, Close: 100.0, Volume: 100}

	// Bar 22: sweep below both equal lows
	bars[22] = data.OHLCV{Time: minute(22), Open: 99.5, High: 99.8, Low: 97.0, Close: 99.0, Volume: 200}

	// Bars 23-28: recovery
	for i := 23; i < 29; i++ {
		bars[i] = data.OHLCV{
			Time: minute(i), Open: 99.5, High: 100.5, Low: 99.0, Close: 100.0, Volume: 100,
		}
	}

	// Bar 29: bullish displacement
	bars[29] = data.OHLCV{Time: minute(29), Open: 100.0, High: 104.0, Low: 99.8, Close: 103.5, Volume: 400}

	s := &TurtleSoupStrategy{}
	s.Init(bars, map[string]float64{
		"swing_period":  3,
		"atr_period":    5,
		"disp_mult":     0.5,
		"body_ratio":    0.3,
		"ts_lookback":   25,
		"equal_tol":     0.2,
		"require_equal": 1,
	})

	sig := s.Signal(len(bars) - 1)
	if sig != BuySignal {
		t.Errorf("expected BuySignal on equal-lows sweep + displacement, got %d", sig)
	}
}
