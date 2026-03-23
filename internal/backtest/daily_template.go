package backtest

import (
	"math"
	"time"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── Daily Template Strategy ─────────────────────────────────────────────
//
// Classifies intraday price action into templates based on session ranges:
//
//   Template 1 – London Expansion + NY Continuation:
//     London makes the directional move, NY continues.
//   Template 2 – London Expansion + NY Reversal:
//     London makes the directional move, NY reverses.
//   Template 3 – Asia Expansion:
//     Big Asian move, London/NY follow through.
//   Template 4 – NY Only:
//     Flat in Asia/London, expansion begins in NY.
//
// Signal generation:
//   - If "NY continuation" template is detected → enter in London's direction during NY
//   - If "NY reversal" template is detected → enter opposite to London during NY
//   - Uses session range ratios to classify template type
//
// Parameters:
//   swing_period    – swing detection lookback (default 5)
//   atr_period      – ATR for displacement sizing (default 14)
//   disp_mult       – displacement threshold as ATR multiple (default 1.0)
//   body_ratio      – min body/range ratio for displacement (default 0.5)
//   template_mode   – 0=auto-detect, 1=continuation only, 2=reversal only (default 0)

type DailyTemplateStrategy struct {
	bars        []data.OHLCV
	atr         []float64
	swingHighs  []float64
	swingLows   []float64
	swingPeriod int
	dispMult    float64
	bodyRatio   float64
	mode        int // 0=auto, 1=continuation, 2=reversal
	lastSigBar  int
	est         *time.Location

	dayData map[string]*templateDay
}

type templateDay struct {
	// Asian session (18:00-02:00 EST)
	asianHigh float64
	asianLow  float64
	hasAsian  bool

	// London session (02:00-05:00 EST)
	londonOpen  float64
	londonClose float64
	londonHigh  float64
	londonLow   float64
	hasLondon   bool
	londonBias  int // +1 bullish, -1 bearish

	// NY session (07:00-16:00 EST)
	nyOpen float64
	hasNY  bool

	// Template classification
	template int // 0=unknown, 1=london+ny_cont, 2=london+ny_rev, 3=asia_exp, 4=ny_only
}

func (s *DailyTemplateStrategy) Name() string { return "Daily Template" }
func (s *DailyTemplateStrategy) Description() string {
	return "Daily Template: classify intraday into London-continuation, London-reversal, Asia-expansion, or NY-only patterns"
}

func (s *DailyTemplateStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	s.swingPeriod = int(getParam(params, "swing_period", 5))
	atrPeriod := int(getParam(params, "atr_period", 14))
	s.dispMult = getParam(params, "disp_mult", 1.0)
	s.bodyRatio = getParam(params, "body_ratio", 0.5)
	s.mode = int(getParam(params, "template_mode", 0))
	s.lastSigBar = -20
	s.est = time.FixedZone("EST", -5*3600)

	s.atr = indicators.ATR(bars, atrPeriod)
	s.swingHighs = indicators.SwingHighs(bars, s.swingPeriod)
	s.swingLows = indicators.SwingLows(bars, s.swingPeriod)

	s.precompute()
}

func (s *DailyTemplateStrategy) tradingDateKey(t time.Time) string {
	estTime := t.In(s.est)
	if estTime.Hour() >= 18 {
		return estTime.Add(24 * time.Hour).Format("2006-01-02")
	}
	return estTime.Format("2006-01-02")
}

func (s *DailyTemplateStrategy) precompute() {
	s.dayData = make(map[string]*templateDay)

	// First pass: collect session data
	for _, bar := range s.bars {
		t := bar.Time.In(s.est)
		h := t.Hour()
		m := t.Minute()
		dateKey := s.tradingDateKey(bar.Time)

		dd, exists := s.dayData[dateKey]
		if !exists {
			dd = &templateDay{
				asianHigh:  -math.MaxFloat64,
				asianLow:   math.MaxFloat64,
				londonHigh: -math.MaxFloat64,
				londonLow:  math.MaxFloat64,
			}
			s.dayData[dateKey] = dd
		}

		// Asian session: 18:00-02:00 EST
		if h >= 18 || h < 2 {
			if bar.High > dd.asianHigh {
				dd.asianHigh = bar.High
			}
			if bar.Low < dd.asianLow {
				dd.asianLow = bar.Low
			}
			dd.hasAsian = true
		}

		// London session: 02:00-05:00 EST
		if h >= 2 && h < 5 {
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
			dd.londonClose = bar.Close
		}

		// NY open
		if h == 7 && m < 15 && !dd.hasNY {
			dd.nyOpen = bar.Open
			dd.hasNY = true
		}
	}

	// Second pass: classify templates
	for _, dd := range s.dayData {
		if !dd.hasAsian || !dd.hasLondon {
			continue
		}

		// Compute ranges
		asianRange := dd.asianHigh - dd.asianLow
		londonRange := dd.londonHigh - dd.londonLow

		// Determine London bias
		if dd.londonClose > dd.londonOpen {
			dd.londonBias = 1
		} else if dd.londonClose < dd.londonOpen {
			dd.londonBias = -1
		}

		// Template classification by session range ratios
		if asianRange > 0 && londonRange > 0 {
			ratio := asianRange / londonRange

			if ratio > 1.5 {
				// Asia had a much larger range than London → Asia Expansion
				dd.template = 3
			} else if londonRange > asianRange*1.2 && dd.londonBias != 0 {
				// London expanded beyond Asian range → London Expansion
				// Actual continuation vs reversal is determined at NY open time
				dd.template = 1 // placeholder, will be refined at signal time
			} else {
				// Flat in both → NY Only template
				dd.template = 4
			}
		}
	}
}

func (s *DailyTemplateStrategy) Signal(i int) SignalType {
	if i < s.swingPeriod*2+3 || math.IsNaN(s.atr[i]) || s.atr[i] == 0 {
		return NoSignal
	}
	if i-s.lastSigBar < s.swingPeriod*2 {
		return NoSignal
	}

	bar := s.bars[i]
	t := bar.Time.In(s.est)
	h := t.Hour()

	// Only trade during NY session (07:00-16:00 EST)
	if h < 7 || h >= 16 {
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

	// Determine the real-time template at this point
	// For London Expansion days, check NY direction vs London
	var tradeDir int

	switch dd.template {
	case 1, 2:
		// London Expansion: determine continuation vs reversal dynamically
		// If NY price is moving with London → continuation
		// If NY price is moving against London → reversal
		if dd.hasNY {
			nyContinuation := false
			if dd.londonBias == 1 && bar.Close > dd.nyOpen {
				nyContinuation = true
			} else if dd.londonBias == -1 && bar.Close < dd.nyOpen {
				nyContinuation = true
			}

			if nyContinuation {
				// Continuation template
				if s.mode == 2 { // reversal only mode, skip
					return NoSignal
				}
				tradeDir = dd.londonBias
			} else {
				// Reversal template
				if s.mode == 1 { // continuation only mode, skip
					return NoSignal
				}
				tradeDir = -dd.londonBias
			}
		}

	case 3:
		// Asia Expansion: follow through in Asia's direction during NY
		// Determine Asia direction
		if dd.hasNY {
			asianMid := (dd.asianHigh + dd.asianLow) / 2.0
			if dd.londonClose > asianMid {
				tradeDir = 1
			} else {
				tradeDir = -1
			}
		}

	case 4:
		// NY Only: flat prior sessions, enter on first displacement
		// No prior bias, just enter on displacement in either direction
		bullBody := bar.Close - bar.Open
		bearBody := bar.Open - bar.Close
		if bullBody > 0 && bullBody >= s.dispMult*s.atr[i] && bullBody/rng >= s.bodyRatio {
			s.lastSigBar = i
			return BuySignal
		}
		if bearBody > 0 && bearBody >= s.dispMult*s.atr[i] && bearBody/rng >= s.bodyRatio {
			s.lastSigBar = i
			return SellSignal
		}
		return NoSignal

	default:
		return NoSignal
	}

	if tradeDir == 0 {
		return NoSignal
	}

	// Enter on displacement confirmation
	if tradeDir == 1 {
		body := bar.Close - bar.Open
		if body > 0 && body >= s.dispMult*s.atr[i] && body/rng >= s.bodyRatio {
			s.lastSigBar = i
			return BuySignal
		}
	}
	if tradeDir == -1 {
		body := bar.Open - bar.Close
		if body > 0 && body >= s.dispMult*s.atr[i] && body/rng >= s.bodyRatio {
			s.lastSigBar = i
			return SellSignal
		}
	}

	return NoSignal
}
