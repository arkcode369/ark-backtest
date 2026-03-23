package backtest

import (
	"math"
	"time"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── AMD Session Strategy ──────────────────────────────────────────────────
//
// ICT AMD (Accumulation → Manipulation → Distribution):
//
//   1. Accumulation: Asian session forms a tight range (CBDR).
//   2. Manipulation: London session breaks one side of the Asian range
//      (false breakout / Judas Swing).
//   3. Distribution: NY session trends in the OPPOSITE direction of
//      the London manipulation.
//
// Bullish AMD: Asian range → London breaks BELOW Asian low → NY reverses up.
// Bearish AMD: Asian range → London breaks ABOVE Asian high → NY reverses down.
//
// Entry is generated at the start of the Distribution phase when
// displacement confirms the reversal.
//
// Parameters:
//   atr_period    – ATR for displacement detection (default 14)
//   disp_mult     – displacement ATR multiple (default 1.0)
//   body_ratio    – min body/range ratio (default 0.5)
//   swing_period  – swing detection period (default 5)

type AMDSessionStrategy struct {
	bars        []data.OHLCV
	atr         []float64
	swingHighs  []float64
	swingLows   []float64
	swingPeriod int
	dispMult    float64
	bodyRatio   float64
	lastSigBar  int
	est         *time.Location

	// Per-day tracking
	dayRanges map[string]*amdDayData
}

type amdDayData struct {
	asianHigh  float64
	asianLow   float64
	hasAsian   bool
	londonBias int // +1 = broke above asian high, -1 = broke below asian low, 0 = no break
}

func (s *AMDSessionStrategy) Name() string { return "AMD Session" }
func (s *AMDSessionStrategy) Description() string {
	return "ICT AMD: Accumulation (Asian) → Manipulation (London false breakout) → Distribution (NY reversal)"
}

func (s *AMDSessionStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	s.swingPeriod = int(getParam(params, "swing_period", 5))
	atrPeriod := int(getParam(params, "atr_period", 14))
	s.dispMult = getParam(params, "disp_mult", 1.0)
	s.bodyRatio = getParam(params, "body_ratio", 0.5)
	s.lastSigBar = -20
	s.est = time.FixedZone("EST", -5*3600)

	s.atr = indicators.ATR(bars, atrPeriod)
	s.swingHighs = indicators.SwingHighs(bars, s.swingPeriod)
	s.swingLows = indicators.SwingLows(bars, s.swingPeriod)

	// Pre-compute per-day Asian ranges and London manipulation
	s.precomputeAMD()
}

func (s *AMDSessionStrategy) precomputeAMD() {
	s.dayRanges = make(map[string]*amdDayData)

	for _, bar := range s.bars {
		t := bar.Time.In(s.est)
		h := t.Hour()

		// Determine trading date
		var dateKey string
		if h >= 18 {
			dateKey = t.Add(24 * time.Hour).Format("2006-01-02")
		} else {
			dateKey = t.Format("2006-01-02")
		}

		dd, exists := s.dayRanges[dateKey]
		if !exists {
			dd = &amdDayData{asianHigh: -math.MaxFloat64, asianLow: math.MaxFloat64}
			s.dayRanges[dateKey] = dd
		}

		// Asian session: 18:00-00:00 EST (next day's CBDR) or 00:00-02:00
		if h >= 18 || h < 2 {
			if bar.High > dd.asianHigh {
				dd.asianHigh = bar.High
			}
			if bar.Low < dd.asianLow {
				dd.asianLow = bar.Low
			}
			dd.hasAsian = true
		}

		// London session: 02:00-05:00 EST — detect manipulation
		if h >= 2 && h < 5 && dd.hasAsian && dd.asianHigh > -math.MaxFloat64 {
			if bar.High > dd.asianHigh && dd.londonBias == 0 {
				dd.londonBias = 1 // broke above Asian high
			}
			if bar.Low < dd.asianLow && dd.londonBias == 0 {
				dd.londonBias = -1 // broke below Asian low
			}
		}
	}
}

func (s *AMDSessionStrategy) getDayData(i int) *amdDayData {
	t := s.bars[i].Time.In(s.est)
	h := t.Hour()
	var dateKey string
	if h >= 18 {
		dateKey = t.Add(24 * time.Hour).Format("2006-01-02")
	} else {
		dateKey = t.Format("2006-01-02")
	}
	return s.dayRanges[dateKey]
}

func (s *AMDSessionStrategy) Signal(i int) SignalType {
	if i < s.swingPeriod*2+3 || math.IsNaN(s.atr[i]) {
		return NoSignal
	}
	if i-s.lastSigBar < s.swingPeriod*2 {
		return NoSignal
	}

	// Only generate signals during NY session (07:00-16:00 EST)
	t := s.bars[i].Time.In(s.est)
	h := t.Hour()
	if h < 7 || h >= 16 {
		return NoSignal
	}

	dd := s.getDayData(i)
	if dd == nil || !dd.hasAsian || dd.londonBias == 0 {
		return NoSignal
	}

	// Check displacement on current bar confirming distribution
	rng := s.bars[i].High - s.bars[i].Low
	if rng == 0 || math.IsNaN(s.atr[i]) || s.atr[i] == 0 {
		return NoSignal
	}

	// Bullish AMD: London broke below Asian low → NY bullish displacement
	if dd.londonBias == -1 {
		body := s.bars[i].Close - s.bars[i].Open
		if body > 0 && body >= s.dispMult*s.atr[i] && body/rng >= s.bodyRatio {
			s.lastSigBar = i
			return BuySignal
		}
	}

	// Bearish AMD: London broke above Asian high → NY bearish displacement
	if dd.londonBias == 1 {
		body := s.bars[i].Open - s.bars[i].Close
		if body > 0 && body >= s.dispMult*s.atr[i] && body/rng >= s.bodyRatio {
			s.lastSigBar = i
			return SellSignal
		}
	}

	return NoSignal
}
