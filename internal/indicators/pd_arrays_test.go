package indicators

import (
	"math"
	"testing"
	"time"
	"trading-backtest-bot/internal/data"
)

// makeBar is a helper to build an OHLCV bar quickly.
func makeBar(o, h, l, c, vol float64) data.OHLCV {
	return data.OHLCV{
		Time:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Open:   o,
		High:   h,
		Low:    l,
		Close:  c,
		Volume: vol,
	}
}

// ── 1. TestDetectFVGs ──────────────────────────────────────────────────────

func TestDetectFVGs(t *testing.T) {
	// Bullish FVG: bar[0].High=100, bar[1] is big bullish candle, bar[2].Low=103
	// Gap: bar[2].Low (103) > bar[0].High (100) → bullish FVG zone [100, 103]
	bars := []data.OHLCV{
		makeBar(95, 100, 94, 98, 1000),   // bar 0: High=100
		makeBar(99, 108, 98, 107, 2000),   // bar 1: big bullish candle
		makeBar(105, 110, 103, 109, 1500), // bar 2: Low=103
	}

	zones := detectFVGs(bars)
	foundBullish := false
	for _, z := range zones {
		if z.Type == PDFairValueGap && z.Direction == Bullish {
			foundBullish = true
			if !almostEqual(z.Bottom, 100.0, tol) {
				t.Errorf("Bullish FVG bottom expected 100, got %f", z.Bottom)
			}
			if !almostEqual(z.Top, 103.0, tol) {
				t.Errorf("Bullish FVG top expected 103, got %f", z.Top)
			}
		}
	}
	if !foundBullish {
		t.Error("Expected a bullish FVG to be detected")
	}

	// Bearish FVG: bar[0].Low=105, bar[1] is big bearish candle, bar[2].High=102
	// Gap: bar[2].High (102) < bar[0].Low (105) → bearish FVG zone [105, 102]
	bars2 := []data.OHLCV{
		makeBar(106, 108, 105, 107, 1000), // bar 0: Low=105
		makeBar(104, 106, 95, 96, 2000),   // bar 1: big bearish candle
		makeBar(97, 102, 94, 95, 1500),    // bar 2: High=102
	}

	zones2 := detectFVGs(bars2)
	foundBearish := false
	for _, z := range zones2 {
		if z.Type == PDFairValueGap && z.Direction == Bearish {
			foundBearish = true
			if !almostEqual(z.Top, 105.0, tol) {
				t.Errorf("Bearish FVG top expected 105, got %f", z.Top)
			}
			if !almostEqual(z.Bottom, 102.0, tol) {
				t.Errorf("Bearish FVG bottom expected 102, got %f", z.Bottom)
			}
		}
	}
	if !foundBearish {
		t.Error("Expected a bearish FVG to be detected")
	}
}

// ── 2. TestDetectOrderBlocks ───────────────────────────────────────────────

func TestDetectOrderBlocks(t *testing.T) {
	// We need ATR values. We'll create a simple scenario:
	// bar[0]: bearish candle (close < open)
	// bar[1]: large bullish impulse candle with range > 1.5 * ATR
	// ATR[1] = 2.0, so impulse range must be > 3.0.

	bars := []data.OHLCV{
		makeBar(102, 103, 99, 100, 1000), // bearish candle: open=102, close=100, low=99
		makeBar(101, 107, 100, 106, 2000), // bullish impulse: range = 107-100 = 7 > 1.5*2=3
	}

	atr := []float64{2.0, 2.0}

	zones := detectOrderBlocks(bars, atr, 1.5)
	foundBullishOB := false
	for _, z := range zones {
		if z.Type == PDOrderBlock && z.Direction == Bullish {
			foundBullishOB = true
			// Bullish OB: top = bars[0].Open=102, bottom = bars[0].Low=99
			if !almostEqual(z.Top, 102.0, tol) {
				t.Errorf("Bullish OB top expected 102, got %f", z.Top)
			}
			if !almostEqual(z.Bottom, 99.0, tol) {
				t.Errorf("Bullish OB bottom expected 99, got %f", z.Bottom)
			}
		}
	}
	if !foundBullishOB {
		t.Error("Expected a bullish order block to be detected")
	}
}

