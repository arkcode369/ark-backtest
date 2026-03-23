package backtest

import (
	"math"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── Intermarket Analysis Strategy (Multi-Timeframe Proxy) ────────────────
//
// Since only single-instrument OHLCV data is available, this strategy
// proxies intermarket analysis via a multi-timeframe mean-reversion
// approach:
//
//  1. Determine the higher timeframe (HTF) trend using a rolling window
//     on HTF bars (e.g., daily): bullish if close > open over htf_lookback
//     bars net, bearish otherwise.
//  2. On the entry timeframe (LTF), look for pullbacks against the HTF
//     trend that exceed a threshold (reversion_atr x ATR).
//  3. Enter with displacement confirmation in the direction of the HTF
//     trend (buy the dip in an uptrend, sell the rip in a downtrend).
//
// Implements MultiTimeframeStrategy so the engine provides HTF bars.
//
// Parameters:
//   swing_period   – signal throttle (default 5)
//   atr_period     – ATR period (default 14)
//   disp_mult      – displacement ATR multiple (default 1.0)
//   body_ratio     – min body/range ratio (default 0.5)
//   reversion_atr  – pullback depth in ATR multiples to qualify (default 1.5)
//   htf_lookback   – bars on HTF to assess trend (default 20)

type IntermarketStrategy struct {
	bars        []data.OHLCV
	atr         []float64
	swingPeriod int
	dispMult    float64
	bodyRatio   float64
	reversionATR float64
	htfLookback int
	lastSigBar  int

	// HTF state: +1 bullish, -1 bearish, 0 neutral
	htfBias int
	// Rolling high/low on entry TF for pullback detection
	swingHighs []float64
	swingLows  []float64
}

func (s *IntermarketStrategy) Name() string { return "Intermarket" }
func (s *IntermarketStrategy) Description() string {
	return "MTF mean-reversion: buy HTF-trend dips, sell HTF-trend rips with displacement confirmation"
}

// Timeframes returns the additional timeframes needed for HTF bias.
func (s *IntermarketStrategy) Timeframes() []string {
	return []string{"1d"}
}

func (s *IntermarketStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	s.swingPeriod = int(getParam(params, "swing_period", 5))
	atrPeriod := int(getParam(params, "atr_period", 14))
	s.dispMult = getParam(params, "disp_mult", 1.0)
	s.bodyRatio = getParam(params, "body_ratio", 0.5)
	s.reversionATR = getParam(params, "reversion_atr", 1.5)
	s.htfLookback = int(getParam(params, "htf_lookback", 20))
	s.lastSigBar = -10

	s.atr = indicators.ATR(bars, atrPeriod)
	s.swingHighs = indicators.SwingHighs(bars, s.swingPeriod)
	s.swingLows = indicators.SwingLows(bars, s.swingPeriod)

	// Default HTF bias from LTF bars if InitMTF is not called
	s.htfBias = s.computeBiasFromBars(bars)
}

// InitMTF is called by the engine with higher-timeframe bars.
func (s *IntermarketStrategy) InitMTF(barsByTF map[string][]data.OHLCV, params map[string]float64) {
	daily, ok := barsByTF["1d"]
	if !ok || len(daily) == 0 {
		return
	}
	s.htfBias = s.computeBiasFromBars(daily)
}

// computeBiasFromBars determines trend bias: net positive close-over-open
// in the last htfLookback bars → bullish (+1), negative → bearish (-1).
func (s *IntermarketStrategy) computeBiasFromBars(bars []data.OHLCV) int {
	n := len(bars)
	lookback := s.htfLookback
	if lookback > n {
		lookback = n
	}
	if lookback == 0 {
		return 0
	}

	netMove := 0.0
	for j := n - lookback; j < n; j++ {
		netMove += bars[j].Close - bars[j].Open
	}
	if netMove > 0 {
		return 1
	}
	if netMove < 0 {
		return -1
	}
	return 0
}

func (s *IntermarketStrategy) Signal(i int) SignalType {
	if i < s.swingPeriod*2+3 || math.IsNaN(s.atr[i]) || s.atr[i] == 0 {
		return NoSignal
	}
	if i-s.lastSigBar < s.swingPeriod {
		return NoSignal
	}
	if s.htfBias == 0 {
		return NoSignal
	}

	bar := s.bars[i]
	rng := bar.High - bar.Low
	if rng == 0 {
		return NoSignal
	}
	atr := s.atr[i]

	// Find the most recent confirmed swing high and low
	recentSwingHigh := math.NaN()
	recentSwingLow := math.NaN()
	start := i - s.swingPeriod*4
	if start < 0 {
		start = 0
	}
	for j := i - 1; j >= start; j-- {
		if math.IsNaN(recentSwingHigh) && !math.IsNaN(s.swingHighs[j]) {
			recentSwingHigh = s.swingHighs[j]
		}
		if math.IsNaN(recentSwingLow) && !math.IsNaN(s.swingLows[j]) {
			recentSwingLow = s.swingLows[j]
		}
		if !math.IsNaN(recentSwingHigh) && !math.IsNaN(recentSwingLow) {
			break
		}
	}

	// Bullish HTF: buy the dip
	if s.htfBias == 1 && !math.IsNaN(recentSwingHigh) {
		// Pullback depth: how far price has dropped from recent swing high
		pullback := recentSwingHigh - bar.Low
		if pullback >= s.reversionATR*atr {
			// Confirm with bullish displacement
			body := bar.Close - bar.Open
			if body > 0 && body >= s.dispMult*atr && body/rng >= s.bodyRatio {
				s.lastSigBar = i
				return BuySignal
			}
		}
	}

	// Bearish HTF: sell the rip
	if s.htfBias == -1 && !math.IsNaN(recentSwingLow) {
		// Rally depth: how far price has risen from recent swing low
		rally := bar.High - recentSwingLow
		if rally >= s.reversionATR*atr {
			// Confirm with bearish displacement
			body := bar.Open - bar.Close
			if body > 0 && body >= s.dispMult*atr && body/rng >= s.bodyRatio {
				s.lastSigBar = i
				return SellSignal
			}
		}
	}

	return NoSignal
}
