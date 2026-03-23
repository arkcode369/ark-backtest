package backtest

import (
	"math"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── IRL to ERL (Internal to External Range Liquidity) Strategy ──────────
//
// IRL = internal range liquidity: FVGs and OBs within the current range.
// ERL = external range liquidity: swing highs and lows (stop clusters).
//
// Price moves FROM IRL (retraces to FVG/OB) TOWARD ERL (targets swing H/L).
//
// Implementation:
//   1. Detect unmitigated FVGs and OBs as IRL targets via DetectAllPDArrays.
//   2. Detect recent swing highs/lows as ERL targets.
//   3. When price touches an IRL zone, enter in the direction of the
//      nearest ERL target:
//      - Bullish: price touches bullish FVG/OB, ERL target is swing high above.
//      - Bearish: price touches bearish FVG/OB, ERL target is swing low below.
//
// Parameters:
//   swing_period – swing detection period (default 5)
//   atr_period   – ATR period (default 14)
//   ob_impulse   – OB impulse ATR multiple (default 1.5)
//   pd_lookback  – max age (bars) for IRL zones & swing search (default 50)

type IRLERLStrategy struct {
	bars        []data.OHLCV
	atr         []float64
	swingHighs  []float64
	swingLows   []float64
	irlZones    []indicators.PDZone // FVGs + OBs
	swingPeriod int
	pdLookback  int
	lastSigBar  int
}

func (s *IRLERLStrategy) Name() string { return "IRL to ERL" }
func (s *IRLERLStrategy) Description() string {
	return "Internal to External Range Liquidity: enter at FVG/OB retracement targeting swing H/L"
}

func (s *IRLERLStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	s.swingPeriod = int(getParam(params, "swing_period", 5))
	atrPeriod := int(getParam(params, "atr_period", 14))
	obImpulse := getParam(params, "ob_impulse", 1.5)
	s.pdLookback = int(getParam(params, "pd_lookback", 50))
	s.lastSigBar = -10

	s.atr = indicators.ATR(bars, atrPeriod)
	s.swingHighs = indicators.SwingHighs(bars, s.swingPeriod)
	s.swingLows = indicators.SwingLows(bars, s.swingPeriod)

	pdParams := indicators.DefaultPDParams()
	pdParams.ATRPeriod = atrPeriod
	pdParams.OBImpulseMult = obImpulse
	pdParams.SwingPeriod = s.swingPeriod
	allZones := indicators.DetectAllPDArrays(bars, pdParams)

	// Keep only FVGs and OBs as IRL zones.
	for _, z := range allZones {
		if z.Type == indicators.PDFairValueGap || z.Type == indicators.PDOrderBlock {
			s.irlZones = append(s.irlZones, z)
		}
	}
}

func (s *IRLERLStrategy) Signal(i int) SignalType {
	if i < s.swingPeriod*2+3 || math.IsNaN(s.atr[i]) {
		return NoSignal
	}
	if i-s.lastSigBar < s.swingPeriod {
		return NoSignal
	}

	barHigh := s.bars[i].High
	barLow := s.bars[i].Low

	for _, zone := range s.irlZones {
		// Zone must be in the past and within lookback window.
		if zone.Index > i || i-zone.Index > s.pdLookback {
			continue
		}
		// Must not be mitigated before current bar.
		if zone.Mitigated && zone.MitIndex < i {
			continue
		}
		// Check if current bar touches the IRL zone.
		if barLow > zone.Top || barHigh < zone.Bottom {
			continue
		}

		if zone.Direction == indicators.Bullish {
			// Look for an ERL target above: nearest swing high above current price.
			erlTarget := s.findNearestSwingHigh(i, s.bars[i].Close)
			if erlTarget > 0 && erlTarget > s.bars[i].Close {
				s.lastSigBar = i
				return BuySignal
			}
		}

		if zone.Direction == indicators.Bearish {
			// Look for an ERL target below: nearest swing low below current price.
			erlTarget := s.findNearestSwingLow(i, s.bars[i].Close)
			if erlTarget > 0 && erlTarget < s.bars[i].Close {
				s.lastSigBar = i
				return SellSignal
			}
		}
	}

	return NoSignal
}

// findNearestSwingHigh returns the closest confirmed swing high above price
// within the lookback window, or 0 if none found.
func (s *IRLERLStrategy) findNearestSwingHigh(i int, price float64) float64 {
	best := 0.0
	bestDist := math.MaxFloat64
	start := i - s.pdLookback
	if start < 0 {
		start = 0
	}
	for j := start; j < i; j++ {
		if math.IsNaN(s.swingHighs[j]) {
			continue
		}
		sh := s.swingHighs[j]
		if sh > price {
			dist := sh - price
			if dist < bestDist {
				bestDist = dist
				best = sh
			}
		}
	}
	return best
}

// findNearestSwingLow returns the closest confirmed swing low below price
// within the lookback window, or 0 if none found.
func (s *IRLERLStrategy) findNearestSwingLow(i int, price float64) float64 {
	best := 0.0
	bestDist := math.MaxFloat64
	start := i - s.pdLookback
	if start < 0 {
		start = 0
	}
	for j := start; j < i; j++ {
		if math.IsNaN(s.swingLows[j]) {
			continue
		}
		sl := s.swingLows[j]
		if sl < price {
			dist := price - sl
			if dist < bestDist {
				bestDist = dist
				best = sl
			}
		}
	}
	return best
}
