package backtest

import (
	"math"
	"time"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── Flout Strategy (CBDR + Asian Range Combined) ─────────────────────────
//
// "Flout" combines the CBDR range (18:00-00:00 EST) with the full Asian
// session range (19:00-00:00 EST) to produce a more accurate daily
// projection envelope. The Flout range is either the average or the wider
// of the two ranges (controlled by flout_mode).
//
// STD levels are projected from the Flout range instead of just CBDR.
// Entry occurs at Flout STD levels with displacement reversal, only
// during London/NY sessions (02:00-16:00 EST).
//
// Parameters:
//   swing_period – signal throttle period (default 5)
//   atr_period   – ATR period (default 14)
//   disp_mult    – displacement ATR multiple (default 0.5)
//   body_ratio   – min body/range ratio (default 0.4)
//   min_std      – minimum STD level to enter (default 1.0)
//   max_std      – maximum STD (skip beyond, likely trend day) (default 2.5)
//   flout_mode   – 0 = average of CBDR & Asian range, 1 = wider of two (default 0)

// floutData holds the combined Flout range for a trading day.
type floutData struct {
	date     time.Time
	high     float64 // upper boundary of the Flout zone
	low      float64 // lower boundary of the Flout zone
	floutRng float64 // the Flout range used for STD projections
}

// FloutStrategy implements mean-reversion at combined CBDR+Asian STD levels.
type FloutStrategy struct {
	bars        []data.OHLCV
	atr         []float64
	flouts      []floutData
	swingPeriod int
	dispMult    float64
	bodyRatio   float64
	minSTD      float64
	maxSTD      float64
	lastSigBar  int
	est         *time.Location
}

func (s *FloutStrategy) Name() string { return "Flout" }
func (s *FloutStrategy) Description() string {
	return "CBDR + Asian range combined (Flout) STD projections with displacement reversal"
}

func (s *FloutStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	s.swingPeriod = int(getParam(params, "swing_period", 5))
	atrPeriod := int(getParam(params, "atr_period", 14))
	s.dispMult = getParam(params, "disp_mult", 0.5)
	s.bodyRatio = getParam(params, "body_ratio", 0.4)
	s.minSTD = getParam(params, "min_std", 1.0)
	s.maxSTD = getParam(params, "max_std", 2.5)
	floutMode := int(getParam(params, "flout_mode", 0))
	s.lastSigBar = -10
	s.est = time.FixedZone("EST", -5*3600)

	s.atr = indicators.ATR(bars, atrPeriod)
	s.flouts = s.computeFlout(bars, floutMode)
}

// computeFlout builds per-day Flout data by combining CBDR (18:00-00:00)
// and the extended Asian range (19:00-00:00). Both windows overlap
// heavily; the CBDR starts at 18:00 while the extended Asian starts at
// 19:00. We compute each independently to capture the subtle difference.
func (s *FloutStrategy) computeFlout(bars []data.OHLCV, mode int) []floutData {
	est := s.est

	type dayRange struct {
		date                     time.Time
		cbdrHigh, cbdrLow        float64
		asianHigh, asianLow      float64
		hasCBDR, hasAsian        bool
	}

	days := make(map[string]*dayRange)
	var dayOrder []string

	for _, bar := range bars {
		t := bar.Time.In(est)
		h := t.Hour()

		// CBDR window: 18:00-23:59 EST → next day's session
		if h >= 18 {
			nextDay := t.Add(24 * time.Hour)
			dateKey := nextDay.Format("2006-01-02")
			d, exists := days[dateKey]
			if !exists {
				d = &dayRange{
					date:      nextDay.Truncate(24 * time.Hour),
					cbdrHigh:  bar.High,
					cbdrLow:   bar.Low,
					asianHigh: -math.MaxFloat64,
					asianLow:  math.MaxFloat64,
				}
				days[dateKey] = d
				dayOrder = append(dayOrder, dateKey)
			}

			// CBDR: full 18:00-23:59
			if bar.High > d.cbdrHigh {
				d.cbdrHigh = bar.High
			}
			if bar.Low < d.cbdrLow {
				d.cbdrLow = bar.Low
			}
			d.hasCBDR = true

			// Extended Asian: 19:00-23:59
			if h >= 19 {
				if bar.High > d.asianHigh {
					d.asianHigh = bar.High
				}
				if bar.Low < d.asianLow {
					d.asianLow = bar.Low
				}
				d.hasAsian = true
			}
		}
	}

	var results []floutData
	for _, key := range dayOrder {
		d := days[key]
		if !d.hasCBDR {
			continue
		}

		cbdrRange := d.cbdrHigh - d.cbdrLow
		asianRange := 0.0
		if d.hasAsian && d.asianHigh > -math.MaxFloat64 {
			asianRange = d.asianHigh - d.asianLow
		}

		// Compute the Flout range
		var fRange float64
		if !d.hasAsian || asianRange == 0 {
			fRange = cbdrRange
		} else if mode == 1 {
			// Wider of the two
			fRange = math.Max(cbdrRange, asianRange)
		} else {
			// Average of the two
			fRange = (cbdrRange + asianRange) / 2.0
		}

		// Use the overall high/low envelope
		high := d.cbdrHigh
		low := d.cbdrLow
		if d.hasAsian {
			if d.asianHigh > high && d.asianHigh > -math.MaxFloat64 {
				high = d.asianHigh
			}
			if d.asianLow < low && d.asianLow < math.MaxFloat64 {
				low = d.asianLow
			}
		}

		if fRange > 0 {
			results = append(results, floutData{
				date:     d.date,
				high:     high,
				low:      low,
				floutRng: fRange,
			})
		}
	}
	return results
}

// findFloutForBar returns the floutData for the given bar's trading day, or nil.
func (s *FloutStrategy) findFloutForBar(bar data.OHLCV) *floutData {
	t := bar.Time.In(s.est)
	var dateKey string
	if t.Hour() >= 18 {
		dateKey = t.Add(24 * time.Hour).Format("2006-01-02")
	} else {
		dateKey = t.Format("2006-01-02")
	}
	for i := range s.flouts {
		if s.flouts[i].date.Format("2006-01-02") == dateKey {
			return &s.flouts[i]
		}
	}
	return nil
}

func (s *FloutStrategy) Signal(i int) SignalType {
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

	fl := s.findFloutForBar(bar)
	if fl == nil || fl.floutRng == 0 {
		return NoSignal
	}

	price := bar.Close
	rng := bar.High - bar.Low
	if rng == 0 {
		return NoSignal
	}

	// Bullish: price is STD below Flout low
	if price < fl.low {
		stdDown := (fl.low - price) / fl.floutRng
		if stdDown >= s.minSTD && stdDown <= s.maxSTD {
			body := bar.Close - bar.Open
			if body > 0 && body >= s.dispMult*s.atr[i] && body/rng >= s.bodyRatio {
				s.lastSigBar = i
				return BuySignal
			}
		}
	}

	// Bearish: price is STD above Flout high
	if price > fl.high {
		stdUp := (price - fl.high) / fl.floutRng
		if stdUp >= s.minSTD && stdUp <= s.maxSTD {
			body := bar.Open - bar.Close
			if body > 0 && body >= s.dispMult*s.atr[i] && body/rng >= s.bodyRatio {
				s.lastSigBar = i
				return SellSignal
			}
		}
	}

	return NoSignal
}
