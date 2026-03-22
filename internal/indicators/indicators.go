package indicators

import (
	"math"
	"trading-backtest-bot/internal/data"
)

// ── Simple Moving Average ──────────────────────────────────────────────────

func SMA(closes []float64, period int) []float64 {
	result := make([]float64, len(closes))
	if len(closes) == 0 || period <= 0 {
		return result
	}
	sum := 0.0
	for i := range closes {
		sum += closes[i]
		if i < period-1 {
			result[i] = math.NaN()
			continue
		}
		if i >= period {
			sum -= closes[i-period]
		}
		result[i] = sum / float64(period)
	}
	return result
}

// ── Exponential Moving Average ─────────────────────────────────────────────

func EMA(closes []float64, period int) []float64 {
	result := make([]float64, len(closes))
	k := 2.0 / float64(period+1)
	for i, c := range closes {
		if i == 0 {
			result[i] = c
		} else {
			result[i] = c*k + result[i-1]*(1-k)
		}
	}
	return result
}

// ── RSI ────────────────────────────────────────────────────────────────────

func RSI(closes []float64, period int) []float64 {
	result := make([]float64, len(closes))
	if len(closes) < period+1 {
		for i := range result {
			result[i] = math.NaN()
		}
		return result
	}

	var gains, losses float64
	for i := 1; i <= period; i++ {
		diff := closes[i] - closes[i-1]
		if diff > 0 {
			gains += diff
		} else {
			losses -= diff
		}
	}
	avgGain := gains / float64(period)
	avgLoss := losses / float64(period)

	for i := 0; i < period; i++ {
		result[i] = math.NaN()
	}

	if avgLoss == 0 {
		result[period] = 100
	} else {
		rs := avgGain / avgLoss
		result[period] = 100 - 100/(1+rs)
	}

	for i := period + 1; i < len(closes); i++ {
		diff := closes[i] - closes[i-1]
		gain, loss := 0.0, 0.0
		if diff > 0 {
			gain = diff
		} else {
			loss = -diff
		}
		avgGain = (avgGain*float64(period-1) + gain) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + loss) / float64(period)
		if avgLoss == 0 {
			result[i] = 100
		} else {
			rs := avgGain / avgLoss
			result[i] = 100 - 100/(1+rs)
		}
	}
	return result
}

// ── MACD ───────────────────────────────────────────────────────────────────

type MACDResult struct {
	MACD      []float64
	Signal    []float64
	Histogram []float64
}

func MACD(closes []float64, fast, slow, signal int) MACDResult {
	emaFast := EMA(closes, fast)
	emaSlow := EMA(closes, slow)

	macdLine := make([]float64, len(closes))
	for i := range closes {
		macdLine[i] = emaFast[i] - emaSlow[i]
	}

	signalLine := EMA(macdLine, signal)
	histogram := make([]float64, len(closes))
	for i := range closes {
		histogram[i] = macdLine[i] - signalLine[i]
	}

	return MACDResult{MACD: macdLine, Signal: signalLine, Histogram: histogram}
}

// ── Bollinger Bands ────────────────────────────────────────────────────────

type BBResult struct {
	Upper  []float64
	Middle []float64
	Lower  []float64
}

func BollingerBands(closes []float64, period int, stdDev float64) BBResult {
	middle := SMA(closes, period)
	upper := make([]float64, len(closes))
	lower := make([]float64, len(closes))

	for i := range closes {
		if i < period-1 {
			upper[i] = math.NaN()
			lower[i] = math.NaN()
			continue
		}
		mean := middle[i]
		variance := 0.0
		for j := i - period + 1; j <= i; j++ {
			diff := closes[j] - mean
			variance += diff * diff
		}
		std := math.Sqrt(variance / float64(period))
		upper[i] = middle[i] + stdDev*std
		lower[i] = middle[i] - stdDev*std
	}
	return BBResult{Upper: upper, Middle: middle, Lower: lower}
}

// ── ATR (Average True Range) ───────────────────────────────────────────────

func ATR(bars []data.OHLCV, period int) []float64 {
	result := make([]float64, len(bars))
	if len(bars) == 0 {
		return result
	}

	trueRanges := make([]float64, len(bars))
	trueRanges[0] = bars[0].High - bars[0].Low

	for i := 1; i < len(bars); i++ {
		hl := bars[i].High - bars[i].Low
		hc := math.Abs(bars[i].High - bars[i-1].Close)
		lc := math.Abs(bars[i].Low - bars[i-1].Close)
		trueRanges[i] = math.Max(hl, math.Max(hc, lc))
	}

	// First ATR = simple average
	sum := 0.0
	for i := 0; i < period && i < len(trueRanges); i++ {
		sum += trueRanges[i]
		result[i] = math.NaN()
	}
	if period <= len(bars) {
		result[period-1] = sum / float64(period)
		for i := period; i < len(bars); i++ {
			result[i] = (result[i-1]*float64(period-1) + trueRanges[i]) / float64(period)
		}
	}
	return result
}

// ── Stochastic Oscillator ──────────────────────────────────────────────────

type StochResult struct {
	K []float64
	D []float64
}

