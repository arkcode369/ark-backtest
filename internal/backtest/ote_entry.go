package backtest

import (
	"math"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── OTE (Optimal Trade Entry) Strategy ───────────────────────────────────
//
// After a strong impulse move (displacement candle), wait for price to
// retrace into the 62-79% Fibonacci retracement zone (the OTE zone).
// Confirm with a Fair Value Gap at or near the OTE level.
//
// Bullish OTE:
//   1. Detect bullish displacement candle (body > disp_mult * ATR)
//   2. Identify the impulse leg (swing low to displacement high)
//   3. Wait for price to retrace 62-79% of the move (OTE zone)
//   4. Confirm with bullish FVG at or near the OTE level
//   5. Enter long
//
// Bearish OTE: opposite of above.
//
// Parameters:
//   swing_period – swing detection period (default 5)
//   atr_period   – ATR calculation period (default 14)
//   disp_mult    – displacement ATR multiple (default 1.5)
//   body_ratio   – min body/range ratio (default 0.6)
//   fib_low      – lower bound of OTE zone (default 0.62)
//   fib_high     – upper bound of OTE zone (default 0.79)
//   lookback     – lookback window for finding setups (default 30)

type OTEEntryStrategy struct {
	bars        []data.OHLCV
	atr         []float64
	swingHighs  []float64
	swingLows   []float64
	swingPeriod int
	dispMult    float64
	bodyRatio   float64
	fibLow      float64
	fibHigh     float64
	lookback    int
	lastSigBar  int
}

func (s *OTEEntryStrategy) Name() string { return "OTE Entry" }
func (s *OTEEntryStrategy) Description() string {
	return "Optimal Trade Entry: enter at 62-79% fib retracement with FVG confirmation after displacement"
}

func (s *OTEEntryStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	s.swingPeriod = int(getParam(params, "swing_period", 5))
	atrPeriod := int(getParam(params, "atr_period", 14))
	s.dispMult = getParam(params, "disp_mult", 1.5)
	s.bodyRatio = getParam(params, "body_ratio", 0.6)
	s.fibLow = getParam(params, "fib_low", 0.62)
	s.fibHigh = getParam(params, "fib_high", 0.79)
	s.lookback = int(getParam(params, "lookback", 30))
	if s.lookback < s.swingPeriod*3 {
		s.lookback = s.swingPeriod * 3
	}
	s.lastSigBar = -s.lookback

	s.atr = indicators.ATR(bars, atrPeriod)
	s.swingHighs = indicators.SwingHighs(bars, s.swingPeriod)
	s.swingLows = indicators.SwingLows(bars, s.swingPeriod)
}

func (s *OTEEntryStrategy) Signal(i int) SignalType {
	if i < s.swingPeriod*2+3 || math.IsNaN(s.atr[i]) {
		return NoSignal
	}
	if i-s.lastSigBar < s.swingPeriod {
		return NoSignal
	}

	if s.checkBullishOTE(i) {
		s.lastSigBar = i
		return BuySignal
	}
	if s.checkBearishOTE(i) {
		s.lastSigBar = i
		return SellSignal
	}
	return NoSignal
}

// checkBullishOTE looks for a bullish displacement followed by OTE retracement + FVG.
func (s *OTEEntryStrategy) checkBullishOTE(i int) bool {
	bars := s.bars
	start := i - s.lookback
	if start < s.swingPeriod {
		start = s.swingPeriod
	}

	// Find bullish displacement candles in the lookback window
	for d := i - 1; d >= start; d-- {
		if math.IsNaN(s.atr[d]) || s.atr[d] == 0 {
			continue
		}
		body := bars[d].Close - bars[d].Open
		rng := bars[d].High - bars[d].Low
		if rng == 0 {
			continue
		}
		if body < s.dispMult*s.atr[d] || body/rng < s.bodyRatio {
			continue
		}

		// Found bullish displacement at bar d.
		// Find the swing low before the displacement (impulse leg origin).
		swingLowPrice := 0.0
		for k := d - 1; k >= start; k-- {
			if !math.IsNaN(s.swingLows[k]) {
				swingLowPrice = s.swingLows[k]
				break
			}
		}
		if swingLowPrice == 0 {
			// Fallback: use the lowest low in the range before displacement
			swingLowPrice = bars[start].Low
			for k := start + 1; k < d; k++ {
				if bars[k].Low < swingLowPrice {
					swingLowPrice = bars[k].Low
				}
			}
		}

		// Impulse high is the displacement candle's high
		impulseHigh := bars[d].High
		impulseLeg := impulseHigh - swingLowPrice
		if impulseLeg <= 0 {
			continue
		}

		// OTE zone: price retraces 62-79% of the impulse leg
		// Retracement from the high: high - fib_low*leg to high - fib_high*leg
		oteTop := impulseHigh - s.fibLow*impulseLeg
		oteBot := impulseHigh - s.fibHigh*impulseLeg

		// Check if current bar enters the OTE zone
		if bars[i].Low > oteTop || bars[i].High < oteBot {
			continue
		}

		// Look for a bullish FVG between displacement and current bar
		if s.hasBullishFVGInRange(d, i, oteBot, oteTop) {
			return true
		}
	}
	return false
}

// checkBearishOTE looks for bearish displacement followed by OTE retracement + FVG.
func (s *OTEEntryStrategy) checkBearishOTE(i int) bool {
	bars := s.bars
	start := i - s.lookback
	if start < s.swingPeriod {
		start = s.swingPeriod
	}

	for d := i - 1; d >= start; d-- {
		if math.IsNaN(s.atr[d]) || s.atr[d] == 0 {
			continue
		}
		body := bars[d].Open - bars[d].Close
		rng := bars[d].High - bars[d].Low
		if rng == 0 {
			continue
		}
		if body < s.dispMult*s.atr[d] || body/rng < s.bodyRatio {
			continue
		}

		// Found bearish displacement at bar d.
		// Find the swing high before the displacement.
		swingHighPrice := 0.0
		for k := d - 1; k >= start; k-- {
			if !math.IsNaN(s.swingHighs[k]) {
				swingHighPrice = s.swingHighs[k]
				break
			}
		}
		if swingHighPrice == 0 {
			swingHighPrice = bars[start].High
			for k := start + 1; k < d; k++ {
				if bars[k].High > swingHighPrice {
					swingHighPrice = bars[k].High
				}
			}
		}

		impulseLow := bars[d].Low
		impulseLeg := swingHighPrice - impulseLow
		if impulseLeg <= 0 {
			continue
		}

		// OTE zone for bearish: price retraces up 62-79% of the downward leg
		oteBot := impulseLow + s.fibLow*impulseLeg
		oteTop := impulseLow + s.fibHigh*impulseLeg

		if bars[i].Low > oteTop || bars[i].High < oteBot {
			continue
		}

		if s.hasBearishFVGInRange(d, i, oteBot, oteTop) {
			return true
		}
	}
	return false
}

// hasBullishFVGInRange checks for a bullish FVG between bars from and to,
// where the FVG overlaps with the zone [zoneBot, zoneTop].
func (s *OTEEntryStrategy) hasBullishFVGInRange(from, to int, zoneBot, zoneTop float64) bool {
	bars := s.bars
	for j := from; j < to-1; j++ {
		if j-1 < 0 || j+1 >= len(bars) {
			continue
		}
		// Bullish FVG: gap between bar[j-1].High and bar[j+1].Low
		gapBot := bars[j-1].High
		gapTop := bars[j+1].Low
		if gapTop <= gapBot {
			continue
		}
		// Check overlap with OTE zone
		overlapBot := math.Max(gapBot, zoneBot)
		overlapTop := math.Min(gapTop, zoneTop)
		if overlapTop > overlapBot {
			return true
		}
	}
	return false
}

// hasBearishFVGInRange checks for a bearish FVG between bars from and to,
// where the FVG overlaps with the zone [zoneBot, zoneTop].
func (s *OTEEntryStrategy) hasBearishFVGInRange(from, to int, zoneBot, zoneTop float64) bool {
	bars := s.bars
	for j := from; j < to-1; j++ {
		if j-1 < 0 || j+1 >= len(bars) {
			continue
		}
		// Bearish FVG: gap between bar[j+1].High and bar[j-1].Low
		gapTop := bars[j-1].Low
		gapBot := bars[j+1].High
		if gapTop <= gapBot {
			continue
		}
		overlapBot := math.Max(gapBot, zoneBot)
		overlapTop := math.Min(gapTop, zoneTop)
		if overlapTop > overlapBot {
			return true
		}
	}
	return false
}