// ── 3. TestDetectVolumeImbalance ───────────────────────────────────────────

func TestDetectVolumeImbalance(t *testing.T) {
	// Bullish VI: bar[1].Open > bar[0].Close (gap up)
	bars := []data.OHLCV{
		makeBar(100, 102, 99, 101, 1000),  // close=101
		makeBar(103, 106, 102, 105, 1500), // open=103 > close[0]=101 → gap up
	}

	zones := detectVolumeImbalances(bars)
	foundBullishVI := false
	for _, z := range zones {
		if z.Type == PDVolumeImbalance && z.Direction == Bullish {
			foundBullishVI = true
			// Zone: top=103 (open[1]), bottom=101 (close[0])
			if !almostEqual(z.Top, 103.0, tol) {
				t.Errorf("Bullish VI top expected 103, got %f", z.Top)
			}
			if !almostEqual(z.Bottom, 101.0, tol) {
				t.Errorf("Bullish VI bottom expected 101, got %f", z.Bottom)
			}
		}
	}
	if !foundBullishVI {
		t.Error("Expected a bullish volume imbalance to be detected")
	}
}

// ── 4. TestDetectRejectionBlock ────────────────────────────────────────────

func TestDetectRejectionBlock(t *testing.T) {
	// Create a candle with a very long lower wick at a swing low position.
	// Wick > 2x body. Body: |close - open|. Lower wick: min(open,close) - low.
	// open=101, close=102, body=1. low=95, so lower wick = 101-95 = 6 > 2*1=2.

	n := 11 // need enough bars for swing detection (period=5)
	bars := make([]data.OHLCV, n)
	// Build bars descending to a low at bar 5 (swing low), then ascending
	for i := 0; i < n; i++ {
		price := 110.0 - float64(i)*2
		if i > 5 {
			price = 100.0 + float64(i-5)*2
		}
		bars[i] = makeBar(price, price+1, price-1, price+0.5, 1000)
	}
	// Override bar 5 to have a very long lower wick
	bars[5] = makeBar(101, 103, 93, 102, 2000) // body=1, lower wick=101-93=8 > 2*1

	swingHighs := SwingHighs(bars, 5)
	swingLows := SwingLows(bars, 5)

	zones := detectRejectionBlocks(bars, swingHighs, swingLows, 2.0)
	foundBullishRB := false
	for _, z := range zones {
		if z.Type == PDRejectionBlock && z.Direction == Bullish && z.Index == 5 {
			foundBullishRB = true
			// Zone: bottom=93, top=min(101,102)=101
			if !almostEqual(z.Bottom, 93.0, tol) {
				t.Errorf("Rejection block bottom expected 93, got %f", z.Bottom)
			}
			if !almostEqual(z.Top, 101.0, tol) {
				t.Errorf("Rejection block top expected 101, got %f", z.Top)
			}
		}
	}
	if !foundBullishRB {
		t.Error("Expected a bullish rejection block at bar 5")
	}
}

// ── 5. TestComputePremiumDiscount ──────────────────────────────────────────

