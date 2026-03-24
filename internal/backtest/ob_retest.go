package backtest

import (
	"math"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── OB Retest Strategy ────────────────────────────────────────────────────
//
// Order Block Retest: Detect order blocks, then enter when price retraces
// back into an unmitigated OB zone. The OB acts as a supply/demand zone.
//
// Bullish: price retraces into a bullish (unmitigated) order block → buy.
// Bearish: price retraces into a bearish (unmitigated) order block → sell.
//
// Additional filters:
//   - OB must be at least `min_ob_age` bars old (confirmed, not fresh)
//   - ATR-based displacement must have occurred after OB formation
//   - Price must be in discount (for bullish) or premium (for bearish)
//
// Parameters:
//   swing_period   – swing detection period (default 5)
//   atr_period     – ATR period (default 14)
//   ob_impulse     – ATR multiple for OB impulse candle (default 1.5)
//   min_ob_age     – minimum bars since OB formation (default 5)
//   pd_lookback    – premium/discount lookback (default 50)
//   use_pd_filter  – require premium/discount alignment (default 1)

type OBRetestStrategy struct {
	bars       []data.OHLCV
	atr        []float64
	pdResult   indicators.PremiumDiscountResult
	pdZones    []indicators.PDZone // all detected PD arrays
	obZones    []indicators.PDZone // just order blocks
	minOBAge   int
	usePDFilter bool
	lastSigBar int
	swingPeriod int
}

func (s *OBRetestStrategy) Name() string { return "OB Retest" }
func (s *OBRetestStrategy) Description() string {
	return "ICT Order Block Retest: enter on retrace into unmitigated OB zone"
}

func (s *OBRetestStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	s.swingPeriod = int(getParam(params, "swing_period", 5))
	atrPeriod := int(getParam(params, "atr_period", 14))
	s.minOBAge = int(getParam(params, "min_ob_age", 5))
	pdLookback := int(getParam(params, "pd_lookback", 50))
	s.usePDFilter = getParam(params, "use_pd_filter", 1) == 1
	s.lastSigBar = -10

	obImpulse := getParam(params, "ob_impulse", 1.5)

	s.atr = indicators.ATR(bars, atrPeriod)
	s.pdResult = indicators.ComputePremiumDiscount(bars, pdLookback)

	// Detect all PD arrays
	pdParams := indicators.DefaultPDParams()
	pdParams.ATRPeriod = atrPeriod
	pdParams.OBImpulseMult = obImpulse
	pdParams.SwingPeriod = s.swingPeriod
	s.pdZones = indicators.DetectAllPDArrays(bars, pdParams)

	// Extract just order blocks
	for _, z := range s.pdZones {
		if z.Type == indicators.PDOrderBlock {
			s.obZones = append(s.obZones, z)
		}
	}
}

func (s *OBRetestStrategy) Signal(i int) SignalType {
	if i < s.swingPeriod*2+3 || math.IsNaN(s.atr[i]) {
		return NoSignal
	}
	if i-s.lastSigBar < s.swingPeriod {
		return NoSignal
	}

	price := s.bars[i].Close
	barHigh := s.bars[i].High
	barLow := s.bars[i].Low

	for _, ob := range s.obZones {
		// Must be old enough
		if i-ob.Index < s.minOBAge {
			continue
		}
		// Must not be in the future
		if ob.Index > i {
			continue
		}

		// Check if bar touches the OB zone
		touchesZone := barLow <= ob.Top && barHigh >= ob.Bottom

		if !touchesZone {
			continue
		}

		// Check mitigation: skip only if mitigated before current bar.
		// Do NOT check ob.Mitigated (precomputed over entire dataset = look-ahead bias).
		if ob.MitIndex >= 0 && ob.MitIndex < i {
			continue
		}

		// Bullish OB retest
		if ob.Direction == indicators.Bullish {
			// PD filter: should be in discount zone
			if s.usePDFilter && i < len(s.pdResult.InDiscount) && !s.pdResult.InDiscount[i] {
				continue
			}
			// Price should be touching from above (retracing down into OB)
			if price >= ob.Bottom {
				s.lastSigBar = i
				return BuySignal
			}
		}

		// Bearish OB retest
		if ob.Direction == indicators.Bearish {
			// PD filter: should be in premium zone
			if s.usePDFilter && i < len(s.pdResult.InPremium) && !s.pdResult.InPremium[i] {
				continue
			}
			if price <= ob.Top {
				s.lastSigBar = i
				return SellSignal
			}
		}
	}
	return NoSignal
}
