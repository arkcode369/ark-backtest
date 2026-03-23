package backtest

import (
	"testing"
	"time"
	"trading-backtest-bot/internal/data"
)

func TestICTAdvanced_BuySignal_NoFilters(t *testing.T) {
	// Same synthetic data as TestICTMentorship_BuySignal
	// but with all advanced filters disabled
	n := 50
	bars := make([]data.OHLCV, n)
	set := func(i int, o, h, l, c float64) {
		bars[i] = data.OHLCV{
			Time:   time.Date(2024, 1, 1+i, 0, 0, 0, 0, time.UTC),
			Open:   o, High: h, Low: l, Close: c, Volume: 1000,
		}
	}

	// Bars 0-5: rising to swing high at bar 5
	set(0, 98, 99, 97, 98)
	set(1, 98, 100, 97, 99)
	set(2, 99, 101, 98, 100)
	set(3, 100, 103, 99, 102)
	set(4, 102, 105, 101, 104)
	set(5, 104, 108, 103, 106)

	set(6, 106, 107, 103, 104)
	set(7, 104, 105, 101, 102)
	set(8, 102, 103, 99, 100)
	set(9, 100, 101, 97, 98)
	set(10, 98, 99, 96, 97)

	set(11, 97, 98, 95, 96)
	set(12, 96, 97, 94, 95)
	set(13, 95, 96, 92, 93) // swing low: low=92

	set(14, 93, 95, 93, 94)
	set(15, 94, 96, 93, 95)
	set(16, 95, 97, 94, 96)
	set(17, 96, 98, 95, 97)
	set(18, 97, 99, 96, 98)

	set(19, 98, 99, 96, 97)
	set(20, 97, 98, 90, 94) // liquidity grab
	set(21, 94, 96, 93, 95)
	set(22, 95, 106, 95, 105) // displacement
	set(23, 105, 108, 104, 107) // FVG

	set(24, 107, 109, 106, 108) // MSS
	set(25, 108, 110, 107, 109)
	set(26, 109, 111, 108, 110)
	set(27, 110, 112, 108, 111)
	set(28, 111, 112, 109, 110)

	set(29, 110, 111, 107, 108)
	set(30, 108, 109, 105, 106)
	set(31, 106, 107, 103, 104) // enters FVG
	set(32, 104, 105, 101, 102)
	set(33, 102, 104, 99, 101)
	set(34, 101, 103, 98, 100)
	set(35, 100, 103, 97, 101)

	for i := 36; i < n; i++ {
		set(i, 101, 104, 99, 102)
	}

	strat := &ICTAdvancedStrategy{}
	params := map[string]float64{
		"swing_period":   5,
		"atr_period":     14,
		"disp_mult":      1.0,
		"body_ratio":     0.5,
		"fvg_fib_valid":  0,
		"lookback":       40,
		"htf_filter":     0, // disabled
		"killzone_only":  0, // disabled
		"cbdr_filter":    0, // disabled
		"smt_confluence": 0, // disabled
		"gap_awareness":  0, // disabled
	}
	strat.Init(bars, params)

	gotBuy := false
	for i := 20; i < n; i++ {
		sig := strat.Signal(i)
		if sig == BuySignal {
			gotBuy = true
			t.Logf("BuySignal at bar %d", i)
			break
		}
	}
	if !gotBuy {
		t.Error("expected BuySignal from ICT Advanced (no filters), got none")
	}
}

