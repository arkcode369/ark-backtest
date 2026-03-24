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
	if math.IsNaN(prevFast) || math.IsNaN(prevSlow) || math.IsNaN(curFast) || math.IsNaN(curSlow) {
		return NoSignal
	}
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
	if math.IsNaN(prev) || math.IsNaN(cur) {
		return NoSignal
	}
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
	if math.IsNaN(prevM) || math.IsNaN(prevS) || math.IsNaN(curM) || math.IsNaN(curS) {
		return NoSignal
	}
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
	if math.IsNaN(s.bb.Upper[i-1]) || math.IsNaN(s.bb.Lower[i-1]) || math.IsNaN(c) || math.IsNaN(prev) {
		return NoSignal
	}
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
	if math.IsNaN(prevHi) || math.IsNaN(prevLo) {
		return NoSignal
	}
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
	if math.IsNaN(sma) || math.IsNaN(prevRSI) || math.IsNaN(curRSI) {
		return NoSignal
	}

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
		for d := grabIdx + 1; d < i-1; d++ {
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
		for d := grabIdx + 1; d < i-1; d++ {
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
	"silver_bullet": {
		Name:        "Silver Bullet",
		Description: "ICT Silver Bullet: sweep + FVG in 3 daily kill-zone windows",
		Params: map[string]float64{
			"swing_period": 5, "atr_period": 14, "disp_mult": 1.0,
			"body_ratio": 0.5, "sb_lookback": 20,
		},
		Factory: func() Strategy { return &SilverBulletStrategy{} },
	},
	"turtle_soup": {
		Name:        "Turtle Soup",
		Description: "ICT Turtle Soup: sweep of swing H/L → displacement reversal",
		Params: map[string]float64{
			"swing_period": 5, "atr_period": 14, "disp_mult": 1.5,
			"body_ratio": 0.6, "ts_lookback": 30, "equal_tol": 0.1, "require_equal": 0,
		},
		Factory: func() Strategy { return &TurtleSoupStrategy{} },
	},
	"amd_session": {
		Name:        "AMD Session",
		Description: "ICT AMD: Accumulation → Manipulation → Distribution",
		Params: map[string]float64{
			"swing_period": 5, "atr_period": 14, "disp_mult": 1.0, "body_ratio": 0.5,
		},
		Factory: func() Strategy { return &AMDSessionStrategy{} },
	},
	"ob_retest": {
		Name:        "OB Retest",
		Description: "ICT Order Block Retest: enter on retrace into unmitigated OB",
		Params: map[string]float64{
			"swing_period": 5, "atr_period": 14, "ob_impulse": 1.5,
			"min_ob_age": 5, "pd_lookback": 50, "use_pd_filter": 1,
		},
		Factory: func() Strategy { return &OBRetestStrategy{} },
	},
	"weekly_profile": {
		Name:        "Weekly Profile",
		Description: "ICT Weekly Profile: trade retraces to weekly open with PWH/PWL bias",
		Params: map[string]float64{
			"swing_period": 5, "atr_period": 14, "disp_mult": 1.0,
			"body_ratio": 0.5, "touch_atr": 0.5,
		},
		Factory: func() Strategy { return &WeeklyProfileStrategy{} },
	},
	"breaker_entry": {
		Name:        "Breaker Entry",
		Description: "ICT Breaker Block: enter on retest of failed (flipped) OB",
		Params: map[string]float64{
			"swing_period": 5, "atr_period": 14, "ob_impulse": 1.5,
			"min_age": 3, "pd_lookback": 50, "use_pd_filter": 0,
		},
		Factory: func() Strategy { return &BreakerEntryStrategy{} },
	},
	"cbdr_std": {
		Name:        "CBDR STD",
		Description: "ICT CBDR: mean-reversion at Asian range STD projections",
		Params: map[string]float64{
			"swing_period": 5, "atr_period": 14, "disp_mult": 0.5,
			"body_ratio": 0.4, "min_std": 1.0, "max_std": 2.5,
		},
		Factory: func() Strategy { return &CBDRSTDStrategy{} },
	},
	"ict_advanced": {
		Name:        "ICT Advanced",
		Description: "ICT 2022 + MTF bias, Kill Zones, CBDR/STD, NDOG/NWOG, SMT, Judas, IPDA",
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
			"judas_swing":    0,
			"liq_void":       0,
			"ipda_filter":    0,
			"bias9_step":     0,
		},
		Factory: func() Strategy { return &ICTAdvancedStrategy{} },
	},
	"cpe_entry": {
		Name:        "CPE Entry",
		Description: "Close Proximity Entry: session open retrace after displacement",
		Params: map[string]float64{
			"swing_period": 5, "atr_period": 14, "disp_mult": 1.0,
			"body_ratio": 0.5, "proximity_atr": 0.5,
		},
		Factory: func() Strategy { return &CPEEntryStrategy{} },
	},
	"london_close": {
		Name:        "London Close",
		Description: "London Close Kill Zone: reversal of London move during close window",
		Params: map[string]float64{
			"swing_period": 5, "atr_period": 14, "disp_mult": 1.0,
			"body_ratio": 0.5,
		},
		Factory: func() Strategy { return &LondonCloseStrategy{} },
	},
	"ny_open_rule": {
		Name:        "NY Open Rule",
		Description: "Compare London direction with NY open for continuation or reversal",
		Params: map[string]float64{
			"swing_period": 5, "atr_period": 14, "disp_mult": 1.0,
			"body_ratio": 0.5, "mode": 0,
		},
		Factory: func() Strategy { return &NYOpenRuleStrategy{} },
	},
	"dow_pattern": {
		Name:        "DOW Pattern",
		Description: "Day-of-week ICT tendencies with reversal bias on Wed/Thu",
		Params: map[string]float64{
			"swing_period": 5, "atr_period": 14, "disp_mult": 1.0,
			"body_ratio": 0.5, "trade_monday": 0, "trade_tuesday": 0,
			"trade_wednesday": 1, "trade_thursday": 1, "trade_friday": 0,
		},
		Factory: func() Strategy { return &DOWPatternStrategy{} },
	},
	"daily_template": {
		Name:        "Daily Template",
		Description: "Classify intraday into session templates and trade accordingly",
		Params: map[string]float64{
			"swing_period": 5, "atr_period": 14, "disp_mult": 1.0,
			"body_ratio": 0.5, "template_mode": 0,
		},
		Factory: func() Strategy { return &DailyTemplateStrategy{} },
	},
	"hod_lod": {
		Name:        "HOD/LOD Projection",
		Description: "Project daily HOD/LOD via CBDR STD levels; reversal at projected extremes",
		Params: map[string]float64{
			"swing_period": 5, "atr_period": 14, "disp_mult": 0.5,
			"body_ratio": 0.4, "min_std": 1.0, "max_std": 2.5,
		},
		Factory: func() Strategy { return &HODLODStrategy{} },
	},
	"flout": {
		Name:        "Flout",
		Description: "CBDR + Asian range combined (Flout) STD projections with displacement reversal",
		Params: map[string]float64{
			"swing_period": 5, "atr_period": 14, "disp_mult": 0.5,
			"body_ratio": 0.4, "min_std": 1.0, "max_std": 2.5, "flout_mode": 0,
		},
		Factory: func() Strategy { return &FloutStrategy{} },
	},
	"intermarket": {
		Name:        "Intermarket",
		Description: "MTF mean-reversion: buy HTF-trend dips, sell HTF-trend rips",
		Params: map[string]float64{
			"swing_period": 5, "atr_period": 14, "disp_mult": 1.0,
			"body_ratio": 0.5, "reversion_atr": 1.5, "htf_lookback": 20,
		},
		Factory: func() Strategy { return &IntermarketStrategy{} },
	},
	"cot_proxy": {
		Name:        "COT Proxy",
		Description: "Volume-based institutional positioning proxy: price-volume divergence",
		Params: map[string]float64{
			"swing_period": 5, "atr_period": 14, "disp_mult": 1.0,
			"body_ratio": 0.5, "vol_period": 20, "divergence_threshold": 0.5,
		},
		Factory: func() Strategy { return &COTProxyStrategy{} },
	},
	"megatrade": {
		Name:        "Megatrade",
		Description: "Multi-week swing trades on mega-structure shifts (large-TF CHoCH)",
		Params: map[string]float64{
			"swing_period": 10, "atr_period": 20, "disp_mult": 2.0,
			"body_ratio": 0.5, "min_swing_count": 3, "lookback": 100,
		},
		Factory: func() Strategy { return &MegatradeStrategy{} },
	},
	"mss_choch": {
		Name:        "MSS/CHoCH",
		Description: "Market Structure Shift / Change of Character with displacement entry",
		Params: map[string]float64{
			"swing_period": 5, "atr_period": 14, "disp_mult": 1.0,
			"body_ratio": 0.5, "lookback": 30,
		},
		Factory: func() Strategy { return &MSSCHoCHStrategy{} },
	},
	"ote_entry": {
		Name:        "OTE Entry",
		Description: "Optimal Trade Entry: 62-79% fib retracement at FVG after impulse",
		Params: map[string]float64{
			"swing_period": 5, "atr_period": 14, "disp_mult": 1.5,
			"body_ratio": 0.6, "fib_low": 0.62, "fib_high": 0.79, "lookback": 30,
		},
		Factory: func() Strategy { return &OTEEntryStrategy{} },
	},
	"mm_model": {
		Name:        "Market Maker Model",
		Description: "Full MM model: consolidation → sweep → displacement → FVG → OTE entry",
		Params: map[string]float64{
			"swing_period": 5, "atr_period": 14, "disp_mult": 1.5,
			"body_ratio": 0.6, "consol_bars": 10, "consol_atr": 1.5, "lookback": 40,
		},
		Factory: func() Strategy { return &MarketMakerModelStrategy{} },
	},
	"three_drives": {
		Name:        "Three Drives",
		Description: "Three progressive swing points with fib extensions → reversal",
		Params: map[string]float64{
			"swing_period": 5, "atr_period": 14, "disp_mult": 1.0,
			"body_ratio": 0.5, "fib_min": 1.1, "fib_max": 1.8, "lookback": 50,
		},
		Factory: func() Strategy { return &ThreeDrivesStrategy{} },
	},
	"lrlr_entry": {
		Name:        "HRLR→LRLR",
		Description: "Detect transition from choppy (HRLR) to trending (LRLR) and enter",
		Params: map[string]float64{
			"swing_period": 5, "atr_period": 14, "disp_mult": 1.0,
			"body_ratio": 0.5, "chop_period": 20, "chop_threshold": 0.5,
		},
		Factory: func() Strategy { return &LRLREntryStrategy{} },
	},
	"irl_erl": {
		Name:        "IRL→ERL",
		Description: "Enter at internal liquidity (FVG/OB) targeting external liquidity (swing H/L)",
		Params: map[string]float64{
			"swing_period": 5, "atr_period": 14, "ob_impulse": 1.5, "pd_lookback": 50,
		},
		Factory: func() Strategy { return &IRLERLStrategy{} },
	},
	"open_float": {
		Name:        "Open Float",
		Description: "Sweep of 20/40/60-day H/L institutional liquidity pools → reversal",
		Params: map[string]float64{
			"swing_period": 5, "atr_period": 14, "disp_mult": 1.5,
			"body_ratio": 0.6, "pool_20": 1, "pool_40": 1, "pool_60": 1,
		},
		Factory: func() Strategy { return &OpenFloatStrategy{} },
	},
	"ce_entry": {
		Name:        "CE Entry",
		Description: "Consequent Encroachment: enter at FVG midpoint (50% of gap)",
		Params: map[string]float64{
			"swing_period": 5, "atr_period": 14, "ob_impulse": 1.5,
			"max_fvg_age": 30, "touch_tolerance": 0.2,
		},
		Factory: func() Strategy { return &CEEntryStrategy{} },
	},
	"event_horizon": {
		Name:        "Event Horizon",
		Description: "Trade at midpoint between adjacent NWOGs (gravitational price level)",
		Params: map[string]float64{
			"swing_period": 5, "atr_period": 14, "disp_mult": 0.5,
			"body_ratio": 0.4, "touch_atr": 1.0,
		},
		Factory: func() Strategy { return &EventHorizonStrategy{} },
	},
}

func getParam(params map[string]float64, key string, def float64) float64 {
	if v, ok := params[key]; ok {
		return v
	}
	return def
}
