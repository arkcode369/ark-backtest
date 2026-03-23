package backtest

import (
	"math"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── Consequent Encroachment Entry Strategy ──────────────────────────────
//
// CE = midpoint (Consequent Encroachment) of a Fair Value Gap. Price is
// attracted to the CE level — the 50 % point of an FVG.
//
// Implementation:
//   1. Detect all FVGs via DetectAllPDArrays.
//   2. For each unmitigated FVG, the CE is the MidPoint field.
//   3. When price touches the CE level (within ATR-based tolerance),
//      enter in the FVG's direction:
//      - Bullish FVG CE → BUY
//      - Bearish FVG CE → SELL
//   4. A max_fvg_age filter discards stale FVGs.
//
// Parameters:
//   swing_period    – swing detection period (default 5)
//   atr_period      – ATR period (default 14)
//   ob_impulse      – OB impulse mult passed to PD detector (default 1.5)
//   max_fvg_age     – max bars since FVG formation (default 30)
//   touch_tolerance – fraction of ATR for CE touch (default 0.2)

type CEEntryStrategy struct {
	bars           []data.OHLCV
	atr            []float64
	fvgZones       []indicators.PDZone
	swingPeriod    int
	maxFVGAge      int
	touchTolerance float64
	lastSigBar     int
}

func (s *CEEntryStrategy) Name() string { return "CE Entry" }
func (s *CEEntryStrategy) Description() string {
	return "Consequent Encroachment: enter when price touches the midpoint of an unmitigated FVG"
}

func (s *CEEntryStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	s.swingPeriod = int(getParam(params, "swing_period", 5))
	atrPeriod := int(getParam(params, "atr_period", 14))
	obImpulse := getParam(params, "ob_impulse", 1.5)
	s.maxFVGAge = int(getParam(params, "max_fvg_age", 30))
	s.touchTolerance = getParam(params, "touch_tolerance", 0.2)
	s.lastSigBar = -10

	s.atr = indicators.ATR(bars, atrPeriod)

	pdParams := indicators.DefaultPDParams()
	pdParams.ATRPeriod = atrPeriod
	pdParams.OBImpulseMult = obImpulse
	pdParams.SwingPeriod = s.swingPeriod
	allZones := indicators.DetectAllPDArrays(bars, pdParams)

	// Keep only FVGs.
	for _, z := range allZones {
		if z.Type == indicators.PDFairValueGap {
			s.fvgZones = append(s.fvgZones, z)
		}
	}
}

func (s *CEEntryStrategy) Signal(i int) SignalType {
	if i < s.swingPeriod*2+3 || math.IsNaN(s.atr[i]) || s.atr[i] == 0 {
		return NoSignal
	}
	if i-s.lastSigBar < s.swingPeriod {
		return NoSignal
	}

	barHigh := s.bars[i].High
	barLow := s.bars[i].Low
	tol := s.touchTolerance * s.atr[i]

	for _, fvg := range s.fvgZones {
		// Must be in the past.
		if fvg.Index > i {
			continue
		}
		// Age filter.
		if i-fvg.Index > s.maxFVGAge {
			continue
		}
		// Must not be mitigated before current bar.
		if fvg.Mitigated && fvg.MitIndex < i {
			continue
		}

		ce := fvg.MidPoint

		// Check if bar touches the CE level within tolerance.
		if barLow > ce+tol || barHigh < ce-tol {
			continue
		}

		if fvg.Direction == indicators.Bullish {
			s.lastSigBar = i
			return BuySignal
		}
		if fvg.Direction == indicators.Bearish {
			s.lastSigBar = i
			return SellSignal
		}
	}

	return NoSignal
}
