package backtest

import (
	"math"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── Turtle Soup / Stop Run Strategy ───────────────────────────────────────
//
// ICT Turtle Soup: Detect a sweep of swing highs or swing lows (stop run),
// then look for a displacement candle in the OPPOSITE direction confirming
// that the sweep was a false breakout. Entry on the bar that produces
// displacement after the sweep.
//
// Bullish Turtle Soup:
//   1. Price sweeps below a swing low (or equal lows)
//   2. Displacement: bullish candle body > disp_mult * ATR with body/range > body_ratio
//   3. Entry on displacement bar or next bar
//
// Bearish Turtle Soup:
//   1. Price sweeps above a swing high (or equal highs)
//   2. Displacement: bearish candle body > disp_mult * ATR with body/range > body_ratio
//   3. Entry on displacement bar or next bar
//
// "Equal highs/lows" detection: if two swing highs (or lows) are within
// equal_tol * ATR of each other, they form "equal" levels — a higher
// probability stop-run target.
//
// Parameters:
//   swing_period  – swing detection period (default 5)
//   atr_period    – ATR calculation period (default 14)
//   disp_mult     – displacement ATR multiple (default 1.5)
//   body_ratio    – min body/range ratio (default 0.6)
//   ts_lookback   – lookback window for finding sweeps (default 30)
//   equal_tol     – tolerance for equal highs/lows as ATR fraction (default 0.1)
//   require_equal – if 1, only trade equal highs/lows; if 0, any swing (default 0)

type TurtleSoupStrategy struct {
	bars         []data.OHLCV
	atr          []float64
	swingHighs   []float64
	swingLows    []float64
	swingPeriod  int
	dispMult     float64
	bodyRatio    float64
	lookback     int
	equalTol     float64
	requireEqual bool
	lastSigBar   int
}

func (s *TurtleSoupStrategy) Name() string { return "Turtle Soup" }
func (s *TurtleSoupStrategy) Description() string {
	return "ICT Turtle Soup: sweep of swing high/low (stop run) → displacement reversal entry"
}

func (s *TurtleSoupStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	s.swingPeriod = int(getParam(params, "swing_period", 5))
	atrPeriod := int(getParam(params, "atr_period", 14))
	s.dispMult = getParam(params, "disp_mult", 1.5)
	s.bodyRatio = getParam(params, "body_ratio", 0.6)
	s.lookback = int(getParam(params, "ts_lookback", 30))
	if s.lookback < s.swingPeriod*3 {
		s.lookback = s.swingPeriod * 3
	}
	s.equalTol = getParam(params, "equal_tol", 0.1)
	s.requireEqual = getParam(params, "require_equal", 0) == 1
	s.lastSigBar = -s.lookback

	s.atr = indicators.ATR(bars, atrPeriod)
	s.swingHighs = indicators.SwingHighs(bars, s.swingPeriod)
	s.swingLows = indicators.SwingLows(bars, s.swingPeriod)
}

func (s *TurtleSoupStrategy) Signal(i int) SignalType {
	if i < s.swingPeriod*2+3 || math.IsNaN(s.atr[i]) {
		return NoSignal
	}
	if i-s.lastSigBar < s.swingPeriod {
		return NoSignal
	}

	if s.checkBullishTS(i) {
		s.lastSigBar = i
		return BuySignal
	}
	if s.checkBearishTS(i) {
		s.lastSigBar = i
		return SellSignal
	}
	return NoSignal
}

// findEqualLows collects swing lows within lookback that are "equal"
// (within equalTol * ATR of each other). Returns the equal level if found.
func (s *TurtleSoupStrategy) findEqualLows(i int) (float64, bool) {
	start := i - s.lookback
	if start < s.swingPeriod {
		start = s.swingPeriod
	}

	// Collect all swing lows in range
	var swingLevels []float64
	for j := start; j < i-s.swingPeriod; j++ {
		if !math.IsNaN(s.swingLows[j]) {
			swingLevels = append(swingLevels, s.swingLows[j])
		}
	}

	if len(swingLevels) < 2 {
		return 0, false
	}

	// Find any pair within tolerance
	tol := s.equalTol * s.atr[i]
	for a := 0; a < len(swingLevels); a++ {
		for b := a + 1; b < len(swingLevels); b++ {
			if math.Abs(swingLevels[a]-swingLevels[b]) <= tol {
				// Return the lower of the two as the target
				level := math.Min(swingLevels[a], swingLevels[b])
				return level, true
			}
		}
	}
	return 0, false
}

// findEqualHighs collects swing highs within lookback that are "equal".
func (s *TurtleSoupStrategy) findEqualHighs(i int) (float64, bool) {
	start := i - s.lookback
	if start < s.swingPeriod {
		start = s.swingPeriod
	}

	var swingLevels []float64
	for j := start; j < i-s.swingPeriod; j++ {
		if !math.IsNaN(s.swingHighs[j]) {
			swingLevels = append(swingLevels, s.swingHighs[j])
		}
	}

	if len(swingLevels) < 2 {
		return 0, false
	}

	tol := s.equalTol * s.atr[i]
	for a := 0; a < len(swingLevels); a++ {
		for b := a + 1; b < len(swingLevels); b++ {
			if math.Abs(swingLevels[a]-swingLevels[b]) <= tol {
				level := math.Max(swingLevels[a], swingLevels[b])
				return level, true
			}
		}
	}
	return 0, false
}

// checkBullishTS: sweep below swing low (or equal lows) → bullish displacement
func (s *TurtleSoupStrategy) checkBullishTS(i int) bool {
	bars := s.bars
	start := i - s.lookback
	if start < s.swingPeriod {
		start = s.swingPeriod
	}

	// Determine target level(s) to sweep
	type sweepTarget struct {
		price float64
		idx   int // swing bar index (-1 for equal-low level)
	}
	var targets []sweepTarget

	if s.requireEqual {
		// Only trade equal lows
		level, found := s.findEqualLows(i)
		if !found {
			return false
		}
		targets = append(targets, sweepTarget{price: level, idx: -1})
	} else {
		// Any swing low
		for j := i - s.swingPeriod - 1; j >= start; j-- {
			if !math.IsNaN(s.swingLows[j]) {
				targets = append(targets, sweepTarget{price: s.swingLows[j], idx: j})
			}
		}
		// Also check equal lows as bonus targets
		if level, found := s.findEqualLows(i); found {
			targets = append(targets, sweepTarget{price: level, idx: -1})
		}
	}

	for _, target := range targets {
		sweepStart := start
		if target.idx > 0 {
			sweepStart = target.idx + 1
		}

		// Find bar that sweeps below target
		sweepIdx := -1
		for g := sweepStart; g <= i; g++ {
			if bars[g].Low < target.price {
				sweepIdx = g
				break
			}
		}
		if sweepIdx < 0 {
			continue
		}

		// Look for bullish displacement after the sweep (including sweep bar itself)
		for d := sweepIdx; d <= i; d++ {
			if math.IsNaN(s.atr[d]) || s.atr[d] == 0 {
				continue
			}
			body := bars[d].Close - bars[d].Open // positive = bullish
			rng := bars[d].High - bars[d].Low
			if rng == 0 {
				continue
			}
			if body >= s.dispMult*s.atr[d] && body/rng >= s.bodyRatio {
				// Displacement found — if this is on bar i or we're on bar i now
				if d == i || d == i-1 {
					return true
				}
			}
		}
	}
	return false
}

// checkBearishTS: sweep above swing high (or equal highs) → bearish displacement
func (s *TurtleSoupStrategy) checkBearishTS(i int) bool {
	bars := s.bars
	start := i - s.lookback
	if start < s.swingPeriod {
		start = s.swingPeriod
	}

	type sweepTarget struct {
		price float64
		idx   int
	}
	var targets []sweepTarget

	if s.requireEqual {
		level, found := s.findEqualHighs(i)
		if !found {
			return false
		}
		targets = append(targets, sweepTarget{price: level, idx: -1})
	} else {
		for j := i - s.swingPeriod - 1; j >= start; j-- {
			if !math.IsNaN(s.swingHighs[j]) {
				targets = append(targets, sweepTarget{price: s.swingHighs[j], idx: j})
			}
		}
		if level, found := s.findEqualHighs(i); found {
			targets = append(targets, sweepTarget{price: level, idx: -1})
		}
	}

	for _, target := range targets {
		sweepStart := start
		if target.idx > 0 {
			sweepStart = target.idx + 1
		}

		// Find bar that sweeps above target
		sweepIdx := -1
		for g := sweepStart; g <= i; g++ {
			if bars[g].High > target.price {
				sweepIdx = g
				break
			}
		}
		if sweepIdx < 0 {
			continue
		}

		// Look for bearish displacement after the sweep
		for d := sweepIdx; d <= i; d++ {
			if math.IsNaN(s.atr[d]) || s.atr[d] == 0 {
				continue
			}
			body := bars[d].Open - bars[d].Close // positive = bearish
			rng := bars[d].High - bars[d].Low
			if rng == 0 {
				continue
			}
			if body >= s.dispMult*s.atr[d] && body/rng >= s.bodyRatio {
				if d == i || d == i-1 {
					return true
				}
			}
		}
	}
	return false
}
