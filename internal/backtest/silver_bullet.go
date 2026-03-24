package backtest

import (
	"math"
	"time"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── Silver Bullet Strategy ────────────────────────────────────────────────
//
// ICT Silver Bullet: 3x daily scalp windows. Within each 1-hour kill zone,
// look for a liquidity sweep (above/below recent swing) followed by a
// Fair Value Gap (FVG) forming in the opposite direction. Enter on
// retrace into FVG.
//
// Windows (EST):
//   1. London Silver Bullet:  03:00 – 04:00
//   2. NY AM Silver Bullet:   10:00 – 11:00
//   3. NY PM Silver Bullet:   14:00 – 15:00
//
// Parameters:
//   swing_period  – swing detection lookback (default 5)
//   atr_period    – ATR for displacement sizing (default 14)
//   disp_mult     – displacement threshold as ATR multiple (default 1.0)
//   body_ratio    – min body/range ratio for displacement (default 0.5)
//   sb_lookback   – how many bars back to scan for sweep+FVG (default 20)

type SilverBulletStrategy struct {
	bars        []data.OHLCV
	atr         []float64
	swingHighs  []float64
	swingLows   []float64
	swingPeriod int
	dispMult    float64
	bodyRatio   float64
	lookback    int
	lastSigBar  int
	est         *time.Location
}

func (s *SilverBulletStrategy) Name() string { return "Silver Bullet" }
func (s *SilverBulletStrategy) Description() string {
	return "ICT Silver Bullet: liquidity sweep + FVG entry in 3 daily kill-zone windows (03-04, 10-11, 14-15 EST)"
}

func (s *SilverBulletStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	s.swingPeriod = int(getParam(params, "swing_period", 5))
	atrPeriod := int(getParam(params, "atr_period", 14))
	s.dispMult = getParam(params, "disp_mult", 1.0)
	s.bodyRatio = getParam(params, "body_ratio", 0.5)
	s.lookback = int(getParam(params, "sb_lookback", 20))
	if s.lookback < s.swingPeriod*2 {
		s.lookback = s.swingPeriod * 2
	}
	s.lastSigBar = -s.lookback
	s.est = estLoc

	s.atr = indicators.ATR(bars, atrPeriod)
	s.swingHighs = indicators.SwingHighs(bars, s.swingPeriod)
	s.swingLows = indicators.SwingLows(bars, s.swingPeriod)
}

// inSilverBulletWindow checks if bar time falls within one of the 3 SB windows.
func (s *SilverBulletStrategy) inSilverBulletWindow(t time.Time) bool {
	estTime := t.In(s.est)
	h := estTime.Hour()
	// Window 1: 03:00-04:00 EST  (h == 3)
	// Window 2: 10:00-11:00 EST  (h == 10)
	// Window 3: 14:00-15:00 EST  (h == 14)
	return h == 3 || h == 10 || h == 14
}

func (s *SilverBulletStrategy) Signal(i int) SignalType {
	if i < s.swingPeriod*2+3 || math.IsNaN(s.atr[i]) {
		return NoSignal
	}
	// Throttle: no repeat signals within swingPeriod bars
	if i-s.lastSigBar < s.swingPeriod {
		return NoSignal
	}
	// Must be inside a Silver Bullet window
	if !s.inSilverBulletWindow(s.bars[i].Time) {
		return NoSignal
	}

	if s.checkBullishSB(i) {
		s.lastSigBar = i
		return BuySignal
	}
	if s.checkBearishSB(i) {
		s.lastSigBar = i
		return SellSignal
	}
	return NoSignal
}

// checkBullishSB: sweep below a swing low → bullish FVG → retrace into FVG
func (s *SilverBulletStrategy) checkBullishSB(i int) bool {
	bars := s.bars
	start := i - s.lookback
	if start < s.swingPeriod {
		start = s.swingPeriod
	}

	// Find a swing low that was swept within the lookback
	for j := i - s.swingPeriod - 1; j >= start; j-- {
		if math.IsNaN(s.swingLows[j]) {
			continue
		}
		swingLowPrice := s.swingLows[j]

		// Find sweep bar: wick below swing low but closes above
		sweepIdx := -1
		for g := j + 1; g <= i; g++ {
			if bars[g].Low < swingLowPrice && bars[g].Close > swingLowPrice {
				sweepIdx = g
				break
			}
		}
		if sweepIdx < 0 {
			continue
		}

		// Find bullish FVG after the sweep
		for d := sweepIdx; d < i-2; d++ {
			if d+2 >= len(bars) {
				break
			}
			// Bullish FVG: bar[d].High < bar[d+2].Low
			gapBot := bars[d].High
			gapTop := bars[d+2].Low
			if gapTop <= gapBot {
				continue
			}
			// Validate displacement on bar[d+1]
			if math.IsNaN(s.atr[d+1]) || s.atr[d+1] == 0 {
				continue
			}
			body := bars[d+1].Close - bars[d+1].Open
			rng := bars[d+1].High - bars[d+1].Low
			if rng == 0 {
				continue
			}
			if body < s.dispMult*s.atr[d+1] || body/rng < s.bodyRatio {
				continue
			}

			// Current bar touches the FVG zone
			if bars[i].Low <= gapTop && bars[i].High >= gapBot {
				return true
			}
		}
	}
	return false
}

// checkBearishSB: sweep above a swing high → bearish FVG → retrace into FVG
func (s *SilverBulletStrategy) checkBearishSB(i int) bool {
	bars := s.bars
	start := i - s.lookback
	if start < s.swingPeriod {
		start = s.swingPeriod
	}

	for j := i - s.swingPeriod - 1; j >= start; j-- {
		if math.IsNaN(s.swingHighs[j]) {
			continue
		}
		swingHighPrice := s.swingHighs[j]

		// Find sweep bar: wick above swing high but closes below
		sweepIdx := -1
		for g := j + 1; g <= i; g++ {
			if bars[g].High > swingHighPrice && bars[g].Close < swingHighPrice {
				sweepIdx = g
				break
			}
		}
		if sweepIdx < 0 {
			continue
		}

		// Find bearish FVG after the sweep
		for d := sweepIdx; d < i-2; d++ {
			if d+2 >= len(bars) {
				break
			}
			// Bearish FVG: bar[d].Low > bar[d+2].High
			gapTop := bars[d].Low
			gapBot := bars[d+2].High
			if gapTop <= gapBot {
				continue
			}
			// Validate displacement on bar[d+1]
			if math.IsNaN(s.atr[d+1]) || s.atr[d+1] == 0 {
				continue
			}
			body := bars[d+1].Open - bars[d+1].Close
			rng := bars[d+1].High - bars[d+1].Low
			if rng == 0 {
				continue
			}
			if body < s.dispMult*s.atr[d+1] || body/rng < s.bodyRatio {
				continue
			}

			// Current bar touches the FVG zone
			if bars[i].High >= gapBot && bars[i].Low <= gapTop {
				return true
			}
		}
	}
	return false
}
