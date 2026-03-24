package backtest

import (
	"math"
	"time"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── CBDR STD Strategy ─────────────────────────────────────────────────────
//
// Uses the Central Bank Dealers Range (Asian session range) and its
// standard deviation projections to identify high-probability reversal
// levels during London/NY sessions.
//
// Bullish: price reaches STD 1-2 DOWN projection → mean reversion buy.
// Bearish: price reaches STD 1-2 UP projection → mean reversion sell.
//
// The idea: 70% of days, price reverses from STD 1-2 of the CBDR range.
// STD 3+ is a "runner" day (trend day) where mean reversion fails.
//
// Parameters:
//   atr_period   – ATR period (default 14)
//   min_std      – minimum STD level to enter (default 1)
//   max_std      – maximum STD level (skip if beyond, likely trend day) (default 2.5)
//   disp_mult    – displacement for confirmation (default 0.5)
//   body_ratio   – min body/range (default 0.4)
//   swing_period – for throttling (default 5)

type CBDRSTDStrategy struct {
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
}

func (s *CBDRSTDStrategy) Name() string { return "CBDR STD" }
func (s *CBDRSTDStrategy) Description() string {
	return "ICT CBDR: mean-reversion at Asian range STD projections during London/NY"
}

func (s *CBDRSTDStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	s.swingPeriod = int(getParam(params, "swing_period", 5))
	atrPeriod := int(getParam(params, "atr_period", 14))
	s.dispMult = getParam(params, "disp_mult", 0.5)
	s.bodyRatio = getParam(params, "body_ratio", 0.4)
	s.minSTD = getParam(params, "min_std", 1.0)
	s.maxSTD = getParam(params, "max_std", 2.5)
	s.lastSigBar = -10
	s.est = estLoc

	s.atr = indicators.ATR(bars, atrPeriod)
	s.cbdrs = ComputeCBDR(bars)
}

func (s *CBDRSTDStrategy) Signal(i int) SignalType {
	if i < s.swingPeriod*2+3 || math.IsNaN(s.atr[i]) || s.atr[i] == 0 {
		return NoSignal
	}
	if i-s.lastSigBar < s.swingPeriod {
		return NoSignal
	}

	// Only trade during London/NY sessions (02:00-16:00 EST)
	t := s.bars[i].Time.In(s.est)
	h := t.Hour()
	if h < 2 || h >= 16 {
		return NoSignal
	}

	cbdr := FindCBDRForBar(s.cbdrs, s.bars[i])
	if cbdr == nil || cbdr.Range == 0 {
		return NoSignal
	}

	price := s.bars[i].Close
	rng := s.bars[i].High - s.bars[i].Low
	if rng == 0 {
		return NoSignal
	}

	// Calculate how many STDs the price is from the CBDR range
	cbdrRange := cbdr.Range

	// Bullish: price is STD 1-2 below Asian range
	if price < cbdr.Low {
		stdDown := (cbdr.Low - price) / cbdrRange
		if stdDown >= s.minSTD && stdDown <= s.maxSTD {
			// Confirm with bullish displacement
			body := s.bars[i].Close - s.bars[i].Open
			if body > 0 && body >= s.dispMult*s.atr[i] && body/rng >= s.bodyRatio {
				s.lastSigBar = i
				return BuySignal
			}
		}
	}

	// Bearish: price is STD 1-2 above Asian range
	if price > cbdr.High {
		stdUp := (price - cbdr.High) / cbdrRange
		if stdUp >= s.minSTD && stdUp <= s.maxSTD {
			// Confirm with bearish displacement
			body := s.bars[i].Open - s.bars[i].Close
			if body > 0 && body >= s.dispMult*s.atr[i] && body/rng >= s.bodyRatio {
				s.lastSigBar = i
				return SellSignal
			}
		}
	}

	return NoSignal
}
