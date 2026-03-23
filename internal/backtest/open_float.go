package backtest

import (
	"math"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── Open Float Liquidity Pools Strategy ─────────────────────────────────
//
// Institutional liquidity pools form at multi-day highs and lows where
// stop orders cluster. This strategy tracks 20-day, 40-day, and 60-day
// highs and lows. When price sweeps past one of these levels and reverses
// with displacement, it signals an entry in the reversal direction.
//
// Bullish: price sweeps below a multi-day low, then produces bullish
//          displacement → BUY.
// Bearish: price sweeps above a multi-day high, then produces bearish
//          displacement → SELL.
//
// Parameters:
//   swing_period – swing detection period (default 5)
//   atr_period   – ATR period (default 14)
//   disp_mult    – displacement ATR multiple (default 1.5)
//   body_ratio   – min body/range ratio for displacement (default 0.6)
//   pool_20      – enable 20-day pool (1=on, default 1)
//   pool_40      – enable 40-day pool (1=on, default 1)
//   pool_60      – enable 60-day pool (1=on, default 1)

type OpenFloatStrategy struct {
	bars        []data.OHLCV
	atr         []float64
	swingPeriod int
	dispMult    float64
	bodyRatio   float64
	poolPeriods []int
	lastSigBar  int
	// Rolling highs/lows per pool period. Keyed by period, each is length n.
	rollingHighs map[int][]float64
	rollingLows  map[int][]float64
}

func (s *OpenFloatStrategy) Name() string { return "Open Float" }
func (s *OpenFloatStrategy) Description() string {
	return "Open Float Liquidity Pools: sweep of multi-day H/L with displacement reversal"
}

func (s *OpenFloatStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	s.swingPeriod = int(getParam(params, "swing_period", 5))
	atrPeriod := int(getParam(params, "atr_period", 14))
	s.dispMult = getParam(params, "disp_mult", 1.5)
	s.bodyRatio = getParam(params, "body_ratio", 0.6)
	s.lastSigBar = -10

	s.atr = indicators.ATR(bars, atrPeriod)

	// Determine which pool periods are enabled.
	s.poolPeriods = nil
	if getParam(params, "pool_20", 1) == 1 {
		s.poolPeriods = append(s.poolPeriods, 20)
	}
	if getParam(params, "pool_40", 1) == 1 {
		s.poolPeriods = append(s.poolPeriods, 40)
	}
	if getParam(params, "pool_60", 1) == 1 {
		s.poolPeriods = append(s.poolPeriods, 60)
	}

	n := len(bars)
	s.rollingHighs = make(map[int][]float64)
	s.rollingLows = make(map[int][]float64)

	for _, period := range s.poolPeriods {
		highs := make([]float64, n)
		lows := make([]float64, n)
		for i := range highs {
			highs[i] = math.NaN()
			lows[i] = math.NaN()
		}
		for i := period - 1; i < n; i++ {
			hi := bars[i].High
			lo := bars[i].Low
			for j := i - period + 1; j < i; j++ {
				if bars[j].High > hi {
					hi = bars[j].High
				}
				if bars[j].Low < lo {
					lo = bars[j].Low
				}
			}
			highs[i] = hi
			lows[i] = lo
		}
		s.rollingHighs[period] = highs
		s.rollingLows[period] = lows
	}
}

func (s *OpenFloatStrategy) Signal(i int) SignalType {
	if i < 2 || math.IsNaN(s.atr[i]) || s.atr[i] == 0 {
		return NoSignal
	}
	if i-s.lastSigBar < s.swingPeriod {
		return NoSignal
	}

	bar := s.bars[i]
	prev := s.bars[i-1]
	rng := bar.High - bar.Low
	if rng == 0 {
		return NoSignal
	}

	bullBody := bar.Close - bar.Open
	bearBody := bar.Open - bar.Close
	isBullDisp := bullBody >= s.dispMult*s.atr[i] && bullBody/rng >= s.bodyRatio
	isBearDisp := bearBody >= s.dispMult*s.atr[i] && bearBody/rng >= s.bodyRatio

	for _, period := range s.poolPeriods {
		if i < period {
			continue
		}
		poolHigh := s.rollingHighs[period][i-1] // use previous bar's rolling level
		poolLow := s.rollingLows[period][i-1]
		if math.IsNaN(poolHigh) || math.IsNaN(poolLow) {
			continue
		}

		// Bullish: previous bar swept below pool low, current bar is bullish displacement
		if prev.Low < poolLow && isBullDisp {
			s.lastSigBar = i
			return BuySignal
		}
		// Bearish: previous bar swept above pool high, current bar is bearish displacement
		if prev.High > poolHigh && isBearDisp {
			s.lastSigBar = i
			return SellSignal
		}
	}

	return NoSignal
}
