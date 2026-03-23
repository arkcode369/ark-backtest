package backtest

import (
	"math"
	"time"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── HOD/LOD Projection Strategy ─────────────────────────────────────────
//
// Projects the expected High of Day (HOD) and Low of Day (LOD) using CBDR
// standard deviation levels. The Asian session range (CBDR) sets a
// statistical boundary: most days, the HOD or LOD forms at STD 1-2 of the
// CBDR range. Once price reaches a projected level, we treat it as the
// likely HOD or LOD and enter for reversal.
//
// Key difference from cbdr_std: this strategy explicitly tracks whether
// the day's HOD or LOD has already been established. If the day has
// already printed a large move (exceeding max_std), the day is classified
// as a "runner" and entries are skipped.
//
// Bullish: price reaches projected LOD (STD below CBDR low) with reversal.
// Bearish: price reaches projected HOD (STD above CBDR high) with reversal.
//
// Parameters:
//   swing_period – signal throttle period (default 5)
//   atr_period   – ATR period (default 14)
//   disp_mult    – displacement ATR multiple (default 0.5)
//   body_ratio   – min body/range ratio (default 0.4)
//   min_std      – minimum STD level to enter (default 1.0)
//   max_std      – maximum STD level; beyond = runner day (default 2.5)

type HODLODStrategy struct {
	bars        []data.OHLCV
	atr         []float64
	cbdrs       []CBDRResult
	swingPeriod int
	dispMult    float64
	bodyRatio   float64
	minSTD      float64
	maxSTD      float64
	lastSigBar  int
	est         *time.Location

	// Per-day tracking: keyed by date string "2006-01-02"
	dayHighSeen map[string]float64 // running high of day
	dayLowSeen  map[string]float64 // running low of day
	daySignaled map[string]bool    // whether we already signaled this day
}

func (s *HODLODStrategy) Name() string { return "HOD/LOD Projection" }
func (s *HODLODStrategy) Description() string {
	return "Project daily HOD/LOD via CBDR STD levels; enter reversal at projected extremes"
}

func (s *HODLODStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	s.swingPeriod = int(getParam(params, "swing_period", 5))
	atrPeriod := int(getParam(params, "atr_period", 14))
	s.dispMult = getParam(params, "disp_mult", 0.5)
	s.bodyRatio = getParam(params, "body_ratio", 0.4)
	s.minSTD = getParam(params, "min_std", 1.0)
	s.maxSTD = getParam(params, "max_std", 2.5)
	s.lastSigBar = -10
	s.est = time.FixedZone("EST", -5*3600)

	s.atr = indicators.ATR(bars, atrPeriod)
	s.cbdrs = ComputeCBDR(bars)

	s.dayHighSeen = make(map[string]float64)
	s.dayLowSeen = make(map[string]float64)
	s.daySignaled = make(map[string]bool)
}

func (s *HODLODStrategy) Signal(i int) SignalType {
	if i < s.swingPeriod*2+3 || math.IsNaN(s.atr[i]) || s.atr[i] == 0 {
		return NoSignal
	}
	if i-s.lastSigBar < s.swingPeriod {
		return NoSignal
	}

	bar := s.bars[i]
	t := bar.Time.In(s.est)
	h := t.Hour()

	// Only trade during London/NY sessions (02:00-16:00 EST)
	if h < 2 || h >= 16 {
		return NoSignal
	}

	// Determine trading date
	dateKey := t.Format("2006-01-02")

	// Update running high/low for the day
	if prev, ok := s.dayHighSeen[dateKey]; !ok || bar.High > prev {
		s.dayHighSeen[dateKey] = bar.High
	}
	if prev, ok := s.dayLowSeen[dateKey]; !ok || bar.Low < prev {
		s.dayLowSeen[dateKey] = bar.Low
	}

	// Only one signal per day to avoid chasing
	if s.daySignaled[dateKey] {
		return NoSignal
	}

	cbdr := FindCBDRForBar(s.cbdrs, bar)
	if cbdr == nil || cbdr.Range == 0 {
		return NoSignal
	}

	price := bar.Close
	rng := bar.High - bar.Low
	if rng == 0 {
		return NoSignal
	}

	cbdrRange := cbdr.Range
	dayHigh := s.dayHighSeen[dateKey]
	dayLow := s.dayLowSeen[dateKey]

	// Check if the day has already exceeded max_std on either side (runner day)
	if dayHigh > cbdr.High {
		stdUpDay := (dayHigh - cbdr.High) / cbdrRange
		if stdUpDay > s.maxSTD {
			return NoSignal // runner day on the upside
		}
	}
	if dayLow < cbdr.Low {
		stdDownDay := (cbdr.Low - dayLow) / cbdrRange
		if stdDownDay > s.maxSTD {
			return NoSignal // runner day on the downside
		}
	}

	// Bullish: price reaches projected LOD (STD level below CBDR low) with reversal
	if price < cbdr.Low {
		stdDown := (cbdr.Low - price) / cbdrRange
		if stdDown >= s.minSTD && stdDown <= s.maxSTD {
			// This level is the projected LOD; check for bullish displacement reversal
			body := bar.Close - bar.Open
			if body > 0 && body >= s.dispMult*s.atr[i] && body/rng >= s.bodyRatio {
				s.lastSigBar = i
				s.daySignaled[dateKey] = true
				return BuySignal
			}
		}
	}

	// Bearish: price reaches projected HOD (STD level above CBDR high) with reversal
	if price > cbdr.High {
		stdUp := (price - cbdr.High) / cbdrRange
		if stdUp >= s.minSTD && stdUp <= s.maxSTD {
			// This level is the projected HOD; check for bearish displacement reversal
			body := bar.Open - bar.Close
			if body > 0 && body >= s.dispMult*s.atr[i] && body/rng >= s.bodyRatio {
				s.lastSigBar = i
				s.daySignaled[dateKey] = true
				return SellSignal
			}
		}
	}

	return NoSignal
}
