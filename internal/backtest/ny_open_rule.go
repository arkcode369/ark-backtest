package backtest

import (
	"math"
	"time"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── NY Open Rule Strategy ───────────────────────────────────────────────
//
// Compares London session direction with NY open behaviour:
//   - If NY opens and continues in London's direction → continuation trade
//   - If NY opens and reverses London's direction → reversal trade
//
// Implementation:
//   1. Compute London session high/low (02:00-05:00 EST bars)
//   2. Determine London bias (close vs open)
//   3. At NY open (07:00+ EST), check if first NY bars align or diverge
//   4. Enter on displacement confirmation in the determined direction
//
// Parameters:
//   swing_period  – swing detection lookback (default 5)
//   atr_period    – ATR for displacement sizing (default 14)
//   disp_mult     – displacement threshold as ATR multiple (default 1.0)
//   body_ratio    – min body/range ratio for displacement (default 0.5)
//   mode          – 0=reversal only, 1=continuation only, 2=both (default 0)

type NYOpenRuleStrategy struct {
	bars        []data.OHLCV
	atr         []float64
	swingHighs  []float64
	swingLows   []float64
	swingPeriod int
	dispMult    float64
	bodyRatio   float64
	mode        int // 0=reversal, 1=continuation, 2=both
	lastSigBar  int
	est         *time.Location

	dayData map[string]*nyOpenDay
}

type nyOpenDay struct {
	londonOpen  float64
	londonClose float64
	londonHigh  float64
	londonLow   float64
	hasLondon   bool
	londonBias  int // +1 = bullish, -1 = bearish

	nyOpen    float64
	hasNY     bool
	nyBias    int // +1 = continuing London, -1 = reversing London, 0 = undetermined
	signalled bool
}

func (s *NYOpenRuleStrategy) Name() string { return "NY Open Rule" }
func (s *NYOpenRuleStrategy) Description() string {
	return "NY Open Rule: compare London direction with NY open for continuation or reversal trades"
}

func (s *NYOpenRuleStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	s.swingPeriod = int(getParam(params, "swing_period", 5))
	atrPeriod := int(getParam(params, "atr_period", 14))
	s.dispMult = getParam(params, "disp_mult", 1.0)
	s.bodyRatio = getParam(params, "body_ratio", 0.5)
	s.mode = int(getParam(params, "mode", 0))
	s.lastSigBar = -20
	s.est = time.FixedZone("EST", -5*3600)

	s.atr = indicators.ATR(bars, atrPeriod)
	s.swingHighs = indicators.SwingHighs(bars, s.swingPeriod)
	s.swingLows = indicators.SwingLows(bars, s.swingPeriod)

	s.precompute()
}

func (s *NYOpenRuleStrategy) tradingDateKey(t time.Time) string {
	estTime := t.In(s.est)
	if estTime.Hour() >= 18 {
		return estTime.Add(24 * time.Hour).Format("2006-01-02")
	}
	return estTime.Format("2006-01-02")
}

func (s *NYOpenRuleStrategy) precompute() {
	s.dayData = make(map[string]*nyOpenDay)

	for _, bar := range s.bars {
		t := bar.Time.In(s.est)
		h := t.Hour()
		m := t.Minute()
		dateKey := s.tradingDateKey(bar.Time)

		dd, exists := s.dayData[dateKey]
		if !exists {
			dd = &nyOpenDay{
				londonHigh: -math.MaxFloat64,
				londonLow:  math.MaxFloat64,
			}
			s.dayData[dateKey] = dd
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
			// Update London close to be the last bar's close in the session
			dd.londonClose = bar.Close
		}

		// Finalize London bias after London ends
		if dd.hasLondon && h == 5 && dd.londonBias == 0 {
			if dd.londonClose > dd.londonOpen {
				dd.londonBias = 1 // bullish London
			} else if dd.londonClose < dd.londonOpen {
				dd.londonBias = -1 // bearish London
			}
		}

		// NY open: first bar at 07:00 EST
		if h == 7 && m < 15 && !dd.hasNY {
			dd.nyOpen = bar.Open
			dd.hasNY = true
		}

		// Determine NY bias from early NY bars (07:00-08:00 EST)
		if dd.hasNY && dd.hasLondon && dd.londonBias != 0 && dd.nyBias == 0 && h >= 7 && h < 8 {
			if bar.Close > dd.nyOpen && dd.londonBias == 1 {
				dd.nyBias = 1 // continuation
			} else if bar.Close < dd.nyOpen && dd.londonBias == -1 {
				dd.nyBias = 1 // continuation
			} else if bar.Close < dd.nyOpen && dd.londonBias == 1 {
				dd.nyBias = -1 // reversal
			} else if bar.Close > dd.nyOpen && dd.londonBias == -1 {
				dd.nyBias = -1 // reversal
			}
		}
	}
}

func (s *NYOpenRuleStrategy) Signal(i int) SignalType {
	if i < s.swingPeriod*2+3 || math.IsNaN(s.atr[i]) || s.atr[i] == 0 {
		return NoSignal
	}
	if i-s.lastSigBar < s.swingPeriod*2 {
		return NoSignal
	}

	bar := s.bars[i]
	t := bar.Time.In(s.est)
	h := t.Hour()

	// Only trade during NY AM kill zone (07:00-10:00 EST)
	if h < 7 || h >= 10 {
		return NoSignal
	}

	dateKey := s.tradingDateKey(bar.Time)
	dd := s.dayData[dateKey]
	if dd == nil || !dd.hasLondon || !dd.hasNY || dd.londonBias == 0 || dd.nyBias == 0 {
		return NoSignal
	}

	// Mode filtering: 0=reversal, 1=continuation, 2=both
	isContinuation := dd.nyBias == 1
	isReversal := dd.nyBias == -1

	if s.mode == 0 && !isReversal {
		return NoSignal
	}
	if s.mode == 1 && !isContinuation {
		return NoSignal
	}

	rng := bar.High - bar.Low
	if rng == 0 {
		return NoSignal
	}

	// Determine trade direction
	var tradeDir int
	if isContinuation {
		tradeDir = dd.londonBias // same as London
	} else {
		tradeDir = -dd.londonBias // opposite to London
	}

	// Bullish entry
	if tradeDir == 1 {
		body := bar.Close - bar.Open
		if body > 0 && body >= s.dispMult*s.atr[i] && body/rng >= s.bodyRatio {
			s.lastSigBar = i
			return BuySignal
		}
	}

	// Bearish entry
	if tradeDir == -1 {
		body := bar.Open - bar.Close
		if body > 0 && body >= s.dispMult*s.atr[i] && body/rng >= s.bodyRatio {
			s.lastSigBar = i
			return SellSignal
		}
	}

	return NoSignal
}
