package backtest

import (
	"math"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── HRLR to LRLR Transition Strategy ────────────────────────────────────
//
// HRLR (High Resistance Liquidity Run) = choppy, ranging price action
// characterised by small ATR relative to the recent range and many
// directional reversals.
//
// LRLR (Low Resistance Liquidity Run) = fast, impulsive trending move
// characterised by ATR expansion and sustained directional momentum.
//
// The strategy detects the transition from HRLR to LRLR:
//   1. Compute a "choppiness" score over a rolling window: ratio of sum
//      of individual bar ranges to the total high-low range of the window.
//      A value near 1.0 is very choppy; near 0.0 is strongly trending.
//   2. When choppiness drops significantly (crosses below threshold after
//      having been elevated), an LRLR has begun.
//   3. Enter in the direction of the LRLR move, confirmed by a
//      displacement candle.
//
// Parameters:
//   swing_period   – swing detection period (default 5)
//   atr_period     – ATR calculation period (default 14)
//   disp_mult      – displacement ATR multiple (default 1.0)
//   body_ratio     – min body/range ratio for displacement (default 0.5)
//   chop_period    – rolling window for choppiness (default 20)
//   chop_threshold – choppiness level that divides HRLR from LRLR (default 0.5)

type LRLREntryStrategy struct {
	bars          []data.OHLCV
	atr           []float64
	choppiness    []float64
	swingPeriod   int
	dispMult      float64
	bodyRatio     float64
	chopPeriod    int
	chopThreshold float64
	lastSigBar    int
}

func (s *LRLREntryStrategy) Name() string { return "LRLR Entry" }
func (s *LRLREntryStrategy) Description() string {
	return "HRLR to LRLR transition: enter when choppiness drops and displacement confirms direction"
}

func (s *LRLREntryStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	s.swingPeriod = int(getParam(params, "swing_period", 5))
	atrPeriod := int(getParam(params, "atr_period", 14))
	s.dispMult = getParam(params, "disp_mult", 1.0)
	s.bodyRatio = getParam(params, "body_ratio", 0.5)
	s.chopPeriod = int(getParam(params, "chop_period", 20))
	s.chopThreshold = getParam(params, "chop_threshold", 0.5)
	s.lastSigBar = -s.chopPeriod

	s.atr = indicators.ATR(bars, atrPeriod)

	// Compute choppiness index per bar.
	n := len(bars)
	s.choppiness = make([]float64, n)
	for i := range s.choppiness {
		s.choppiness[i] = math.NaN()
	}
	for i := s.chopPeriod - 1; i < n; i++ {
		sumBarRanges := 0.0
		windowHigh := bars[i].High
		windowLow := bars[i].Low
		for j := i - s.chopPeriod + 1; j <= i; j++ {
			sumBarRanges += bars[j].High - bars[j].Low
			if bars[j].High > windowHigh {
				windowHigh = bars[j].High
			}
			if bars[j].Low < windowLow {
				windowLow = bars[j].Low
			}
		}
		totalRange := windowHigh - windowLow
		if totalRange > 0 {
			// Normalise: 1.0 = pure chop (bar ranges add up to much more
			// than total range), approaching 1/N = perfect trend.
			// We cap at 1.0 for clarity.
			chop := sumBarRanges / totalRange / float64(s.chopPeriod)
			if chop > 1.0 {
				chop = 1.0
			}
			s.choppiness[i] = chop
		}
	}
}

func (s *LRLREntryStrategy) Signal(i int) SignalType {
	if i < s.chopPeriod+1 || math.IsNaN(s.atr[i]) || math.IsNaN(s.choppiness[i]) || math.IsNaN(s.choppiness[i-1]) {
		return NoSignal
	}
	if i-s.lastSigBar < s.swingPeriod {
		return NoSignal
	}

	prevChop := s.choppiness[i-1]
	curChop := s.choppiness[i]

	// Detect HRLR → LRLR transition: choppiness was above threshold and
	// has now dropped below it.
	if prevChop <= s.chopThreshold || curChop >= s.chopThreshold {
		return NoSignal
	}

	// Confirm direction with displacement candle at bar i.
	bar := s.bars[i]
	rng := bar.High - bar.Low
	if rng == 0 || s.atr[i] == 0 {
		return NoSignal
	}

	bullBody := bar.Close - bar.Open
	bearBody := bar.Open - bar.Close

	if bullBody >= s.dispMult*s.atr[i] && bullBody/rng >= s.bodyRatio {
		s.lastSigBar = i
		return BuySignal
	}
	if bearBody >= s.dispMult*s.atr[i] && bearBody/rng >= s.bodyRatio {
		s.lastSigBar = i
		return SellSignal
	}

	return NoSignal
}
