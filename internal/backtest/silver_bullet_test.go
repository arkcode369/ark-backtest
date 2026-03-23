package backtest

import (
	"testing"
	"time"
	"trading-backtest-bot/internal/data"
)

// makeSBBars creates test bars with timestamps in a Silver Bullet window.
// The bars simulate: range → swing low → sweep below → bullish displacement + FVG → retrace.
func makeSBBars(baseHourEST int) []data.OHLCV {
	est := time.FixedZone("EST", -5*3600)
	// Base time at the target hour
	base := time.Date(2024, 6, 10, baseHourEST, 0, 0, 0, est)
	minute := func(m int) time.Time { return base.Add(time.Duration(m) * time.Minute) }

	// 30 bars of 1-minute data
	bars := make([]data.OHLCV, 30)

	// Bars 0-9: establish range with a swing low at bar 5 (low = 99.0)
	for i := 0; i < 10; i++ {
		bars[i] = data.OHLCV{
			Time: minute(i), Open: 100.5, High: 101.0, Low: 100.0, Close: 100.5, Volume: 100,
		}
	}
	// Swing low at bar 5: lowest in [0..10]
	bars[5] = data.OHLCV{Time: minute(5), Open: 100.0, High: 100.2, Low: 99.0, Close: 100.0, Volume: 100}

	// Bars 10-14: normal range (higher than swing low)
	for i := 10; i < 15; i++ {
		bars[i] = data.OHLCV{
			Time: minute(i), Open: 100.5, High: 101.0, Low: 100.0, Close: 100.5, Volume: 100,
		}
	}

	// Bar 15: Sweep below swing low (wick down to 98.5 but closes above 99.0)
	bars[15] = data.OHLCV{Time: minute(15), Open: 100.0, High: 100.2, Low: 98.5, Close: 99.5, Volume: 200}

	// Bars 16-17: Setup for bullish FVG
	// Bar 16: pre-FVG bar
	bars[16] = data.OHLCV{Time: minute(16), Open: 99.5, High: 100.0, Low: 99.3, Close: 99.8, Volume: 100}

	// Bar 17: Displacement candle (large bullish)
	bars[17] = data.OHLCV{Time: minute(17), Open: 99.8, High: 103.0, Low: 99.7, Close: 102.8, Volume: 300}

	// Bar 18: Creates FVG — bar[16].High=100.0, bar[18].Low should be > 100.0
	bars[18] = data.OHLCV{Time: minute(18), Open: 102.5, High: 103.0, Low: 100.5, Close: 102.8, Volume: 150}

	// Bars 19-28: drift around
	for i := 19; i < 29; i++ {
		bars[i] = data.OHLCV{
			Time: minute(i), Open: 102.5, High: 103.0, Low: 102.0, Close: 102.5, Volume: 100,
		}
	}

	// Bar 29: Retrace into FVG zone [100.0, 100.5]
	bars[29] = data.OHLCV{Time: minute(29), Open: 101.0, High: 101.5, Low: 100.2, Close: 100.8, Volume: 100}

	return bars
}

func TestSilverBulletBuyInWindow(t *testing.T) {
	// Test with NY AM window (10:00 EST)
	bars := makeSBBars(10)
	s := &SilverBulletStrategy{}
	s.Init(bars, map[string]float64{
		"swing_period": 3,
		"atr_period":   5,
		"disp_mult":    0.5,
		"body_ratio":   0.3,
		"sb_lookback":  25,
	})

	// Check that bar 29 produces a buy signal (retrace into FVG within window)
	sig := s.Signal(len(bars) - 1)
	if sig != BuySignal {
		t.Errorf("expected BuySignal at bar 29 in NY AM window, got %d", sig)
	}
}

func TestSilverBulletNoSignalOutsideWindow(t *testing.T) {
	// Same setup but at 08:00 EST (not a SB window)
	bars := makeSBBars(8)
	s := &SilverBulletStrategy{}
	s.Init(bars, map[string]float64{
		"swing_period": 3,
		"atr_period":   5,
		"disp_mult":    0.5,
		"body_ratio":   0.3,
		"sb_lookback":  25,
	})

	sig := s.Signal(len(bars) - 1)
	if sig != NoSignal {
		t.Errorf("expected NoSignal outside SB window (08:00 EST), got %d", sig)
	}
}

func TestSilverBulletLondonWindow(t *testing.T) {
	bars := makeSBBars(3)
	s := &SilverBulletStrategy{}
	s.Init(bars, map[string]float64{
		"swing_period": 3,
		"atr_period":   5,
		"disp_mult":    0.5,
		"body_ratio":   0.3,
		"sb_lookback":  25,
	})

	sig := s.Signal(len(bars) - 1)
	if sig != BuySignal {
		t.Errorf("expected BuySignal at bar 29 in London SB window, got %d", sig)
	}
}

func TestSilverBulletPMWindow(t *testing.T) {
	bars := makeSBBars(14)
	s := &SilverBulletStrategy{}
	s.Init(bars, map[string]float64{
		"swing_period": 3,
		"atr_period":   5,
		"disp_mult":    0.5,
		"body_ratio":   0.3,
		"sb_lookback":  25,
	})

	sig := s.Signal(len(bars) - 1)
	if sig != BuySignal {
		t.Errorf("expected BuySignal at bar 29 in NY PM window, got %d", sig)
	}
}

func TestSilverBulletRegistry(t *testing.T) {
	meta, ok := StrategyRegistry["silver_bullet"]
	if !ok {
		t.Fatal("silver_bullet not in registry")
	}
	s := meta.Factory()
	if s.Name() != "Silver Bullet" {
		t.Errorf("expected name 'Silver Bullet', got '%s'", s.Name())
	}
}
