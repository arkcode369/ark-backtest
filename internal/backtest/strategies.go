package backtest

import (
	"math"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── EMA Crossover ─────────────────────────────────────────────────────────

type EMACrossStrategy struct {
	bars   []data.OHLCV
	fast   []float64
	slow   []float64
}

func (s *EMACrossStrategy) Name() string { return "EMA Crossover" }
func (s *EMACrossStrategy) Description() string {
	return "Buy when fast EMA crosses above slow EMA, sell when crosses below"
}
func (s *EMACrossStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	closes := indicators.ExtractClose(bars)
	fastP := int(getParam(params, "fast", 9))
	slowP := int(getParam(params, "slow", 21))
	s.fast = indicators.EMA(closes, fastP)
	s.slow = indicators.EMA(closes, slowP)
}
func (s *EMACrossStrategy) Signal(i int) SignalType {
	if i < 1 {
		return NoSignal
	}
	prevFast, prevSlow := s.fast[i-1], s.slow[i-1]
	curFast, curSlow := s.fast[i], s.slow[i]
	if prevFast <= prevSlow && curFast > curSlow {
		return BuySignal
	}
	if prevFast >= prevSlow && curFast < curSlow {
		return SellSignal
	}
	return NoSignal
}

// ── RSI Strategy ──────────────────────────────────────────────────────────

type RSIStrategy struct {
	bars   []data.OHLCV
	rsi    []float64
	overbought float64
	oversold   float64
}

func (s *RSIStrategy) Name() string { return "RSI Mean Reversion" }
func (s *RSIStrategy) Description() string {
	return "Buy when RSI crosses above oversold, sell when crosses below overbought"
}
func (s *RSIStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	closes := indicators.ExtractClose(bars)
	period := int(getParam(params, "period", 14))
	s.rsi = indicators.RSI(closes, period)
	s.overbought = getParam(params, "overbought", 70)
	s.oversold = getParam(params, "oversold", 30)
}
func (s *RSIStrategy) Signal(i int) SignalType {
	if i < 1 {
		return NoSignal
	}
	prev, cur := s.rsi[i-1], s.rsi[i]
	if prev <= s.oversold && cur > s.oversold {
		return BuySignal
	}
	if prev >= s.overbought && cur < s.overbought {
		return SellSignal
	}
	return NoSignal
}

// ── MACD Strategy ─────────────────────────────────────────────────────────

type MACDStrategy struct {
	bars []data.OHLCV
	macd indicators.MACDResult
}

func (s *MACDStrategy) Name() string { return "MACD Crossover" }
func (s *MACDStrategy) Description() string {
	return "Buy when MACD crosses above Signal, sell when crosses below"
}
func (s *MACDStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	closes := indicators.ExtractClose(bars)
	fast := int(getParam(params, "fast", 12))
	slow := int(getParam(params, "slow", 26))
	signal := int(getParam(params, "signal", 9))
	s.macd = indicators.MACD(closes, fast, slow, signal)
}
func (s *MACDStrategy) Signal(i int) SignalType {
	if i < 1 {
		return NoSignal
	}
	prevM, prevS := s.macd.MACD[i-1], s.macd.Signal[i-1]
	curM, curS := s.macd.MACD[i], s.macd.Signal[i]
	if prevM <= prevS && curM > curS {
		return BuySignal
	}
	if prevM >= prevS && curM < curS {
		return SellSignal
	}
	return NoSignal
}

// ── Bollinger Band Breakout ───────────────────────────────────────────────

type BBBreakoutStrategy struct {
	bars []data.OHLCV
	bb   indicators.BBResult
}

func (s *BBBreakoutStrategy) Name() string { return "Bollinger Band Breakout" }
func (s *BBBreakoutStrategy) Description() string {
	return "Buy on upper band breakout, sell on lower band breakout"
}
func (s *BBBreakoutStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	closes := indicators.ExtractClose(bars)
	period := int(getParam(params, "period", 20))
	std := getParam(params, "std", 2.0)
	s.bb = indicators.BollingerBands(closes, period, std)
}
func (s *BBBreakoutStrategy) Signal(i int) SignalType {
	if i < 1 {
		return NoSignal
	}
	c := s.bars[i].Close
	prev := s.bars[i-1].Close
	if prev <= s.bb.Upper[i-1] && c > s.bb.Upper[i-1] {
		return BuySignal
	}
	if prev >= s.bb.Lower[i-1] && c < s.bb.Lower[i-1] {
		return SellSignal
	}
	return NoSignal
}

// ── Supertrend Strategy ───────────────────────────────────────────────────

type SupertrendStrategy struct {
	bars       []data.OHLCV
	supertrend indicators.SupertrendResult
}

func (s *SupertrendStrategy) Name() string { return "Supertrend" }
func (s *SupertrendStrategy) Description() string {
	return "Buy/sell based on Supertrend direction change"
}
func (s *SupertrendStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	period := int(getParam(params, "period", 10))
	mult := getParam(params, "multiplier", 3.0)
	s.supertrend = indicators.Supertrend(bars, period, mult)
}
func (s *SupertrendStrategy) Signal(i int) SignalType {
	if i < 1 {
		return NoSignal
	}
	prev, cur := s.supertrend.Direction[i-1], s.supertrend.Direction[i]
	if prev == -1 && cur == 1 {
		return BuySignal
	}
	if prev == 1 && cur == -1 {
		return SellSignal
	}
	return NoSignal
}

// ── Donchian Breakout ─────────────────────────────────────────────────────

type DonchianStrategy struct {
	bars     []data.OHLCV
	donchian indicators.DonchianResult
}

func (s *DonchianStrategy) Name() string { return "Donchian Breakout" }
func (s *DonchianStrategy) Description() string {
	return "Buy on N-period high breakout, sell on N-period low breakout (Turtle Trading)"
}
func (s *DonchianStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	period := int(getParam(params, "period", 20))
	s.donchian = indicators.Donchian(bars, period)
}
func (s *DonchianStrategy) Signal(i int) SignalType {
	if i < 1 {
		return NoSignal
	}
	h := s.bars[i].High
	l := s.bars[i].Low
	prevHi := s.donchian.Upper[i-1]
	prevLo := s.donchian.Lower[i-1]
	if h > prevHi {
		return BuySignal
	}
	if l < prevLo {
		return SellSignal
	}
	return NoSignal
}

// ── SMA + RSI Confluence ──────────────────────────────────────────────────

type SMAConfluenceStrategy struct {
	bars  []data.OHLCV
	sma   []float64
	rsi   []float64
	ob    float64
	os    float64
}

func (s *SMAConfluenceStrategy) Name() string { return "SMA + RSI Confluence" }
func (s *SMAConfluenceStrategy) Description() string {
	return "Buy when price above SMA AND RSI exits oversold; sell when below SMA AND RSI exits overbought"
}
func (s *SMAConfluenceStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	closes := indicators.ExtractClose(bars)
	smaPeriod := int(getParam(params, "sma_period", 50))
	rsiPeriod := int(getParam(params, "rsi_period", 14))
	s.sma = indicators.SMA(closes, smaPeriod)
	s.rsi = indicators.RSI(closes, rsiPeriod)
	s.ob = getParam(params, "overbought", 70)
	s.os = getParam(params, "oversold", 30)
}
func (s *SMAConfluenceStrategy) Signal(i int) SignalType {
	if i < 1 {
		return NoSignal
	}
	c := s.bars[i].Close
	sma := s.sma[i]
	prevRSI, curRSI := s.rsi[i-1], s.rsi[i]

	if c > sma && prevRSI <= s.os && curRSI > s.os {
		return BuySignal
	}
	if c < sma && prevRSI >= s.ob && curRSI < s.ob {
		return SellSignal
	}
	return NoSignal
}

// ── ICT 2022 Mentorship Model ────────────────────────────────────────────
//
// 6-step framework: liquidity grab → displacement + FVG → MSS → fib 50%
// → FVG validation → entry. Supports both bullish (SSL grab) and bearish
// (BSL grab) setups.

type ICTMentorshipStrategy struct {
	bars        []data.OHLCV
	atr         []float64
	swingHighs  []float64
	swingLows   []float64
	swingPeriod int
	dispMult    float64
	bodyRatio   float64
	fibValid    bool
	lookback    int
	lastSigBar  int // prevent double-signal on same setup
}

func (s *ICTMentorshipStrategy) Name() string { return "ICT 2022" }
func (s *ICTMentorshipStrategy) Description() string {
	return "ICT 6-step model: liquidity grab → displacement FVG → MSS → fib-validated entry"
}
func (s *ICTMentorshipStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	s.swingPeriod = int(getParam(params, "swing_period", 5))
	atrPeriod := int(getParam(params, "atr_period", 14))
	s.dispMult = getParam(params, "disp_mult", 1.5)
	s.bodyRatio = getParam(params, "body_ratio", 0.6)
	s.fibValid = getParam(params, "fvg_fib_valid", 1) == 1
	s.lookback = int(getParam(params, "lookback", float64(s.swingPeriod*6)))
	if s.lookback < s.swingPeriod*3 {
		s.lookback = s.swingPeriod * 3
	}
	s.lastSigBar = -s.lookback

	s.atr = indicators.ATR(bars, atrPeriod)
	s.swingHighs = indicators.SwingHighs(bars, s.swingPeriod)
	s.swingLows = indicators.SwingLows(bars, s.swingPeriod)
}

func (s *ICTMentorshipStrategy) Signal(i int) SignalType {
	if i < s.swingPeriod*2+3 || math.IsNaN(s.atr[i]) {
		return NoSignal
	}
	// Prevent double-signal within lookback window of same setup
	if i-s.lastSigBar < s.swingPeriod {
		return NoSignal
	}

	buy := s.checkBuy(i)
	if buy {
		s.lastSigBar = i
		return BuySignal
	}
	sell := s.checkSell(i)
	if sell {
		s.lastSigBar = i
		return SellSignal
	}
	return NoSignal
}

// checkBuy looks for a bullish ICT setup (SSL grab → bullish reversal)
func (s *ICTMentorshipStrategy) checkBuy(i int) bool {
	bars := s.bars
	start := i - s.lookback
	if start < s.swingPeriod {
		start = s.swingPeriod
	}

	// Iterate through all confirmed swing lows in the lookback window (most recent first)
	for j := i - s.swingPeriod - 1; j >= start; j-- {
		if math.IsNaN(s.swingLows[j]) {
			continue
		}
		swingLowPrice := s.swingLows[j]

		// Step 1: Find a bar after the swing low that sweeps below it (liquidity grab)
		grabIdx := -1
		for g := j + 1; g <= i; g++ {
			if bars[g].Low < swingLowPrice && bars[g].Close > swingLowPrice {
				grabIdx = g
				break
			}
		}
		if grabIdx < 0 {
			continue
		}

		// Step 2: Displacement candle + bullish FVG after the grab
		dispIdx := -1
		var fvgTop, fvgBot float64
		for d := grabIdx + 1; d < i; d++ {
			if math.IsNaN(s.atr[d]) || s.atr[d] == 0 {
				continue
			}
			body := bars[d].Close - bars[d].Open
			rng := bars[d].High - bars[d].Low
			if rng == 0 {
				continue
			}
			if body >= s.dispMult*s.atr[d] && body/rng >= s.bodyRatio {
				if d-1 >= 0 && d+1 < len(bars) {
					gapBot := bars[d-1].High
					gapTop := bars[d+1].Low
					if gapTop > gapBot {
						dispIdx = d
						fvgBot = gapBot
						fvgTop = gapTop
						break
					}
				}
			}
		}
		if dispIdx < 0 {
			continue
		}

		// Step 3: MSS — price must break above a recent swing high before the grab
		swingHighPrice := 0.0
		for k := grabIdx - 1; k >= start; k-- {
			if !math.IsNaN(s.swingHighs[k]) {
				swingHighPrice = s.swingHighs[k]
				break
			}
		}
		if swingHighPrice == 0 {
			continue
		}
		foundMSS := false
		for k := dispIdx; k <= i; k++ {
			if bars[k].High > swingHighPrice {
				foundMSS = true
				break
			}
		}
		if !foundMSS {
			continue
		}

		// Step 4 & 5: Fibonacci 50% validation
		if s.fibValid {
			grabLow := bars[grabIdx].Low
			dispHigh := bars[dispIdx].High
			fib50 := grabLow + (dispHigh-grabLow)*0.5
			if fvgBot > fib50 {
				continue
			}
		}

		// Step 6: Entry — current bar retraces into the FVG zone
		if bars[i].Low <= fvgTop && bars[i].High >= fvgBot {
			return true
		}
	}
	return false
}

// checkSell looks for a bearish ICT setup (BSL grab → bearish reversal)
func (s *ICTMentorshipStrategy) checkSell(i int) bool {
	bars := s.bars
	start := i - s.lookback
	if start < s.swingPeriod {
		start = s.swingPeriod
	}

	// Iterate through all confirmed swing highs in the lookback window
	for j := i - s.swingPeriod - 1; j >= start; j-- {
		if math.IsNaN(s.swingHighs[j]) {
			continue
		}
		swingHighPrice := s.swingHighs[j]

		// Step 1: Find a bar that sweeps above the swing high (BSL grab)
		grabIdx := -1
		for g := j + 1; g <= i; g++ {
			if bars[g].High > swingHighPrice && bars[g].Close < swingHighPrice {
				grabIdx = g
				break
			}
		}
		if grabIdx < 0 {
			continue
		}

		// Step 2: Bearish displacement + bearish FVG after the grab
		dispIdx := -1
		var fvgTop, fvgBot float64
		for d := grabIdx + 1; d < i; d++ {
			if math.IsNaN(s.atr[d]) || s.atr[d] == 0 {
				continue
			}
			body := bars[d].Open - bars[d].Close
			rng := bars[d].High - bars[d].Low
			if rng == 0 {
				continue
			}
			if body >= s.dispMult*s.atr[d] && body/rng >= s.bodyRatio {
				if d-1 >= 0 && d+1 < len(bars) {
					gapTop := bars[d-1].Low
					gapBot := bars[d+1].High
					if gapTop > gapBot {
						dispIdx = d
						fvgTop = gapTop
						fvgBot = gapBot
						break
					}
				}
			}
		}
		if dispIdx < 0 {
			continue
		}

		// Step 3: MSS — price must break below a recent swing low
		swingLowPrice := 0.0
		for k := grabIdx - 1; k >= start; k-- {
			if !math.IsNaN(s.swingLows[k]) {
				swingLowPrice = s.swingLows[k]
				break
			}
		}
		if swingLowPrice == 0 {
			continue
		}
		foundMSS := false
		for k := dispIdx; k <= i; k++ {
			if bars[k].Low < swingLowPrice {
				foundMSS = true
				break
			}
		}
		if !foundMSS {
			continue
		}

		// Step 4 & 5: Fibonacci 50% validation
		if s.fibValid {
			grabHigh := bars[grabIdx].High
			dispLow := bars[dispIdx].Low
			fib50 := dispLow + (grabHigh-dispLow)*0.5
			if fvgTop < fib50 {
				continue
			}
		}

		// Step 6: Entry — current bar retraces into FVG zone
		if bars[i].High >= fvgBot && bars[i].Low <= fvgTop {
			return true
		}
	}
	return false
}

// ── Strategy Registry ─────────────────────────────────────────────────────

type StrategyMeta struct {
	Name        string
	Description string
	Params      map[string]float64 // default params
	Factory     func() Strategy
}

var StrategyRegistry = map[string]StrategyMeta{
	"ema_cross": {
		Name:        "EMA Crossover",
		Description: "Buy/Sell on EMA cross",
		Params:      map[string]float64{"fast": 9, "slow": 21},
		Factory:     func() Strategy { return &EMACrossStrategy{} },
	},
	"rsi": {
		Name:        "RSI Mean Reversion",
		Description: "RSI overbought/oversold signals",
		Params:      map[string]float64{"period": 14, "overbought": 70, "oversold": 30},
		Factory:     func() Strategy { return &RSIStrategy{} },
	},
	"macd": {
		Name:        "MACD Crossover",
		Description: "MACD/Signal line crossover",
		Params:      map[string]float64{"fast": 12, "slow": 26, "signal": 9},
		Factory:     func() Strategy { return &MACDStrategy{} },
	},
	"bb_breakout": {
		Name:        "Bollinger Band Breakout",
		Description: "BB upper/lower band breakout",
		Params:      map[string]float64{"period": 20, "std": 2.0},
		Factory:     func() Strategy { return &BBBreakoutStrategy{} },
	},
	"supertrend": {
		Name:        "Supertrend",
		Description: "Supertrend direction change",
		Params:      map[string]float64{"period": 10, "multiplier": 3.0},
		Factory:     func() Strategy { return &SupertrendStrategy{} },
	},
	"donchian": {
		Name:        "Donchian Breakout",
		Description: "Turtle Trading N-period breakout",
		Params:      map[string]float64{"period": 20},
		Factory:     func() Strategy { return &DonchianStrategy{} },
	},
	"sma_rsi": {
		Name:        "SMA + RSI Confluence",
		Description: "Trend filter + momentum signal",
		Params:      map[string]float64{"sma_period": 50, "rsi_period": 14, "overbought": 70, "oversold": 30},
		Factory:     func() Strategy { return &SMAConfluenceStrategy{} },
	},
	"ict2022": {
		Name:        "ICT 2022",
		Description: "Liquidity grab → displacement FVG → MSS → fib-validated entry",
		Params:      map[string]float64{"swing_period": 5, "atr_period": 14, "disp_mult": 1.5, "body_ratio": 0.6, "fvg_fib_valid": 1, "lookback": 30},
		Factory:     func() Strategy { return &ICTMentorshipStrategy{} },
	},
	"ict_advanced": {
		Name:        "ICT Advanced",
		Description: "ICT 2022 + MTF bias, Kill Zones, CBDR/STD, NDOG/NWOG, SMT divergence",
		Params: map[string]float64{
			"swing_period":   5,
			"atr_period":     14,
			"disp_mult":      1.5,
			"body_ratio":     0.6,
			"fvg_fib_valid":  1,
			"lookback":       30,
			"htf_filter":     1,
			"killzone_only":  0,
			"cbdr_filter":    0,
			"smt_confluence": 0,
			"gap_awareness":  0,
			"pd_arrays":      0,
		},
		Factory: func() Strategy { return &ICTAdvancedStrategy{} },
	},
}

func getParam(params map[string]float64, key string, def float64) float64 {
	if v, ok := params[key]; ok {
		return v
	}
	return def
}