func TestComputePremiumDiscount(t *testing.T) {
	// 20 bars with clear swing high=110, swing low=90
	bars := make([]data.OHLCV, 20)
	for i := 0; i < 20; i++ {
		// Oscillate between 90 and 110
		price := 100.0
		if i == 5 {
			price = 90.0 // swing low
		} else if i == 15 {
			price = 110.0 // swing high
		}
		bars[i] = makeBar(price-1, price+1, price-2, price, 1000)
	}
	// Override to ensure exact swing high=110 and swing low=88 -> fix to exactly 110 and 90
	bars[5] = makeBar(91, 91, 90, 91, 1000)   // Low=90
	bars[15] = makeBar(109, 110, 109, 109, 1000) // High=110

	res := ComputePremiumDiscount(bars, 50)

	rng := 110.0 - 90.0 // = 20
	expectedEq := 90.0 + rng*0.5  // 100
	expectedOTEDisc := 90.0 + rng*0.21 // 94.2
	expectedFibBuy := 90.0 + rng*0.38 // 97.6
	expectedFibSell := 90.0 + rng*0.62 // 102.4
	expectedOTEPrem := 90.0 + rng*0.79 // 105.8

	if !almostEqual(res.SwingHigh, 110.0, tol) {
		t.Errorf("SwingHigh expected 110, got %f", res.SwingHigh)
	}
	if !almostEqual(res.SwingLow, 90.0, tol) {
		t.Errorf("SwingLow expected 90, got %f", res.SwingLow)
	}
	if !almostEqual(res.Equilibrium, expectedEq, 0.01) {
		t.Errorf("Equilibrium expected %f, got %f", expectedEq, res.Equilibrium)
	}
	if !almostEqual(res.OTEDiscount, expectedOTEDisc, 0.01) {
		t.Errorf("OTEDiscount expected %f, got %f", expectedOTEDisc, res.OTEDiscount)
	}
	if !almostEqual(res.FibBuy, expectedFibBuy, 0.01) {
		t.Errorf("FibBuy expected %f, got %f", expectedFibBuy, res.FibBuy)
	}
	if !almostEqual(res.FibSell, expectedFibSell, 0.01) {
		t.Errorf("FibSell expected %f, got %f", expectedFibSell, res.FibSell)
	}
	if !almostEqual(res.OTEPremium, expectedOTEPrem, 0.01) {
		t.Errorf("OTEPremium expected %f, got %f", expectedOTEPrem, res.OTEPremium)
	}

	// Verify per-bar flags for known bars
	// Bar 5 close=91, which is < equilibrium=100 → InDiscount
	if !res.InDiscount[5] {
		t.Error("Bar 5 (close=91) should be in discount zone")
	}
	// Bar 15 close=109, which is > equilibrium=100 → InPremium
	if !res.InPremium[15] {
		t.Error("Bar 15 (close=109) should be in premium zone")
	}
}

// ── 6. TestDetectAllPDArrays ──────────────────────────────────────────────

func TestDetectAllPDArrays(t *testing.T) {
	// Build 30 bars with various patterns that should trigger detections
	n := 30
	bars := make([]data.OHLCV, n)

	// Base price movement: gentle uptrend with some gaps
	for i := 0; i < n; i++ {
		base := 100.0 + float64(i)*0.5
		bars[i] = makeBar(base, base+2, base-1, base+1, 1000)
	}

	// Inject a bullish FVG at bars 10-12: bar[10].High < bar[12].Low
	bars[10] = makeBar(104, 105, 103, 104.5, 1000) // High=105
	bars[11] = makeBar(105, 115, 104, 114, 3000)    // big bullish candle
	bars[12] = makeBar(112, 116, 108, 115, 2000)    // Low=108 > bar[10].High=105 → FVG

	// Inject a volume imbalance at bar 20: open[20] > close[19]
	bars[19] = makeBar(109, 111, 108, 109.5, 1000) // close=109.5
	bars[20] = makeBar(112, 114, 111, 113, 1500)   // open=112 > 109.5 → gap up VI

	// Inject an order block at bars 24-25: bearish candle followed by bullish impulse
	bars[24] = makeBar(114, 115, 112, 113, 1000) // bearish: open=114 > close=113
	bars[25] = makeBar(113, 125, 112, 124, 5000) // huge bullish impulse: range=125-112=13

	params := DefaultPDParams()
	zones := DetectAllPDArrays(bars, params)

	if len(zones) == 0 {
		t.Error("Expected DetectAllPDArrays to return a non-empty slice")
	}

	// Verify at least one FVG was found
	foundFVG := false
	for _, z := range zones {
		if z.Type == PDFairValueGap {
			foundFVG = true
			break
		}
	}
	if !foundFVG {
		t.Error("Expected at least one FVG in DetectAllPDArrays results")
	}

	// Verify zones are sorted by index
	for i := 1; i < len(zones); i++ {
		if zones[i].Index < zones[i-1].Index {
			t.Errorf("Zones not sorted by index: zone[%d].Index=%d < zone[%d].Index=%d",
				i, zones[i].Index, i-1, zones[i-1].Index)
		}
	}

	// Suppress unused import warning
	_ = math.NaN()
}
