package backtest

import (
	"math"
	"time"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── Close Proximity Entry (CPE) Strategy ────────────────────────────────
//
// Tracks session opening prices (NY open at 07:00 EST or London open at
// 02:00 EST) and waits for price to displace AWAY from the session open,
// then retraces back NEAR the open with a reversal displacement candle.
//
// Bullish: price opens, dips below session open, then displaces up near
// the open price. Bearish: price opens, rallies above session open, then
// displaces down near the open price.
//
// Parameters:
//   swing_period   – swing detection lookback (default 5)
//   atr_period     – ATR for displacement sizing (default 14)
//   disp_mult      – displacement threshold as ATR multiple (default 1.0)
//   body_ratio     – min body/range ratio for displacement (default 0.5)
//   proximity_atr  – max distance to session open for re-entry (default 0.5)

type CPEEntryStrategy struct {
	bars        []data.OHLCV
	atr         []float64
	swingHighs  []float64
	swingLows   []float64
	swingPeriod int
	dispMult    float64
	bodyRatio   float64
	proxATR     float64
	lastSigBar  int
	est         *time.Location

	// Per-day session open tracking
	dayOpens map[string]*cpeDay
}

type cpeDay struct {
	londonOpen float64
	nyOpen     float64
	hasLondon  bool
	hasNY      bool
	// Track whether price displaced away from each open
	londonDisplacedBelow bool
	londonDisplacedAbove bool
	nyDisplacedBelow     bool
	nyDisplacedAbove     bool
}

func (s *CPEEntryStrategy) Name() string { return "CPE Entry" }
func (s *CPEEntryStrategy) Description() string {
	return "Close Proximity Entry: session open retrace after displacement away, with reversal displacement near open"
}

func (s *CPEEntryStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	s.swingPeriod = int(getParam(params, "swing_period", 5))
	atrPeriod := int(getParam(params, "atr_period", 14))
	s.dispMult = getParam(params, "disp_mult", 1.0)
	s.bodyRatio = getParam(params, "body_ratio", 0.5)
	s.proxATR = getParam(params, "proximity_atr", 0.5)
	s.lastSigBar = -20
	s.est = time.FixedZone("EST", -5*3600)

	s.atr = indicators.ATR(bars, atrPeriod)
	s.swingHighs = indicators.SwingHighs(bars, s.swingPeriod)
	s.swingLows = indicators.SwingLows(bars, s.swingPeriod)

	s.precomputeOpens()
}

func (s *CPEEntryStrategy) tradingDateKey(t time.Time) string {
	if t.Hour() >= 18 {
		return t.Add(24 * time.Hour).Format("2006-01-02")
	}
	return t.Format("2006-01-02")
}

func (s *CPEEntryStrategy) precomputeOpens() {
	s.dayOpens = make(map[string]*cpeDay)

	for _, bar := range s.bars {
		t := bar.Time.In(s.est)
		h := t.Hour()
		m := t.Minute()
		dateKey := s.tradingDateKey(t)

		dd, exists := s.dayOpens[dateKey]
		if !exists {
			dd = &cpeDay{}
			s.dayOpens[dateKey] = dd
		}

		// London open: first bar at 02:00 EST
		if h == 2 && m < 15 && !dd.hasLondon {
			dd.londonOpen = bar.Open
			dd.hasLondon = true
		}

		// NY open: first bar at 07:00 EST
		if h == 7 && m < 15 && !dd.hasNY {
			dd.nyOpen = bar.Open
			dd.hasNY = true
		}

		// Track displacement away from London open
		if dd.hasLondon && h >= 2 && h < 7 {
			if bar.Low < dd.londonOpen {
				dd.londonDisplacedBelow = true
			}
			if bar.High > dd.londonOpen {
				dd.londonDisplacedAbove = true
			}
		}

		// Track displacement away from NY open
		if dd.hasNY && h >= 7 && h < 16 {
			if bar.Low < dd.nyOpen {
				dd.nyDisplacedBelow = true
			}
			if bar.High > dd.nyOpen {
				dd.nyDisplacedAbove = true
			}
		}
	}
}

func (s *CPEEntryStrategy) getDayData(i int) *cpeDay {
	t := s.bars[i].Time.In(s.est)
	dateKey := s.tradingDateKey(t)
	return s.dayOpens[dateKey]
}

func (s *CPEEntryStrategy) Signal(i int) SignalType {
	if i < s.swingPeriod*2+3 || math.IsNaN(s.atr[i]) || s.atr[i] == 0 {
		return NoSignal
	}
	if i-s.lastSigBar < s.swingPeriod*2 {
		return NoSignal
	}

	bar := s.bars[i]
	t := bar.Time.In(s.est)
	h := t.Hour()

	dd := s.getDayData(i)
	if dd == nil {
		return NoSignal
	}

	rng := bar.High - bar.Low
	if rng == 0 {
		return NoSignal
	}

	proxDist := s.proxATR * s.atr[i]

	// Check NY open proximity entry (during NY session 07:00-16:00)
	if dd.hasNY && h >= 7 && h < 16 {
		sig := s.checkProximityEntry(i, dd.nyOpen, dd.nyDisplacedBelow, dd.nyDisplacedAbove, proxDist, rng)
		if sig != NoSignal {
			s.lastSigBar = i
			return sig
		}
	}

	// Check London open proximity entry (during London/early NY 02:00-10:00)
	if dd.hasLondon && h >= 2 && h < 10 {
		sig := s.checkProximityEntry(i, dd.londonOpen, dd.londonDisplacedBelow, dd.londonDisplacedAbove, proxDist, rng)
		if sig != NoSignal {
			s.lastSigBar = i
			return sig
		}
	}

	return NoSignal
}

func (s *CPEEntryStrategy) checkProximityEntry(i int, sessionOpen float64, displacedBelow, displacedAbove bool, proxDist, rng float64) SignalType {
	bar := s.bars[i]

	// Bullish CPE: price displaced below session open, now retracing near it with bullish displacement
	if displacedBelow {
		dist := math.Abs(bar.Close - sessionOpen)
		if dist <= proxDist {
			body := bar.Close - bar.Open
			if body > 0 && body >= s.dispMult*s.atr[i] && body/rng >= s.bodyRatio {
				return BuySignal
			}
		}
	}

	// Bearish CPE: price displaced above session open, now retracing near it with bearish displacement
	if displacedAbove {
		dist := math.Abs(bar.Close - sessionOpen)
		if dist <= proxDist {
			body := bar.Open - bar.Close
			if body > 0 && body >= s.dispMult*s.atr[i] && body/rng >= s.bodyRatio {
				return SellSignal
			}
		}
	}

	return NoSignal
}
