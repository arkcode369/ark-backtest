package backtest

import (
	"testing"
	"time"
	"trading-backtest-bot/internal/data"
)

func makeAMDBars() []data.OHLCV {
	est := estLoc
	bars := make([]data.OHLCV, 0)

	// Asian session (18:00-23:59 EST previous day) — tight range
	for m := 0; m < 12; m++ {
		t := time.Date(2024, 6, 9, 18+m/2, (m%2)*30, 0, 0, est)
		bars = append(bars, data.OHLCV{
			Time: t, Open: 100.0, High: 100.5, Low: 99.5, Close: 100.0, Volume: 100,
		})
	}
	// Asian range: high=100.5, low=99.5

	// London session (02:00-05:00 EST) — breaks below Asian low (manipulation)
	for m := 0; m < 6; m++ {
		t := time.Date(2024, 6, 10, 2+m/2, (m%2)*30, 0, 0, est)
		if m == 2 {
			// Break below Asian low
			bars = append(bars, data.OHLCV{
				Time: t, Open: 99.5, High: 99.6, Low: 98.5, Close: 99.0, Volume: 200,
			})
		} else {
			bars = append(bars, data.OHLCV{
				Time: t, Open: 99.5, High: 100.0, Low: 99.3, Close: 99.5, Volume: 100,
			})
		}
	}

	// Gap between London and NY
	for m := 0; m < 4; m++ {
		t := time.Date(2024, 6, 10, 5+m/2, (m%2)*30, 0, 0, est)
		bars = append(bars, data.OHLCV{
			Time: t, Open: 99.5, High: 100.0, Low: 99.3, Close: 99.5, Volume: 100,
		})
	}

	// NY session (07:00-10:00 EST) — bullish displacement (distribution opposite to manipulation)
	for m := 0; m < 5; m++ {
		t := time.Date(2024, 6, 10, 7+m/2, (m%2)*30, 0, 0, est)
		bars = append(bars, data.OHLCV{
			Time: t, Open: 99.5, High: 100.0, Low: 99.3, Close: 99.8, Volume: 100,
		})
	}
	// NY displacement bar (large bullish)
	t := time.Date(2024, 6, 10, 9, 30, 0, 0, est)
	bars = append(bars, data.OHLCV{
		Time: t, Open: 99.8, High: 103.0, Low: 99.7, Close: 102.8, Volume: 400,
	})

	return bars
}

func TestAMDSessionBullish(t *testing.T) {
	bars := makeAMDBars()
	s := &AMDSessionStrategy{}
	s.Init(bars, map[string]float64{
		"swing_period": 3,
		"atr_period":   5,
		"disp_mult":    0.5,
		"body_ratio":   0.3,
	})

	sig := s.Signal(len(bars) - 1)
	if sig != BuySignal {
		t.Errorf("expected BuySignal for AMD bullish (London broke below, NY bullish disp), got %d", sig)
	}
}

func TestAMDSessionNoSignalDuringAsian(t *testing.T) {
	bars := makeAMDBars()
	s := &AMDSessionStrategy{}
	s.Init(bars, map[string]float64{
		"swing_period": 3,
		"atr_period":   5,
		"disp_mult":    0.5,
		"body_ratio":   0.3,
	})

	// Bar 5 is in Asian session — should have no signal
	sig := s.Signal(5)
	if sig != NoSignal {
		t.Errorf("expected NoSignal during Asian session, got %d", sig)
	}
}

func TestAMDSessionRegistry(t *testing.T) {
	meta, ok := StrategyRegistry["amd_session"]
	if !ok {
		t.Fatal("amd_session not in registry")
	}
	s := meta.Factory()
	if s.Name() != "AMD Session" {
		t.Errorf("expected name 'AMD Session', got '%s'", s.Name())
	}
}
