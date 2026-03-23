package backtest

import (
	"math"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── Megatrade Strategy ──────────────────────────────────────────────────
//
// Multi-week swing trades based on large-timeframe structural breaks.
// Applies the MSS/CHoCH concept on a much larger scale using a wide
// swing_period (e.g., 10-20 bars on daily data) to capture quarterly/
// monthly structural shifts.
//
// Implementation:
//  1. Compute "weekly-level" swings using a large swing_period on daily
//     bars.
//  2. Track structure via sequences of swing highs and lows:
//     - HH + HL = bullish structure
//     - LH + LL = bearish structure
//  3. When mega-structure shifts (first LH after a series of HH, or
//     first HL after a series of LL), enter on pullback with
//     displacement confirmation.
//  4. Displacement must be significant (large disp_mult).
//
// Parameters:
//   swing_period    – swing detection period, large for mega swings (default 10)
//   atr_period      – ATR period (default 20)
//   disp_mult       – displacement ATR multiple (default 2.0)
//   body_ratio      – min body/range ratio (default 0.5)
//   min_swing_count – minimum consecutive same-direction swings before a
//                     shift qualifies (default 3)
//   lookback        – bars to scan for structure analysis (default 100)

type MegatradeStrategy struct {
	bars           []data.OHLCV
	atr            []float64
	swingHighs     []float64
	swingLows      []float64
	swingPeriod    int
	dispMult       float64
	bodyRatio      float64
	minSwingCount  int
	lookback       int
	lastSigBar     int
}

func (s *MegatradeStrategy) Name() string { return "Megatrade" }
func (s *MegatradeStrategy) Description() string {
	return "Multi-week swing trades: enter on mega-structure shift (large-TF CHoCH) with significant displacement"
}

func (s *MegatradeStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	s.swingPeriod = int(getParam(params, "swing_period", 10))
	atrPeriod := int(getParam(params, "atr_period", 20))
	s.dispMult = getParam(params, "disp_mult", 2.0)
	s.bodyRatio = getParam(params, "body_ratio", 0.5)
	s.minSwingCount = int(getParam(params, "min_swing_count", 3))
	s.lookback = int(getParam(params, "lookback", 100))
	if s.lookback < s.swingPeriod*4 {
		s.lookback = s.swingPeriod * 4
	}
	s.lastSigBar = -s.lookback

	s.atr = indicators.ATR(bars, atrPeriod)
	s.swingHighs = indicators.SwingHighs(bars, s.swingPeriod)
	s.swingLows = indicators.SwingLows(bars, s.swingPeriod)
}

// megaSwingPoint holds a confirmed mega swing's index and price.
type megaSwingPoint struct {
	idx   int
	price float64
}

func (s *MegatradeStrategy) collectHighs(start, end int) []megaSwingPoint {
	var pts []megaSwingPoint
	for j := start; j < end; j++ {
		if !math.IsNaN(s.swingHighs[j]) {
			pts = append(pts, megaSwingPoint{idx: j, price: s.swingHighs[j]})
		}
	}
	return pts
}

func (s *MegatradeStrategy) collectLows(start, end int) []megaSwingPoint {
	var pts []megaSwingPoint
	for j := start; j < end; j++ {
		if !math.IsNaN(s.swingLows[j]) {
			pts = append(pts, megaSwingPoint{idx: j, price: s.swingLows[j]})
		}
	}
	return pts
}

func (s *MegatradeStrategy) Signal(i int) SignalType {
	if i < s.swingPeriod*2+3 || math.IsNaN(s.atr[i]) || s.atr[i] == 0 {
		return NoSignal
	}
	if i-s.lastSigBar < s.swingPeriod*2 {
		return NoSignal
	}

	if s.checkBullishShift(i) {
		s.lastSigBar = i
		return BuySignal
	}
	if s.checkBearishShift(i) {
		s.lastSigBar = i
		return SellSignal
	}
	return NoSignal
}

// checkBullishShift detects a bearish-to-bullish mega-structure shift.
// Requires min_swing_count consecutive Lower Highs (LH) followed by
// the first Higher High (HH) → enter on bullish displacement.
func (s *MegatradeStrategy) checkBullishShift(i int) bool {
	start := i - s.lookback
	if start < s.swingPeriod {
		start = s.swingPeriod
	}

	highs := s.collectHighs(start, i-s.swingPeriod)
	if len(highs) < s.minSwingCount+1 {
		return false
	}

	// Count consecutive Lower Highs ending just before the last swing high
	lhCount := 0
	for k := len(highs) - 2; k >= 1; k-- {
		if highs[k].price < highs[k-1].price {
			lhCount++
		} else {
			break
		}
	}

	if lhCount < s.minSwingCount {
		return false
	}

	// The last swing high must be higher than the one before it (HH = shift)
	last := highs[len(highs)-1]
	prev := highs[len(highs)-2]
	if last.price <= prev.price {
		return false
	}

	// Look for bullish displacement between the shift point and bar i
	for d := last.idx; d <= i; d++ {
		if d >= len(s.bars) || math.IsNaN(s.atr[d]) || s.atr[d] == 0 {
			continue
		}
		b := s.bars[d]
		body := b.Close - b.Open
		rng := b.High - b.Low
		if rng == 0 {
			continue
		}
		if body >= s.dispMult*s.atr[d] && body/rng >= s.bodyRatio {
			if d == i || d == i-1 {
				return true
			}
		}
	}
	return false
}

// checkBearishShift detects a bullish-to-bearish mega-structure shift.
// Requires min_swing_count consecutive Higher Lows (HL) followed by
// the first Lower Low (LL) → enter on bearish displacement.
func (s *MegatradeStrategy) checkBearishShift(i int) bool {
	start := i - s.lookback
	if start < s.swingPeriod {
		start = s.swingPeriod
	}

	lows := s.collectLows(start, i-s.swingPeriod)
	if len(lows) < s.minSwingCount+1 {
		return false
	}

	// Count consecutive Higher Lows ending just before the last swing low
	hlCount := 0
	for k := len(lows) - 2; k >= 1; k-- {
		if lows[k].price > lows[k-1].price {
			hlCount++
		} else {
			break
		}
	}

	if hlCount < s.minSwingCount {
		return false
	}

	// The last swing low must be lower than the one before it (LL = shift)
	last := lows[len(lows)-1]
	prev := lows[len(lows)-2]
	if last.price >= prev.price {
		return false
	}

	// Look for bearish displacement between the shift point and bar i
	for d := last.idx; d <= i; d++ {
		if d >= len(s.bars) || math.IsNaN(s.atr[d]) || s.atr[d] == 0 {
			continue
		}
		b := s.bars[d]
		body := b.Open - b.Close
		rng := b.High - b.Low
		if rng == 0 {
			continue
		}
		if body >= s.dispMult*s.atr[d] && body/rng >= s.bodyRatio {
			if d == i || d == i-1 {
				return true
			}
		}
	}
	return false
}
