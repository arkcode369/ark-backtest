package backtest

import (
	"math"
	"testing"
	"time"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── Judas Swing Tests ──

func TestDetectJudasSwings(t *testing.T) {
	est := estLoc
	bars := make([]data.OHLCV, 0)

	// Previous session (London 02:00-04:59): range 99-101
	for m := 0; m < 6; m++ {
		tt := time.Date(2024, 6, 10, 2+m/2, (m%2)*30, 0, 0, est)
		bars = append(bars, data.OHLCV{
			Time: tt, Open: 100.0, High: 101.0, Low: 99.0, Close: 100.0, Volume: 100,
		})
	}

	// NY AM session (07:00+): false high above 101, then reversal
	// Bar 6: sweeps above prior high
	tt := time.Date(2024, 6, 10, 7, 0, 0, 0, est)
	bars = append(bars, data.OHLCV{
		Time: tt, Open: 100.5, High: 102.0, Low: 100.0, Close: 100.8, Volume: 200,
	})

	// Bar 7: reverses below prior high
	tt = time.Date(2024, 6, 10, 7, 30, 0, 0, est)
	bars = append(bars, data.OHLCV{
		Time: tt, Open: 100.8, High: 101.0, Low: 99.5, Close: 99.8, Volume: 150,
	})

	// More bars to complete
	for m := 0; m < 4; m++ {
		tt = time.Date(2024, 6, 10, 8+m/2, (m%2)*30, 0, 0, est)
		bars = append(bars, data.OHLCV{
			Time: tt, Open: 99.5, High: 100.0, Low: 99.0, Close: 99.5, Volume: 100,
		})
	}

	sessions := LabelSessions(bars)
	swings := DetectJudasSwings(bars, sessions, 5)

	if len(swings) == 0 {
		t.Fatal("expected at least one Judas Swing detected")
	}

	// Should find bearish Judas (false high)
	found := false
	for _, js := range swings {
		if js.Direction == -1 {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected bearish Judas Swing (false high)")
	}
}

func TestDetectJudasSwingsEmpty(t *testing.T) {
	swings := DetectJudasSwings(nil, nil, 5)
	if len(swings) != 0 {
		t.Error("expected no swings for nil input")
	}
}

// ── Liquidity Void Tests ──

func TestDetectLiquidityVoids(t *testing.T) {
	base := time.Date(2024, 6, 10, 10, 0, 0, 0, time.UTC)
	minute := func(m int) time.Time { return base.Add(time.Duration(m) * time.Minute) }

	bars := make([]data.OHLCV, 20)
	for i := 0; i < 20; i++ {
		bars[i] = data.OHLCV{
			Time: minute(i), Open: 100.0, High: 100.5, Low: 99.5, Close: 100.0, Volume: 100,
		}
	}

	// Bar 10: large bullish candle (void-creating)
	bars[9] = data.OHLCV{Time: minute(9), Open: 100.0, High: 100.5, Low: 99.5, Close: 100.0, Volume: 100}
	bars[10] = data.OHLCV{Time: minute(10), Open: 100.0, High: 106.0, Low: 99.8, Close: 105.5, Volume: 500}
	bars[11] = data.OHLCV{Time: minute(11), Open: 105.0, High: 106.0, Low: 104.0, Close: 105.5, Volume: 100}

	atr := indicators.ATR(bars, 5)
	voids := DetectLiquidityVoids(bars, atr, 2.0)

	if len(voids) == 0 {
		t.Fatal("expected at least one liquidity void detected")
	}

	found := false
	for _, v := range voids {
		if v.Direction == 1 && v.Index == 10 {
			found = true
			if v.Top <= v.Bottom {
				t.Error("void top should be > bottom")
			}
			break
		}
	}
	if !found {
		t.Error("expected bullish liquidity void at bar 10")
	}
}

func TestDetectLiquidityVoidsEmpty(t *testing.T) {
	voids := DetectLiquidityVoids(nil, nil, 2.0)
	if len(voids) != 0 {
		t.Error("expected no voids for nil input")
	}
}

// ── IPDA State Machine Tests ──

func TestComputeIPDAState(t *testing.T) {
	base := time.Date(2024, 6, 10, 10, 0, 0, 0, time.UTC)
	minute := func(m int) time.Time { return base.Add(time.Duration(m) * time.Minute) }

	bars := make([]data.OHLCV, 30)
	// Tight range (consolidation)
	for i := 0; i < 15; i++ {
		bars[i] = data.OHLCV{
			Time: minute(i), Open: 100.0, High: 100.2, Low: 99.8, Close: 100.0, Volume: 100,
		}
	}
	// Expansion
	for i := 15; i < 25; i++ {
		bars[i] = data.OHLCV{
			Time: minute(i), Open: 100.0 + float64(i-15)*0.5, High: 100.5 + float64(i-15)*0.5,
			Low: 99.5 + float64(i-15)*0.5, Close: 100.0 + float64(i-15)*0.5, Volume: 200,
		}
	}
	// Large expansion bar
	bars[20] = data.OHLCV{Time: minute(20), Open: 102.0, High: 108.0, Low: 101.5, Close: 107.5, Volume: 500}

	// Normal after
	for i := 25; i < 30; i++ {
		bars[i] = data.OHLCV{
			Time: minute(i), Open: 107.0, High: 107.5, Low: 106.5, Close: 107.0, Volume: 100,
		}
	}

	atr := indicators.ATR(bars, 5)
	swingHighs := indicators.SwingHighs(bars, 3)
	swingLows := indicators.SwingLows(bars, 3)

	states := ComputeIPDAState(bars, atr, swingHighs, swingLows)
	if len(states) != 30 {
		t.Fatalf("expected 30 states, got %d", len(states))
	}

	// Bar 20 should be expansion (large range)
	if !math.IsNaN(atr[20]) && states[20] != IPDAExpansion {
		t.Errorf("expected Expansion state at bar 20, got %d", states[20])
	}
}

// ── Daily Bias 9-Step Tests ──

func TestDailyBias9Step(t *testing.T) {
	// Create minimal daily bars
	daily := make([]data.OHLCV, 5)
	for i := 0; i < 5; i++ {
		daily[i] = data.OHLCV{
			Time: time.Date(2024, 6, 10+i, 0, 0, 0, 0, time.UTC),
			Open: 100.0, High: 101.0, Low: 99.0, Close: 100.5, Volume: 1000,
		}
	}
	// Strong bullish: day 3 closes above day 2's high
	daily[3] = data.OHLCV{
		Time: time.Date(2024, 6, 13, 0, 0, 0, 0, time.UTC),
		Open: 100.0, High: 103.0, Low: 99.5, Close: 102.0, Volume: 1000,
	}

	// LTF bars all on day 3
	ltf := make([]data.OHLCV, 10)
	for i := 0; i < 10; i++ {
		ltf[i] = data.OHLCV{
			Time:  time.Date(2024, 6, 13, 9+i, 0, 0, 0, time.UTC),
			Open:  101.0,
			High:  102.0,
			Low:   100.0,
			Close: 101.5,
		}
	}

	htfIndex := data.AlignHTFToLTF(ltf, daily)
	sessions := LabelSessions(ltf)
	cbdrs := ComputeCBDR(ltf)
	ndogs := DetectNDOG(ltf)
	nwogs := DetectNWOG(daily)
	atr := indicators.ATR(ltf, 5)

	bias := DailyBias9Step(ltf, daily, htfIndex, sessions, cbdrs, ndogs, nwogs, atr)

	if len(bias) != 10 {
		t.Fatalf("expected 10 bias values, got %d", len(bias))
	}

	// Most bars should have positive bias (bullish day)
	posCount := 0
	for _, b := range bias {
		if b > 0 {
			posCount++
		}
	}
	if posCount == 0 {
		t.Error("expected at least some positive bias values on a bullish day")
	}
}
