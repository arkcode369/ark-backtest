package backtest

import (
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
	if prev <= s.bb.Upper[i-1] && c > s.bb.Upper[i] {
		return BuySignal
	}
	if prev >= s.bb.Lower[i-1] && c < s.bb.Lower[i] {
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
}

func getParam(params map[string]float64, key string, def float64) float64 {
	if v, ok := params[key]; ok {
		return v
	}
	return def
}
