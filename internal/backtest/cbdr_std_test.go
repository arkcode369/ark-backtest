package backtest

import (
	"testing"
	"time"
	"trading-backtest-bot/internal/data"
)

func makeCBDRBars() []data.OHLCV {
	est := estLoc
	bars := make([]data.OHLCV, 0)

	// Asian session bars (19:00-23:59 EST on June 9) — range 99.5-100.5 (CBDR range = 1.0)
	// Using 19:00 start so that the CBDR date computation (which uses Truncate in UTC)
	// correctly maps to the same calendar date as the London/NY bars.
	for m := 0; m < 10; m++ {
		t := time.Date(2024, 6, 9, 19+m/2, (m%2)*30, 0, 0, est)
		bars = append(bars, data.OHLCV{
			Time: t, Open: 100.0, High: 100.5, Low: 99.5, Close: 100.0, Volume: 100,
		})
	}

	// Bars during London (02:00-05:00 EST on June 10) — normal
	for m := 0; m < 6; m++ {
		t := time.Date(2024, 6, 10, 2+m/2, (m%2)*30, 0, 0, est)
		bars = append(bars, data.OHLCV{
			Time: t, Open: 100.0, High: 100.5, Low: 99.5, Close: 100.0, Volume: 100,
		})
	}

	// NY bars — price moves to STD 1 below (99.5 - 1.0 = 98.5)
	for m := 0; m < 8; m++ {
		t := time.Date(2024, 6, 10, 7+m/2, (m%2)*30, 0, 0, est)
		p := 99.5 - float64(m)*0.15
		bars = append(bars, data.OHLCV{
			Time: t, Open: p + 0.1, High: p + 0.3, Low: p - 0.2, Close: p, Volume: 100,
		})
	}

	// Final bar: price closes below cbdr.Low (99.5) at STD ~1 down, with bullish displacement.
	// Close=98.5 → stdDown = (99.5 - 98.5) / 1.0 = 1.0, within [0.5, 3.0].
	// body = 98.5 - 97.5 = 1.0 (bullish), range = 99.0 - 97.3 = 1.7
	// body/range = 1.0/1.7 ≈ 0.59 >= 0.3, body(1.0) >= dispMult*ATR(~0.3) ✓
	t := time.Date(2024, 6, 10, 11, 0, 0, 0, est)
	bars = append(bars, data.OHLCV{
		Time: t, Open: 97.5, High: 99.0, Low: 97.3, Close: 98.5, Volume: 300,
	})

	return bars
}

func TestCBDRSTDBullish(t *testing.T) {
	bars := makeCBDRBars()
	s := &CBDRSTDStrategy{}
	s.Init(bars, map[string]float64{
		"swing_period": 3,
		"atr_period":   5,
		"disp_mult":    0.3,
		"body_ratio":   0.3,
		"min_std":      0.5,
		"max_std":      3.0,
	})

	sig := s.Signal(len(bars) - 1)
	if sig != BuySignal {
		t.Errorf("expected BuySignal at CBDR STD level, got %d", sig)
	}
}

func TestCBDRSTDNoSignalInAsian(t *testing.T) {
	bars := makeCBDRBars()
	s := &CBDRSTDStrategy{}
	s.Init(bars, map[string]float64{
		"swing_period": 3,
		"atr_period":   5,
		"disp_mult":    0.3,
		"body_ratio":   0.3,
		"min_std":      0.5,
		"max_std":      3.0,
	})

	// Asian bars should produce no signal
	sig := s.Signal(5)
	if sig != NoSignal {
		t.Errorf("expected NoSignal during Asian session, got %d", sig)
	}
}

func TestCBDRSTDRegistry(t *testing.T) {
	meta, ok := StrategyRegistry["cbdr_std"]
	if !ok {
		t.Fatal("cbdr_std not in registry")
	}
	s := meta.Factory()
	if s.Name() != "CBDR STD" {
		t.Errorf("expected name 'CBDR STD', got '%s'", s.Name())
	}
}
