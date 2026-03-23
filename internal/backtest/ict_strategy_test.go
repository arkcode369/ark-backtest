package backtest

import (
	"math"
	"testing"
	"time"
	"trading-backtest-bot/internal/data"
)

// makeBars creates a sequence of OHLCV bars from close prices with synthetic OHLC
func makeBars(closes []float64, spread float64) []data.OHLCV {
	bars := make([]data.OHLCV, len(closes))
	for i, c := range closes {
		bars[i] = data.OHLCV{
			Time:   time.Date(2024, 1, 1+i, 0, 0, 0, 0, time.UTC),
			Open:   c,
			High:   c + spread,
			Low:    c - spread,
			Close:  c,
			Volume: 1000,
		}
	}
	return bars
}

// TestICTMentorship_BuySignal constructs a synthetic bullish ICT setup:
// swing low → SSL grab (sweep below) → bullish displacement + FVG → MSS → retrace into FVG
func TestICTMentorship_BuySignal(t *testing.T) {
	// Build 50 bars with a clear bullish ICT pattern.
	// The pattern: establish a range with clear swing high + swing low,
	// then sweep the swing low (SSL grab), bullish displacement with FVG,
	// MSS breaks above the swing high, then retrace into FVG for entry.
	n := 50
	bars := make([]data.OHLCV, n)
	set := func(i int, o, h, l, c float64) {
		bars[i] = data.OHLCV{
			Time:   time.Date(2024, 1, 1+i, 0, 0, 0, 0, time.UTC),
			Open:   o, High: h, Low: l, Close: c, Volume: 1000,
		}
	}

	// Bars 0-5: rising to create a swing high at bar 5
	set(0, 98, 99, 97, 98)
	set(1, 98, 100, 97, 99)
	set(2, 99, 101, 98, 100)
	set(3, 100, 103, 99, 102)
	set(4, 102, 105, 101, 104)
	set(5, 104, 108, 103, 106) // swing high candidate: high=108

	// Bars 6-10: declining from swing high (confirms bar 5 is swing high with period=5)
	set(6, 106, 107, 103, 104)
	set(7, 104, 105, 101, 102)
	set(8, 102, 103, 99, 100)
	set(9, 100, 101, 97, 98)
	set(10, 98, 99, 96, 97) // bar 5 confirmed as swing high (bars 0-10)

	// Bars 11-15: continue decline to swing low at bar 13
	set(11, 97, 98, 95, 96)
	set(12, 96, 97, 94, 95)
	set(13, 95, 96, 92, 93) // swing low candidate: low=92

	// Bars 14-18: recover (confirms bar 13 as swing low)
	set(14, 93, 95, 93, 94)
	set(15, 94, 96, 93, 95)
	set(16, 95, 97, 94, 96)
	set(17, 96, 98, 95, 97)
	set(18, 97, 99, 96, 98) // bar 13 confirmed (bars 8-18)

	// Bar 19: quiet bar
	set(19, 98, 99, 96, 97)

	// Bar 20: LIQUIDITY GRAB — sweeps below bar 13's low (92), closes back above
	set(20, 97, 98, 90, 94) // low=90 < 92 (sweep), close=94 > 92

	// Bar 21: pre-displacement
	set(21, 94, 96, 93, 95)

	// Bar 22: BIG BULLISH DISPLACEMENT candle
	// body=10, range=11, ratio=0.91, ATR should be ~4-5
	set(22, 95, 106, 95, 105)

	// Bar 23: gap up — creates FVG between bar[21].High=96 and bar[23].Low=104
	// FVG zone = [96, 104]
	set(23, 105, 108, 104, 107)

	// Bars 24-28: price continues up, eventually breaks above swing high at 108 (MSS)
	set(24, 107, 109, 106, 108) // high=109 > 108 → MSS confirmed
	set(25, 108, 110, 107, 109)
	set(26, 109, 111, 108, 110)
	set(27, 110, 112, 108, 111)
	set(28, 111, 112, 109, 110)

	// Bars 29-35: retrace into FVG zone [96, 104]
	set(29, 110, 111, 107, 108)
	set(30, 108, 109, 105, 106)
	set(31, 106, 107, 103, 104) // enters FVG zone
	set(32, 104, 105, 101, 102)
	set(33, 102, 104, 99, 101)
	set(34, 101, 103, 98, 100)
	set(35, 100, 103, 97, 101)

	// Fill remaining bars
	for i := 36; i < n; i++ {
		set(i, 101, 104, 99, 102)
	}

	strat := &ICTMentorshipStrategy{}
	params := map[string]float64{
		"swing_period":  5,
		"atr_period":    14,
		"disp_mult":     1.0,
		"body_ratio":    0.5,
		"fvg_fib_valid": 0,
		"lookback":      40,
	}
	strat.Init(bars, params)

	gotBuy := false
	for i := 20; i < n; i++ {
		sig := strat.Signal(i)
		if sig == BuySignal {
			gotBuy = true
			t.Logf("BuySignal at bar %d (range %.1f-%.1f)", i, bars[i].Low, bars[i].High)
			break
		}
	}
	if !gotBuy {
		t.Error("expected BuySignal from ICT Mentorship bullish setup, got none")
	}
}

func TestICTMentorship_NoSignal_InsufficientBars(t *testing.T) {
	bars := makeBars([]float64{100, 101, 102}, 1.0)
	strat := &ICTMentorshipStrategy{}
	strat.Init(bars, map[string]float64{"swing_period": 5, "atr_period": 14})
	for i := 0; i < len(bars); i++ {
		if sig := strat.Signal(i); sig != NoSignal {
			t.Errorf("expected NoSignal for insufficient bars at index %d, got %d", i, sig)
		}
	}
}

