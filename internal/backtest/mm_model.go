package backtest

import (
	"math"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── Market Maker Model Strategy ──────────────────────────────────────────
//
// Implements the ICT Market Maker Buy/Sell Model in 5 phases:
//
// Phase 1 – Consolidation: price is range-bound for consol_bars bars,
//           range < consol_atr * ATR.
// Phase 2 – Manipulation/Sweep: price breaks one side of the consolidation
//           range (stop hunt / liquidity grab).
// Phase 3 – Smart Money Reversal: displacement candle in the OPPOSITE
//           direction of the sweep.
// Phase 4 – FVG Formation: a Fair Value Gap forms after displacement.
// Phase 5 – Entry: price retraces into the FVG zone.
//
// Market Maker Buy Model: sweep below consolidation low → bullish reversal.
// Market Maker Sell Model: sweep above consolidation high → bearish reversal.
//
// Parameters:
//   swing_period  – swing detection period (default 5)
//   atr_period    – ATR calculation period (default 14)
//   disp_mult     – displacement ATR multiple (default 1.5)
//   body_ratio    – min body/range ratio (default 0.6)
//   consol_bars   – min bars for consolidation (default 10)
//   consol_atr    – max consolidation range as ATR multiple (default 1.5)
//   lookback      – lookback window for scanning (default 40)

type MarketMakerModelStrategy struct {
	bars        []data.OHLCV
	atr         []float64
	swingHighs  []float64
	swingLows   []float64
	swingPeriod int
	dispMult    float64
	bodyRatio   float64
	consolBars  int
	consolATR   float64
	lookback    int
	lastSigBar  int
}

func (s *MarketMakerModelStrategy) Name() string { return "Market Maker Model" }
func (s *MarketMakerModelStrategy) Description() string {
	return "ICT Market Maker Model: consolidation → manipulation sweep → displacement reversal → FVG entry"
}

func (s *MarketMakerModelStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	s.swingPeriod = int(getParam(params, "swing_period", 5))
	atrPeriod := int(getParam(params, "atr_period", 14))
	s.dispMult = getParam(params, "disp_mult", 1.5)
	s.bodyRatio = getParam(params, "body_ratio", 0.6)
	s.consolBars = int(getParam(params, "consol_bars", 10))
	s.consolATR = getParam(params, "consol_atr", 1.5)
	s.lookback = int(getParam(params, "lookback", 40))
	if s.lookback < s.consolBars+s.swingPeriod*2 {
		s.lookback = s.consolBars + s.swingPeriod*2
	}
	s.lastSigBar = -s.lookback

	s.atr = indicators.ATR(bars, atrPeriod)
	s.swingHighs = indicators.SwingHighs(bars, s.swingPeriod)
	s.swingLows = indicators.SwingLows(bars, s.swingPeriod)
}

func (s *MarketMakerModelStrategy) Signal(i int) SignalType {
	if i < s.lookback || math.IsNaN(s.atr[i]) {
		return NoSignal
	}
	if i-s.lastSigBar < s.swingPeriod {
		return NoSignal
	}

	if s.checkBuyModel(i) {
		s.lastSigBar = i
		return BuySignal
	}
	if s.checkSellModel(i) {
		s.lastSigBar = i
		return SellSignal
	}
	return NoSignal
}

// consolidation represents a detected consolidation range.
type consolidation struct {
	startIdx int
	endIdx   int
	high     float64
	low      float64
}

// findConsolidations scans for consolidation zones ending before bar i.
func (s *MarketMakerModelStrategy) findConsolidations(i int) []consolidation {
	bars := s.bars
	start := i - s.lookback
	if start < 0 {
		start = 0
	}

	var results []consolidation

	// Slide a window of consol_bars looking for tight ranges.
	for end := i - 1; end >= start+s.consolBars-1; end-- {
		begin := end - s.consolBars + 1
		if begin < start {
			break
		}
		if math.IsNaN(s.atr[end]) || s.atr[end] == 0 {
			continue
		}

		// Compute range of the window
		hi := bars[begin].High
		lo := bars[begin].Low
		for j := begin + 1; j <= end; j++ {
			if bars[j].High > hi {
				hi = bars[j].High
			}
			if bars[j].Low < lo {
				lo = bars[j].Low
			}
		}

		rng := hi - lo
		if rng < s.consolATR*s.atr[end] {
			results = append(results, consolidation{
				startIdx: begin,
				endIdx:   end,
				high:     hi,
				low:      lo,
			})
			// Skip ahead to avoid overlapping consolidations
			end = begin
		}
	}
	return results
}

// checkBuyModel: consolidation → sweep below low → bullish displacement → bullish FVG → retrace entry.
func (s *MarketMakerModelStrategy) checkBuyModel(i int) bool {
	bars := s.bars
	consols := s.findConsolidations(i)

	for _, c := range consols {
		// Phase 2: Find sweep below consolidation low after consolidation ends
		sweepIdx := -1
		for g := c.endIdx + 1; g <= i; g++ {
			if bars[g].Low < c.low {
				sweepIdx = g
				break
			}
		}
		if sweepIdx < 0 {
			continue
		}

		// Phase 3: Bullish displacement after sweep
		dispIdx := -1
		for d := sweepIdx; d < i; d++ {
			if math.IsNaN(s.atr[d]) || s.atr[d] == 0 {
				continue
			}
			body := bars[d].Close - bars[d].Open
			rng := bars[d].High - bars[d].Low
			if rng == 0 {
				continue
			}
			if body >= s.dispMult*s.atr[d] && body/rng >= s.bodyRatio {
				dispIdx = d
				break
			}
		}
		if dispIdx < 0 {
			continue
		}

		// Phase 4: Bullish FVG after displacement
		var fvgBot, fvgTop float64
		fvgFound := false
		for f := dispIdx; f < i; f++ {
			if f-1 < 0 || f+1 >= len(bars) {
				continue
			}
			gapBot := bars[f-1].High
			gapTop := bars[f+1].Low
			if gapTop > gapBot {
				fvgBot = gapBot
				fvgTop = gapTop
				fvgFound = true
				break
			}
		}
		if !fvgFound {
			continue
		}

		// Phase 5: Entry — current bar retraces into FVG zone
		if bars[i].Low <= fvgTop && bars[i].High >= fvgBot {
			return true
		}
	}
	return false
}

// checkSellModel: consolidation → sweep above high → bearish displacement → bearish FVG → retrace entry.
func (s *MarketMakerModelStrategy) checkSellModel(i int) bool {
	bars := s.bars
	consols := s.findConsolidations(i)

	for _, c := range consols {
		// Phase 2: Find sweep above consolidation high
		sweepIdx := -1
		for g := c.endIdx + 1; g <= i; g++ {
			if bars[g].High > c.high {
				sweepIdx = g
				break
			}
		}
		if sweepIdx < 0 {
			continue
		}

		// Phase 3: Bearish displacement after sweep
		dispIdx := -1
		for d := sweepIdx; d < i; d++ {
			if math.IsNaN(s.atr[d]) || s.atr[d] == 0 {
				continue
			}
			body := bars[d].Open - bars[d].Close
			rng := bars[d].High - bars[d].Low
			if rng == 0 {
				continue
			}
			if body >= s.dispMult*s.atr[d] && body/rng >= s.bodyRatio {
				dispIdx = d
				break
			}
		}
		if dispIdx < 0 {
			continue
		}

		// Phase 4: Bearish FVG after displacement
		var fvgBot, fvgTop float64
		fvgFound := false
		for f := dispIdx; f < i; f++ {
			if f-1 < 0 || f+1 >= len(bars) {
				continue
			}
			gapTop := bars[f-1].Low
			gapBot := bars[f+1].High
			if gapTop > gapBot {
				fvgBot = gapBot
				fvgTop = gapTop
				fvgFound = true
				break
			}
		}
		if !fvgFound {
			continue
		}

		// Phase 5: Entry — current bar retraces into FVG zone
		if bars[i].High >= fvgBot && bars[i].Low <= fvgTop {
			return true
		}
	}
	return false
}
