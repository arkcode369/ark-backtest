package backtest

import (
	"math"
	"time"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── London Close Kill Zone Strategy ─────────────────────────────────────
//
// Trades specifically during the London Close window (11:00-12:00 UTC).
// During this window, looks for reversals of the London session move.
//
// Logic:
//   1. Determine London session direction (02:00-05:00 EST open vs current price)
//   2. During London Close (11:00-12:00 UTC), enter opposite to London direction
//      with displacement confirmation
//
// Parameters:
//   swing_period  – swing detection lookback (default 5)
//   atr_period    – ATR for displacement sizing (default 14)
//   disp_mult     – displacement threshold as ATR multiple (default 1.0)
//   body_ratio    – min body/range ratio for displacement (default 0.5)

type LondonCloseStrategy struct {
	bars        []data.OHLCV
	atr         []float64
	swingHighs  []float64
	swingLows   []float64
	swingPeriod int
	dispMult    float64
	bodyRatio   float64
	lastSigBar  int
	est         *time.Location

	// Per-day London session tracking
	dayData map[string]*londonCloseDay
}

type londonCloseDay struct {
	londonOpen  float64
	londonHigh  float64
	londonLow   float64
	hasLondon   bool
	londonBias  int // +1 = bullish London, -1 = bearish London
}

func (s *LondonCloseStrategy) Name() string { return "London Close" }
func (s *LondonCloseStrategy) Description() string {
	return "London Close Kill Zone: reversal of London session move during 11:00-12:00 UTC close window"
}

func (s *LondonCloseStrategy) Init(bars []data.OHLCV, params map[string]float64) {
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

	s.precomputeLondon()
}

func (s *LondonCloseStrategy) tradingDateKey(t time.Time) string {
	estTime := t.In(s.est)
	if estTime.Hour() >= 18 {
		return estTime.Add(24 * time.Hour).Format("2006-01-02")
	}
	return estTime.Format("2006-01-02")
}

func (s *LondonCloseStrategy) precomputeLondon() {
	s.dayData = make(map[string]*londonCloseDay)

	for _, bar := range s.bars {
		t := bar.Time.In(s.est)
		h := t.Hour()
		m := t.Minute()
		dateKey := s.tradingDateKey(bar.Time)

		dd, exists := s.dayData[dateKey]
		if !exists {
			dd = &londonCloseDay{
				londonHigh: -math.MaxFloat64,
				londonLow:  math.MaxFloat64,
			}
			s.dayData[dateKey] = dd
		}

		// London Kill Zone: 02:00-05:00 EST
		if h >= 2 && h < 5 {
			// Capture London open (first bar of London session)
			if !dd.hasLondon && h == 2 && m < 15 {
				dd.londonOpen = bar.Open
				dd.hasLondon = true
			}
			if bar.High > dd.londonHigh {
				dd.londonHigh = bar.High
			}
			if bar.Low < dd.londonLow {
				dd.londonLow = bar.Low
			}
		}

		// Update London bias at end of London session and beyond
		if dd.hasLondon && h >= 5 {
			if dd.londonBias == 0 {
				if bar.Close > dd.londonOpen {
					dd.londonBias = 1 // bullish London
				} else if bar.Close < dd.londonOpen {
					dd.londonBias = -1 // bearish London
				}
			}
		}
	}

	// Second pass: finalize London bias using last London bar's close vs open
	for _, bar := range s.bars {
		t := bar.Time.In(s.est)
		h := t.Hour()
		dateKey := s.tradingDateKey(bar.Time)
		dd := s.dayData[dateKey]
		if dd == nil || !dd.hasLondon {
			continue
		}
		// Use closing price of London session to determine bias
		if h >= 4 && h < 5 {
			if bar.Close > dd.londonOpen {
				dd.londonBias = 1
			} else {
				dd.londonBias = -1
			}
		}
	}
}

func (s *LondonCloseStrategy) inLondonCloseWindow(t time.Time) bool {
	// London Close: 11:00-12:00 UTC
	utcH := t.UTC().Hour()
	return utcH == 11
}

func (s *LondonCloseStrategy) Signal(i int) SignalType {
	if i < s.swingPeriod*2+3 || math.IsNaN(s.atr[i]) || s.atr[i] == 0 {
		return NoSignal
	}
	if i-s.lastSigBar < s.swingPeriod*2 {
		return NoSignal
	}

	bar := s.bars[i]

	// Must be inside London Close window (11:00-12:00 UTC)
	if !s.inLondonCloseWindow(bar.Time) {
		return NoSignal
	}

	dateKey := s.tradingDateKey(bar.Time)
	dd := s.dayData[dateKey]
	if dd == nil || !dd.hasLondon || dd.londonBias == 0 {
		return NoSignal
	}

	rng := bar.High - bar.Low
	if rng == 0 {
		return NoSignal
	}

	// London was bullish → London Close reversal = bearish entry
	if dd.londonBias == 1 {
		body := bar.Open - bar.Close
		if body > 0 && body >= s.dispMult*s.atr[i] && body/rng >= s.bodyRatio {
			s.lastSigBar = i
			return SellSignal
		}
	}

	// London was bearish → London Close reversal = bullish entry
	if dd.londonBias == -1 {
		body := bar.Close - bar.Open
		if body > 0 && body >= s.dispMult*s.atr[i] && body/rng >= s.bodyRatio {
			s.lastSigBar = i
			return BuySignal
		}
	}

	return NoSignal
}