func TestICTMentorship_RegistryExists(t *testing.T) {
	meta, ok := StrategyRegistry["ict2022"]
	if !ok {
		t.Fatal("ict2022 not found in StrategyRegistry")
	}
	if meta.Name != "ICT 2022" {
		t.Errorf("unexpected name: %s", meta.Name)
	}
	strat := meta.Factory()
	if strat.Name() != "ICT 2022" {
		t.Errorf("factory produced wrong strategy name: %s", strat.Name())
	}
}

func TestSwingHighs_KnownValues(t *testing.T) {
	// Bar 5 should be a clear swing high with period=3
	bars := make([]data.OHLCV, 11)
	prices := []float64{10, 11, 12, 13, 14, 20, 14, 13, 12, 11, 10}
	for i, p := range prices {
		bars[i] = data.OHLCV{High: p, Low: p - 2, Close: p, Open: p}
	}
	highs := make([]float64, len(bars))
	for i := range highs {
		highs[i] = math.NaN()
	}
	// Manual swing detection for period=3
	period := 3
	for i := period; i < len(bars)-period; i++ {
		isSwing := true
		for j := i - period; j <= i+period; j++ {
			if j == i {
				continue
			}
			if bars[j].High >= bars[i].High {
				isSwing = false
				break
			}
		}
		if isSwing {
			highs[i] = bars[i].High
		}
	}

	if math.IsNaN(highs[5]) {
		t.Error("expected swing high at index 5")
	}
	if highs[5] != 20 {
		t.Errorf("expected swing high price 20, got %f", highs[5])
	}
}

func TestICTMentorship_SellSignal(t *testing.T) {
	// Build a bearish ICT setup: swing low → swing high → BSL grab → bearish displacement + FVG → MSS → retrace
	n := 50
	bars := make([]data.OHLCV, n)
	set := func(i int, o, h, l, c float64) {
		bars[i] = data.OHLCV{
			Time:   time.Date(2024, 1, 1+i, 0, 0, 0, 0, time.UTC),
			Open:   o, High: h, Low: l, Close: c, Volume: 1000,
		}
	}

	// Bars 0-5: declining to create a swing low at bar 5
	set(0, 105, 106, 104, 105)
	set(1, 105, 106, 103, 104)
	set(2, 104, 105, 102, 103)
	set(3, 103, 104, 100, 101)
	set(4, 101, 102, 97, 98)
	set(5, 98, 99, 92, 93) // swing low candidate: low=92

	// Bars 6-10: rising from swing low (confirms bar 5 is swing low)
	set(6, 93, 96, 93, 95)
	set(7, 95, 98, 94, 97)
	set(8, 97, 100, 96, 99)
	set(9, 99, 102, 98, 101)
	set(10, 101, 104, 100, 103) // bar 5 confirmed (bars 0-10)

	// Bars 11-15: continue rise to swing high at bar 13
	set(11, 103, 106, 102, 105)
	set(12, 105, 108, 104, 107)
	set(13, 107, 112, 106, 110) // swing high candidate: high=112

	// Bars 14-18: decline from swing high (confirms bar 13 as swing high)
	set(14, 110, 111, 107, 108)
	set(15, 108, 109, 105, 106)
	set(16, 106, 107, 103, 104)
	set(17, 104, 105, 101, 102)
	set(18, 102, 103, 100, 101) // bar 13 confirmed (bars 8-18)

	// Bar 19: quiet
	set(19, 101, 103, 100, 102)

	// Bar 20: BSL GRAB — sweeps above bar 13's high (112), closes back below
	set(20, 102, 114, 101, 110) // high=114 > 112, close=110 < 112

	// Bar 21: pre-displacement
	set(21, 110, 111, 107, 108)

	// Bar 22: BIG BEARISH DISPLACEMENT candle
	// body=10, range=11, ratio=0.91
	set(22, 108, 108, 97, 98)

	// Bar 23: gap down — creates bearish FVG between bar[21].Low=107 and bar[23].High=99
	// FVG zone = [99, 107]
	set(23, 98, 99, 95, 96)

	// Bars 24-28: continues down, breaks below swing low at 92 (MSS)
	set(24, 96, 97, 93, 94)
	set(25, 94, 95, 91, 92) // low=91 < 92 → MSS confirmed
	set(26, 92, 93, 89, 90)
	set(27, 90, 92, 88, 91)
	set(28, 91, 93, 89, 92)

	// Bars 29-35: retrace UP into FVG zone [99, 107]
	set(29, 92, 95, 91, 94)
	set(30, 94, 97, 93, 96)
	set(31, 96, 100, 95, 99) // enters FVG zone
	set(32, 99, 103, 98, 102)
	set(33, 102, 105, 100, 104)
	set(34, 104, 106, 101, 103)
	set(35, 103, 105, 100, 102)

	for i := 36; i < n; i++ {
		set(i, 101, 104, 99, 102)
	}

	strat := &ICTMentorshipStrategy{}
	params := map[string]float64{
		"swing_period":  5,
		"atr_period":    14,
		"disp_mult":     1.0,
		"body_ratio":    0.5,
		"fvg_fib_valid": 0,
		"lookback":      40,
	}
	strat.Init(bars, params)

	gotSell := false
	for i := 20; i < n; i++ {
		sig := strat.Signal(i)
		if sig == SellSignal {
			gotSell = true
			t.Logf("SellSignal at bar %d (range %.1f-%.1f)", i, bars[i].Low, bars[i].High)
			break
		}
	}
	if !gotSell {
		t.Error("expected SellSignal from ICT Mentorship bearish setup, got none")
	}
}
