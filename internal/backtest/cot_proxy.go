package backtest

import (
	"math"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── COT Proxy Strategy ──────────────────────────────────────────────────
//
// Since actual COT (Commitment of Traders) data is not available, this
// strategy uses volume analysis as a proxy for institutional positioning:
//
//   Rising price + rising volume   = accumulation  (bullish)
//   Rising price + falling volume  = distribution  (bearish warning)
//   Falling price + rising volume  = distribution  (bearish)
//   Falling price + falling volume = accumulation  (bullish warning)
//
// A "smart money index" is computed as a rolling correlation between
// price changes and volume changes over vol_period bars. When the index
// crosses below -divergence_threshold (negative correlation = divergence),
// a reversal is expected:
//   - If price was rising but volume diverging → sell
//   - If price was falling but volume diverging → buy
//
// Entry requires displacement confirmation in the reversal direction.
//
// Parameters:
//   swing_period          – signal throttle (default 5)
//   atr_period            – ATR period (default 14)
//   disp_mult             – displacement ATR multiple (default 1.0)
//   body_ratio            – min body/range ratio (default 0.5)
//   vol_period            – rolling window for correlation (default 20)
//   divergence_threshold  – abs correlation below which divergence fires (default 0.5)

type COTProxyStrategy struct {
	bars        []data.OHLCV
	atr         []float64
	swingPeriod int
	dispMult    float64
	bodyRatio   float64
	volPeriod   int
	divThresh   float64
	lastSigBar  int

	// Precomputed per-bar
	priceChg []float64 // close[i] - close[i-1]
	volChg   []float64 // volume[i] - volume[i-1]
	corr     []float64 // rolling correlation
}

func (s *COTProxyStrategy) Name() string { return "COT Proxy" }
func (s *COTProxyStrategy) Description() string {
	return "Volume-based institutional positioning proxy: enter on price-volume divergence with displacement"
}

func (s *COTProxyStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	s.swingPeriod = int(getParam(params, "swing_period", 5))
	atrPeriod := int(getParam(params, "atr_period", 14))
	s.dispMult = getParam(params, "disp_mult", 1.0)
	s.bodyRatio = getParam(params, "body_ratio", 0.5)
	s.volPeriod = int(getParam(params, "vol_period", 20))
	s.divThresh = getParam(params, "divergence_threshold", 0.5)
	s.lastSigBar = -10

	s.atr = indicators.ATR(bars, atrPeriod)

	n := len(bars)
	s.priceChg = make([]float64, n)
	s.volChg = make([]float64, n)
	s.corr = make([]float64, n)

	for i := 0; i < n; i++ {
		if i == 0 {
			s.priceChg[i] = 0
			s.volChg[i] = 0
		} else {
			s.priceChg[i] = bars[i].Close - bars[i-1].Close
			s.volChg[i] = bars[i].Volume - bars[i-1].Volume
		}
		s.corr[i] = math.NaN()
	}

	// Compute rolling Pearson correlation
	for i := s.volPeriod; i < n; i++ {
		s.corr[i] = s.pearson(i-s.volPeriod+1, i+1)
	}
}

// pearson computes Pearson correlation between priceChg and volChg in [start, end).
func (s *COTProxyStrategy) pearson(start, end int) float64 {
	n := float64(end - start)
	if n < 3 {
		return math.NaN()
	}

	var sumX, sumY, sumXY, sumX2, sumY2 float64
	for i := start; i < end; i++ {
		x := s.priceChg[i]
		y := s.volChg[i]
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
		sumY2 += y * y
	}

	denom := math.Sqrt((n*sumX2 - sumX*sumX) * (n*sumY2 - sumY*sumY))
	if denom == 0 {
		return 0
	}
	return (n*sumXY - sumX*sumY) / denom
}

// netPriceTrend returns +1 if net price change over vol_period is positive,
// -1 if negative, 0 otherwise.
func (s *COTProxyStrategy) netPriceTrend(i int) int {
	start := i - s.volPeriod + 1
	if start < 0 {
		start = 0
	}
	net := s.bars[i].Close - s.bars[start].Close
	if net > 0 {
		return 1
	}
	if net < 0 {
		return -1
	}
	return 0
}

func (s *COTProxyStrategy) Signal(i int) SignalType {
	if i < s.volPeriod+1 || math.IsNaN(s.atr[i]) || s.atr[i] == 0 {
		return NoSignal
	}
	if i-s.lastSigBar < s.swingPeriod {
		return NoSignal
	}
	if math.IsNaN(s.corr[i]) || math.IsNaN(s.corr[i-1]) {
		return NoSignal
	}

	bar := s.bars[i]
	rng := bar.High - bar.Low
	if rng == 0 {
		return NoSignal
	}
	atr := s.atr[i]

	// Check for divergence: correlation crosses below -threshold
	prevCorr := s.corr[i-1]
	curCorr := s.corr[i]
	divergenceNow := curCorr <= -s.divThresh && prevCorr > -s.divThresh

	if !divergenceNow {
		return NoSignal
	}

	priceTrend := s.netPriceTrend(i)

	// Price was rising + divergence → expect reversal down → sell
	if priceTrend == 1 {
		body := bar.Open - bar.Close
		if body > 0 && body >= s.dispMult*atr && body/rng >= s.bodyRatio {
			s.lastSigBar = i
			return SellSignal
		}
	}

	// Price was falling + divergence → expect reversal up → buy
	if priceTrend == -1 {
		body := bar.Close - bar.Open
		if body > 0 && body >= s.dispMult*atr && body/rng >= s.bodyRatio {
			s.lastSigBar = i
			return BuySignal
		}
	}

	return NoSignal
}