func TestICTAdvanced_KillZoneFilter(t *testing.T) {
	// Same setup but bars are timestamped OUTSIDE kill zones
	// With killzone_only=1, no signal should fire
	n := 50
	bars := make([]data.OHLCV, n)
	set := func(i int, o, h, l, c float64) {
		// 16:00 UTC = 11:00 EST — NOT a kill zone
		bars[i] = data.OHLCV{
			Time:   time.Date(2024, 1, 1+i, 16, 0, 0, 0, time.UTC),
			Open:   o, High: h, Low: l, Close: c, Volume: 1000,
		}
	}

	// Same bar data
	set(0, 98, 99, 97, 98)
	set(1, 98, 100, 97, 99)
	set(2, 99, 101, 98, 100)
	set(3, 100, 103, 99, 102)
	set(4, 102, 105, 101, 104)
	set(5, 104, 108, 103, 106)
	set(6, 106, 107, 103, 104)
	set(7, 104, 105, 101, 102)
	set(8, 102, 103, 99, 100)
	set(9, 100, 101, 97, 98)
	set(10, 98, 99, 96, 97)
	set(11, 97, 98, 95, 96)
	set(12, 96, 97, 94, 95)
	set(13, 95, 96, 92, 93)
	set(14, 93, 95, 93, 94)
	set(15, 94, 96, 93, 95)
	set(16, 95, 97, 94, 96)
	set(17, 96, 98, 95, 97)
	set(18, 97, 99, 96, 98)
	set(19, 98, 99, 96, 97)
	set(20, 97, 98, 90, 94)
	set(21, 94, 96, 93, 95)
	set(22, 95, 106, 95, 105)
	set(23, 105, 108, 104, 107)
	set(24, 107, 109, 106, 108)
	set(25, 108, 110, 107, 109)
	set(26, 109, 111, 108, 110)
	set(27, 110, 112, 108, 111)
	set(28, 111, 112, 109, 110)
	set(29, 110, 111, 107, 108)
	set(30, 108, 109, 105, 106)
	set(31, 106, 107, 103, 104)
	set(32, 104, 105, 101, 102)
	set(33, 102, 104, 99, 101)
	set(34, 101, 103, 98, 100)
	set(35, 100, 103, 97, 101)
	for i := 36; i < n; i++ {
		set(i, 101, 104, 99, 102)
	}

	strat := &ICTAdvancedStrategy{}
	params := map[string]float64{
		"swing_period":   5,
		"atr_period":     14,
		"disp_mult":      1.0,
		"body_ratio":     0.5,
		"fvg_fib_valid":  0,
		"lookback":       40,
		"htf_filter":     0,
		"killzone_only":  1, // ENABLED — should block all signals
		"cbdr_filter":    0,
		"smt_confluence": 0,
		"gap_awareness":  0,
	}
	strat.Init(bars, params)
	// Manually set sessions (since we're not going through Engine)
	strat.sessions = LabelSessions(bars)

	gotSignal := false
	for i := 20; i < n; i++ {
		sig := strat.Signal(i)
		if sig != NoSignal {
			gotSignal = true
			t.Logf("Unexpected signal at bar %d", i)
			break
		}
	}
	if gotSignal {
		t.Error("expected no signals with killzone_only=1 and bars outside kill zones")
	}
}

func TestICTAdvanced_RegistryExists(t *testing.T) {
	meta, ok := StrategyRegistry["ict_advanced"]
	if !ok {
		t.Fatal("ict_advanced not found in StrategyRegistry")
	}
	if meta.Name != "ICT Advanced" {
		t.Errorf("unexpected name: %s", meta.Name)
	}
	strat := meta.Factory()
	if strat.Name() != "ICT Advanced" {
		t.Errorf("factory produced wrong name: %s", strat.Name())
	}
}

func TestICTAdvanced_NoSignal_InsufficientBars(t *testing.T) {
	bars := makeBars([]float64{100, 101, 102}, 1.0)
	strat := &ICTAdvancedStrategy{}
	params := map[string]float64{
		"swing_period":   5,
		"atr_period":     14,
		"htf_filter":     0,
		"killzone_only":  0,
		"cbdr_filter":    0,
		"smt_confluence": 0,
		"gap_awareness":  0,
	}
	strat.Init(bars, params)
	for i := 0; i < len(bars); i++ {
		if sig := strat.Signal(i); sig != NoSignal {
			t.Errorf("expected NoSignal for insufficient bars at index %d, got %d", i, sig)
		}
	}
}
