package backtest

import (
	"math"
	"time"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── Weekly Profile Strategy ───────────────────────────────────────────────
//
// Uses the weekly open, previous week high (PWH), and previous week low (PWL)
// as key reference levels for bias and entry.
//
// Bullish: price above weekly open + retraces to weekly open (support) +
//   displacement confirmation. Targets PWH.
// Bearish: price below weekly open + rallies to weekly open (resistance) +
//   displacement confirmation. Targets PWL.
//
// Parameters:
//   atr_period   – ATR period (default 14)
//   disp_mult    – displacement ATR multiple (default 1.0)
//   body_ratio   – min body/range (default 0.5)
//   swing_period – swing period (default 5)
//   touch_atr    – how close price must be to weekly open (ATR fraction, default 0.5)

type WeeklyProfileStrategy struct {
	bars        []data.OHLCV
	atr         []float64
	swingPeriod int
	dispMult    float64
	bodyRatio   float64
	touchATR    float64
	lastSigBar  int

	// Weekly levels per bar
	weeklyOpen []float64 // weekly open price for each bar
	pwh        []float64 // previous week high
	pwl        []float64 // previous week low
}

func (s *WeeklyProfileStrategy) Name() string { return "Weekly Profile" }
func (s *WeeklyProfileStrategy) Description() string {
	return "ICT Weekly Profile: trade retraces to weekly open using PWH/PWL as bias"
}

func (s *WeeklyProfileStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	s.swingPeriod = int(getParam(params, "swing_period", 5))
	atrPeriod := int(getParam(params, "atr_period", 14))
	s.dispMult = getParam(params, "disp_mult", 1.0)
	s.bodyRatio = getParam(params, "body_ratio", 0.5)
	s.touchATR = getParam(params, "touch_atr", 0.5)
	s.lastSigBar = -10

	s.atr = indicators.ATR(bars, atrPeriod)
	s.computeWeeklyLevels()
}

type weekData struct {
	open     float64
	high     float64
	low      float64
	isoYear  int
	isoWeek  int
}

func (s *WeeklyProfileStrategy) computeWeeklyLevels() {
	n := len(s.bars)
	s.weeklyOpen = make([]float64, n)
	s.pwh = make([]float64, n)
	s.pwl = make([]float64, n)

	// Collect weekly data
	var weeks []weekData
	weekMap := make(map[int]*weekData) // keyed by isoYear*100+isoWeek

	for _, bar := range s.bars {
		y, w := bar.Time.ISOWeek()
		key := y*100 + w
		wd, exists := weekMap[key]
		if !exists {
			wd = &weekData{
				open:    bar.Open,
				high:    bar.High,
				low:     bar.Low,
				isoYear: y,
				isoWeek: w,
			}
			weekMap[key] = wd
			weeks = append(weeks, *wd)
		} else {
			if bar.High > wd.high {
				wd.high = bar.High
			}
			if bar.Low < wd.low {
				wd.low = bar.Low
			}
		}
	}

	// Update weeks slice with final values
	for i := range weeks {
		key := weeks[i].isoYear*100 + weeks[i].isoWeek
		if wd, ok := weekMap[key]; ok {
			weeks[i].high = wd.high
			weeks[i].low = wd.low
		}
	}

	// Build ordered week index
	weekIndex := make(map[int]int) // key → index in weeks
	for i, w := range weeks {
		key := w.isoYear*100 + w.isoWeek
		weekIndex[key] = i
	}

	// Assign per-bar levels
	for i, bar := range s.bars {
		y, w := bar.Time.ISOWeek()
		key := y*100 + w
		idx, ok := weekIndex[key]
		if !ok {
			s.weeklyOpen[i] = math.NaN()
			s.pwh[i] = math.NaN()
			s.pwl[i] = math.NaN()
			continue
		}

		wd := weeks[idx]
		s.weeklyOpen[i] = wd.open

		if idx > 0 {
			prev := weeks[idx-1]
			s.pwh[i] = prev.high
			s.pwl[i] = prev.low
		} else {
			s.pwh[i] = math.NaN()
			s.pwl[i] = math.NaN()
		}
	}
}

func (s *WeeklyProfileStrategy) Signal(i int) SignalType {
	if i < s.swingPeriod*2+3 || math.IsNaN(s.atr[i]) {
		return NoSignal
	}
	if i-s.lastSigBar < s.swingPeriod {
		return NoSignal
	}
	if math.IsNaN(s.weeklyOpen[i]) || math.IsNaN(s.pwh[i]) || math.IsNaN(s.pwl[i]) {
		return NoSignal
	}

	// Only trade Mon-Fri (exclude weekend bars)
	if s.bars[i].Time.Weekday() == time.Saturday || s.bars[i].Time.Weekday() == time.Sunday {
		return NoSignal
	}

	price := s.bars[i].Close
	wo := s.weeklyOpen[i]
	rng := s.bars[i].High - s.bars[i].Low
	if rng == 0 || s.atr[i] == 0 {
		return NoSignal
	}

	touchDist := s.touchATR * s.atr[i]

	// Bullish: price near weekly open (from above) + price > weekly open → buy
	if price > wo && math.Abs(s.bars[i].Low-wo) <= touchDist {
		body := price - s.bars[i].Open
		if body > 0 && body >= s.dispMult*s.atr[i] && body/rng >= s.bodyRatio {
			s.lastSigBar = i
			return BuySignal
		}
	}

	// Bearish: price near weekly open (from below) + price < weekly open → sell
	if price < wo && math.Abs(s.bars[i].High-wo) <= touchDist {
		body := s.bars[i].Open - price
		if body > 0 && body >= s.dispMult*s.atr[i] && body/rng >= s.bodyRatio {
			s.lastSigBar = i
			return SellSignal
		}
	}

	return NoSignal
}
