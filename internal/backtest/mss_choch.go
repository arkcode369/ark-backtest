package backtest

import (
	"math"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── MSS/CHoCH (Market Structure Shift / Change of Character) ─────────────
//
// Tracks market structure via swing highs and swing lows:
//   - Bullish structure: Higher Highs (HH) + Higher Lows (HL)
//   - Bearish structure: Lower Highs (LH) + Lower Lows (LL)
//
// A CHoCH occurs when the structure sequence breaks:
//   - Bearish-to-Bullish CHoCH: after LH/LL sequence, price makes a HH
//   - Bullish-to-Bearish CHoCH: after HH/HL sequence, price makes a LL
//
// After CHoCH detection, wait for a displacement candle to confirm:
//   - BUY on bullish CHoCH + bullish displacement
//   - SELL on bearish CHoCH + bearish displacement
//
// Parameters:
//   swing_period – swing detection period (default 5)
//   atr_period   – ATR calculation period (default 14)
//   disp_mult    – displacement ATR multiple (default 1.0)
//   body_ratio   – min body/range ratio (default 0.5)
//   lookback     – lookback window for structure analysis (default 30)

type MSSCHoCHStrategy struct {
	bars        []data.OHLCV
	atr         []float64
	swingHighs  []float64
	swingLows   []float64
	swingPeriod int
	dispMult    float64
	bodyRatio   float64
	lookback    int
	lastSigBar  int
}

func (s *MSSCHoCHStrategy) Name() string { return "MSS/CHoCH" }
func (s *MSSCHoCHStrategy) Description() string {
	return "Market Structure Shift: detect CHoCH (bullish/bearish structure break) with displacement confirmation"
}

func (s *MSSCHoCHStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	s.swingPeriod = int(getParam(params, "swing_period", 5))
	atrPeriod := int(getParam(params, "atr_period", 14))
	s.dispMult = getParam(params, "disp_mult", 1.0)
	s.bodyRatio = getParam(params, "body_ratio", 0.5)
	s.lookback = int(getParam(params, "lookback", 30))
	if s.lookback < s.swingPeriod*3 {
		s.lookback = s.swingPeriod * 3
	}
	s.lastSigBar = -s.lookback

	s.atr = indicators.ATR(bars, atrPeriod)
	s.swingHighs = indicators.SwingHighs(bars, s.swingPeriod)
	s.swingLows = indicators.SwingLows(bars, s.swingPeriod)
}

func (s *MSSCHoCHStrategy) Signal(i int) SignalType {
	if i < s.swingPeriod*2+3 || math.IsNaN(s.atr[i]) {
		return NoSignal
	}
	if i-s.lastSigBar < s.swingPeriod {
		return NoSignal
	}

	if s.checkBullishCHoCH(i) {
		s.lastSigBar = i
		return BuySignal
	}
	if s.checkBearishCHoCH(i) {
		s.lastSigBar = i
		return SellSignal
	}
	return NoSignal
}

// swingPoint holds index and price of a confirmed swing.
type swingPoint struct {
	idx   int
	price float64
}

// collectSwingHighs gathers confirmed swing highs in [start, end) ordered by index.
func (s *MSSCHoCHStrategy) collectSwingHighs(start, end int) []swingPoint {
	var pts []swingPoint
	for j := start; j < end; j++ {
		if !math.IsNaN(s.swingHighs[j]) {
			pts = append(pts, swingPoint{idx: j, price: s.swingHighs[j]})
		}
	}
	return pts
}

// collectSwingLows gathers confirmed swing lows in [start, end) ordered by index.
func (s *MSSCHoCHStrategy) collectSwingLows(start, end int) []swingPoint {
	var pts []swingPoint
	for j := start; j < end; j++ {
		if !math.IsNaN(s.swingLows[j]) {
			pts = append(pts, swingPoint{idx: j, price: s.swingLows[j]})
		}
	}
	return pts
}

// checkBullishCHoCH detects a bearish-to-bullish CHoCH with displacement.
// Bearish structure = at least 2 consecutive Lower Highs (LH).
// CHoCH = price then breaks above the last LH.
func (s *MSSCHoCHStrategy) checkBullishCHoCH(i int) bool {
	start := i - s.lookback
	if start < s.swingPeriod {
		start = s.swingPeriod
	}

	highs := s.collectSwingHighs(start, i-s.swingPeriod)
	if len(highs) < 3 {
		return false
	}

	// Look for at least 2 consecutive lower highs followed by a higher high.
	// We scan from the end to find the most recent CHoCH.
	for k := len(highs) - 1; k >= 2; k-- {
		h2 := highs[k]   // most recent swing high
		h1 := highs[k-1] // previous
		h0 := highs[k-2] // one before that

		// h0 -> h1 is a Lower High, h1 -> h2 is a Higher High = CHoCH
		if h1.price < h0.price && h2.price > h1.price {
			// CHoCH detected at h2. Now look for bullish displacement
			// between h2.idx and i (inclusive).
			for d := h2.idx; d <= i; d++ {
				if s.isBullishDisplacement(d) {
					if d == i || d == i-1 {
						return true
					}
				}
			}
		}
	}
	return false
}

// checkBearishCHoCH detects a bullish-to-bearish CHoCH with displacement.
// Bullish structure = at least 2 consecutive Higher Lows (HL).
// CHoCH = price then breaks below the last HL.
func (s *MSSCHoCHStrategy) checkBearishCHoCH(i int) bool {
	start := i - s.lookback
	if start < s.swingPeriod {
		start = s.swingPeriod
	}

	lows := s.collectSwingLows(start, i-s.swingPeriod)
	if len(lows) < 3 {
		return false
	}

	for k := len(lows) - 1; k >= 2; k-- {
		l2 := lows[k]
		l1 := lows[k-1]
		l0 := lows[k-2]

		// l0 -> l1 is a Higher Low, l1 -> l2 is a Lower Low = CHoCH
		if l1.price > l0.price && l2.price < l1.price {
			for d := l2.idx; d <= i; d++ {
				if s.isBearishDisplacement(d) {
					if d == i || d == i-1 {
						return true
					}
				}
			}
		}
	}
	return false
}

func (s *MSSCHoCHStrategy) isBullishDisplacement(d int) bool {
	if d >= len(s.bars) || math.IsNaN(s.atr[d]) || s.atr[d] == 0 {
		return false
	}
	b := s.bars[d]
	body := b.Close - b.Open
	rng := b.High - b.Low
	if rng == 0 {
		return false
	}
	return body >= s.dispMult*s.atr[d] && body/rng >= s.bodyRatio
}

func (s *MSSCHoCHStrategy) isBearishDisplacement(d int) bool {
	if d >= len(s.bars) || math.IsNaN(s.atr[d]) || s.atr[d] == 0 {
		return false
	}
	b := s.bars[d]
	body := b.Open - b.Close
	rng := b.High - b.Low
	if rng == 0 {
		return false
	}
	return body >= s.dispMult*s.atr[d] && body/rng >= s.bodyRatio
}
