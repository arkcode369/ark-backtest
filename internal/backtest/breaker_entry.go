package backtest

import (
	"math"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── Breaker Entry Strategy ────────────────────────────────────────────────
//
// ICT Breaker Block: A breaker is a failed order block that has been
// violated. Once the OB fails, it "flips" — what was support becomes
// resistance (bearish breaker) or vice versa. Enter when price retests
// the breaker zone.
//
// Bullish Breaker: A bearish OB that gets broken above → zone flips
//   to support → enter long on retest.
// Bearish Breaker: A bullish OB that gets broken below → zone flips
//   to resistance → enter short on retest.
//
// Parameters:
//   swing_period  – swing detection period (default 5)
//   atr_period    – ATR period (default 14)
//   ob_impulse    – OB impulse ATR multiple (default 1.5)
//   min_age       – min bars since breaker formed (default 3)
//   pd_lookback   – premium/discount lookback (default 50)
//   use_pd_filter – require premium/discount alignment (default 0)

type BreakerEntryStrategy struct {
	bars         []data.OHLCV
	atr          []float64
	pdResult     indicators.PremiumDiscountResult
	breakerZones []indicators.PDZone
	minAge       int
	usePDFilter  bool
	lastSigBar   int
	swingPeriod  int
}

func (s *BreakerEntryStrategy) Name() string { return "Breaker Entry" }
func (s *BreakerEntryStrategy) Description() string {
	return "ICT Breaker Block: enter on retest of failed (flipped) order block"
}

func (s *BreakerEntryStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	s.swingPeriod = int(getParam(params, "swing_period", 5))
	atrPeriod := int(getParam(params, "atr_period", 14))
	s.minAge = int(getParam(params, "min_age", 3))
	pdLookback := int(getParam(params, "pd_lookback", 50))
	s.usePDFilter = getParam(params, "use_pd_filter", 0) == 1
	s.lastSigBar = -10
	obImpulse := getParam(params, "ob_impulse", 1.5)

	s.atr = indicators.ATR(bars, atrPeriod)
	s.pdResult = indicators.ComputePremiumDiscount(bars, pdLookback)

	pdParams := indicators.DefaultPDParams()
	pdParams.ATRPeriod = atrPeriod
	pdParams.OBImpulseMult = obImpulse
	pdParams.SwingPeriod = s.swingPeriod
	allZones := indicators.DetectAllPDArrays(bars, pdParams)

	for _, z := range allZones {
		if z.Type == indicators.PDBreakerBlock {
			s.breakerZones = append(s.breakerZones, z)
		}
	}
}

func (s *BreakerEntryStrategy) Signal(i int) SignalType {
	if i < s.swingPeriod*2+3 || math.IsNaN(s.atr[i]) {
		return NoSignal
	}
	if i-s.lastSigBar < s.swingPeriod {
		return NoSignal
	}

	price := s.bars[i].Close
	barHigh := s.bars[i].High
	barLow := s.bars[i].Low

	for _, bk := range s.breakerZones {
		if bk.Index > i {
			continue
		}
		if i-bk.Index < s.minAge {
			continue
		}

		// Check if bar touches breaker zone
		if barLow > bk.Top || barHigh < bk.Bottom {
			continue
		}

		// Skip if already mitigated before current bar
		if bk.Mitigated && bk.MitIndex < i {
			continue
		}

		// Bullish breaker retest
		if bk.Direction == indicators.Bullish {
			if s.usePDFilter && i < len(s.pdResult.InDiscount) && !s.pdResult.InDiscount[i] {
				continue
			}
			if price >= bk.Bottom {
				s.lastSigBar = i
				return BuySignal
			}
		}

		// Bearish breaker retest
		if bk.Direction == indicators.Bearish {
			if s.usePDFilter && i < len(s.pdResult.InPremium) && !s.pdResult.InPremium[i] {
				continue
			}
			if price <= bk.Top {
				s.lastSigBar = i
				return SellSignal
			}
		}
	}
	return NoSignal
}