func Stochastic(bars []data.OHLCV, kPeriod, dPeriod int) StochResult {
	k := make([]float64, len(bars))
	for i := range bars {
		if i < kPeriod-1 {
			k[i] = math.NaN()
			continue
		}
		lowest := bars[i].Low
		highest := bars[i].High
		for j := i - kPeriod + 1; j <= i; j++ {
			if bars[j].Low < lowest {
				lowest = bars[j].Low
			}
			if bars[j].High > highest {
				highest = bars[j].High
			}
		}
		if highest-lowest == 0 {
			k[i] = 50
		} else {
			k[i] = (bars[i].Close-lowest)/(highest-lowest)*100
		}
	}
	d := make([]float64, len(bars))
	// Compute %D as SMA of valid %K values only
	for i := range bars {
		if i < kPeriod-1+dPeriod-1 {
			d[i] = math.NaN()
			continue
		}
		sum := 0.0
		for j := i - dPeriod + 1; j <= i; j++ {
			sum += k[j]
		}
		d[i] = sum / float64(dPeriod)
	}
	return StochResult{K: k, D: d}
}

// ── VWAP ──────────────────────────────────────────────────────────────────

func VWAP(bars []data.OHLCV) []float64 {
	result := make([]float64, len(bars))
	cumTPV := 0.0
	cumVol := 0.0
	for i, b := range bars {
		tp := (b.High + b.Low + b.Close) / 3
		cumTPV += tp * b.Volume
		cumVol += b.Volume
		if cumVol > 0 {
			result[i] = cumTPV / cumVol
		} else {
			result[i] = math.NaN()
		}
	}
	return result
}

// ── Donchian Channel ──────────────────────────────────────────────────────

type DonchianResult struct {
	Upper  []float64
	Lower  []float64
	Middle []float64
}

func Donchian(bars []data.OHLCV, period int) DonchianResult {
	upper := make([]float64, len(bars))
	lower := make([]float64, len(bars))
	middle := make([]float64, len(bars))
	for i := range bars {
		if i < period-1 {
			upper[i] = math.NaN()
			lower[i] = math.NaN()
			middle[i] = math.NaN()
			continue
		}
		hi := bars[i-period+1].High
		lo := bars[i-period+1].Low
		for j := i - period + 2; j <= i; j++ {
			if bars[j].High > hi {
				hi = bars[j].High
			}
			if bars[j].Low < lo {
				lo = bars[j].Low
			}
		}
		upper[i] = hi
		lower[i] = lo
		middle[i] = (hi + lo) / 2
	}
	return DonchianResult{Upper: upper, Lower: lower, Middle: middle}
}

// ── Supertrend ────────────────────────────────────────────────────────────

type SupertrendResult struct {
	Value     []float64
	Direction []int // 1 = bullish, -1 = bearish
}

func Supertrend(bars []data.OHLCV, period int, multiplier float64) SupertrendResult {
	atr := ATR(bars, period)
	n := len(bars)
	upperBand := make([]float64, n)
	lowerBand := make([]float64, n)
	supertrend := make([]float64, n)
	direction := make([]int, n)

	for i := range bars {
		hl2 := (bars[i].High + bars[i].Low) / 2
		upperBand[i] = hl2 + multiplier*atr[i]
		lowerBand[i] = hl2 - multiplier*atr[i]
	}

	// Initialize first bar
	supertrend[0] = math.NaN()
	direction[0] = 1 // default bullish until proven otherwise

	for i := 1; i < n; i++ {
		if math.IsNaN(atr[i]) {
			supertrend[i] = math.NaN()
			continue
		}
		// Adjust bands
		// Lower band ratchets UP during uptrends
		if bars[i-1].Close > lowerBand[i-1] {
			lowerBand[i] = math.Max(lowerBand[i], lowerBand[i-1])
		}
		// Upper band ratchets DOWN during downtrends
		if bars[i-1].Close < upperBand[i-1] {
			upperBand[i] = math.Min(upperBand[i], upperBand[i-1])
		}

		if bars[i].Close > upperBand[i-1] {
			direction[i] = 1
		} else if bars[i].Close < lowerBand[i-1] {
			direction[i] = -1
		} else {
			direction[i] = direction[i-1]
		}

		if direction[i] == 1 {
			supertrend[i] = lowerBand[i]
		} else {
			supertrend[i] = upperBand[i]
		}
	}
	return SupertrendResult{Value: supertrend, Direction: direction}
}

// ── Helpers ───────────────────────────────────────────────────────────────

func replaceNaN(s []float64) []float64 {
	out := make([]float64, len(s))
	copy(out, s)
	for i, v := range out {
		if math.IsNaN(v) {
			out[i] = 0
		}
	}
	return out
}

// ExtractClose extracts close prices from bars
func ExtractClose(bars []data.OHLCV) []float64 {
	out := make([]float64, len(bars))
	for i, b := range bars {
		out[i] = b.Close
	}
	return out
}

// Last returns the last non-NaN value in a slice
func Last(s []float64) float64 {
	for i := len(s) - 1; i >= 0; i-- {
		if !math.IsNaN(s[i]) {
			return s[i]
		}
	}
	return math.NaN()
}
