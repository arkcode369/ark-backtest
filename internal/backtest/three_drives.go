package backtest

import (
	"math"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── Three Drives Pattern Strategy ────────────────────────────────────────
//
// Detects the Three Drives harmonic pattern:
//
// Bearish Three Drives (sell signal):
//   3 consecutive swing highs, each progressively higher (drive1 < drive2 < drive3).
//   Each extension should be 127-161.8% of the prior drive's retracement.
//   After the 3rd drive completes, expect reversal down.
//   Confirm with bearish displacement candle.
//
// Bullish Three Drives (buy signal):
//   3 consecutive swing lows, each progressively lower (drive1 > drive2 > drive3).
//   After the 3rd drive completes, expect reversal up.
//   Confirm with bullish displacement candle.
//
// Parameters:
//   swing_period – swing detection period (default 5)
//   atr_period   – ATR calculation period (default 14)
//   disp_mult    – displacement ATR multiple (default 1.0)
//   body_ratio   – min body/range ratio (default 0.5)
//   fib_min      – min extension ratio between drives (default 1.1)
//   fib_max      – max extension ratio between drives (default 1.8)
//   lookback     – lookback window for finding patterns (default 50)

type ThreeDrivesStrategy struct {
	bars        []data.OHLCV
	atr         []float64
	swingHighs  []float64
	swingLows   []float64
	swingPeriod int
	dispMult    float64
	bodyRatio   float64
	fibMin      float64
	fibMax      float64
	lookback    int
	lastSigBar  int
}

func (s *ThreeDrivesStrategy) Name() string { return "Three Drives" }
func (s *ThreeDrivesStrategy) Description() string {
	return "Three Drives harmonic pattern: 3 progressive swing points with fib extensions → displacement reversal"
}

func (s *ThreeDrivesStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	s.swingPeriod = int(getParam(params, "swing_period", 5))
	atrPeriod := int(getParam(params, "atr_period", 14))
	s.dispMult = getParam(params, "disp_mult", 1.0)
	s.bodyRatio = getParam(params, "body_ratio", 0.5)
	s.fibMin = getParam(params, "fib_min", 1.1)
	s.fibMax = getParam(params, "fib_max", 1.8)
	s.lookback = int(getParam(params, "lookback", 50))
	if s.lookback < s.swingPeriod*4 {
		s.lookback = s.swingPeriod * 4
	}
	s.lastSigBar = -s.lookback

	s.atr = indicators.ATR(bars, atrPeriod)
	s.swingHighs = indicators.SwingHighs(bars, s.swingPeriod)
	s.swingLows = indicators.SwingLows(bars, s.swingPeriod)
}

func (s *ThreeDrivesStrategy) Signal(i int) SignalType {
	if i < s.swingPeriod*2+3 || math.IsNaN(s.atr[i]) {
		return NoSignal
	}
	if i-s.lastSigBar < s.swingPeriod {
		return NoSignal
	}

	if s.checkBullishThreeDrives(i) {
		s.lastSigBar = i
		return BuySignal
	}
	if s.checkBearishThreeDrives(i) {
		s.lastSigBar = i
		return SellSignal
	}
	return NoSignal
}

// drivePoint holds the index and price of a swing used in the pattern.
type drivePoint struct {
	idx   int
	price float64
}

// collectRecentSwingHighs gathers confirmed swing highs in [start, end) ordered by index.
func (s *ThreeDrivesStrategy) collectRecentSwingHighs(start, end int) []drivePoint {
	var pts []drivePoint
	for j := start; j < end; j++ {
		if !math.IsNaN(s.swingHighs[j]) {
			pts = append(pts, drivePoint{idx: j, price: s.swingHighs[j]})
		}
	}
	return pts
}

// collectRecentSwingLows gathers confirmed swing lows in [start, end) ordered by index.
func (s *ThreeDrivesStrategy) collectRecentSwingLows(start, end int) []drivePoint {
	var pts []drivePoint
	for j := start; j < end; j++ {
		if !math.IsNaN(s.swingLows[j]) {
			pts = append(pts, drivePoint{idx: j, price: s.swingLows[j]})
		}
	}
	return pts
}

// findRetracementLow finds the lowest low between two bar indices (exclusive).
func (s *ThreeDrivesStrategy) findRetracementLow(from, to int) float64 {
	low := s.bars[from].Low
	for j := from + 1; j < to; j++ {
		if s.bars[j].Low < low {
			low = s.bars[j].Low
		}
	}
	return low
}

// findRetracementHigh finds the highest high between two bar indices (exclusive).
func (s *ThreeDrivesStrategy) findRetracementHigh(from, to int) float64 {
	high := s.bars[from].High
	for j := from + 1; j < to; j++ {
		if s.bars[j].High > high {
			high = s.bars[j].High
		}
	}
	return high
}

// checkBearishThreeDrives: 3 progressively higher swing highs with fib extensions → bearish displacement.
func (s *ThreeDrivesStrategy) checkBearishThreeDrives(i int) bool {
	start := i - s.lookback
	if start < s.swingPeriod {
		start = s.swingPeriod
	}

	highs := s.collectRecentSwingHighs(start, i-s.swingPeriod+1)
	if len(highs) < 3 {
		return false
	}

	// Check combinations of 3 consecutive swing highs (most recent triplets first)
	for k := len(highs) - 1; k >= 2; k-- {
		d1 := highs[k-2]
		d2 := highs[k-1]
		d3 := highs[k]

		// Must be progressively higher
		if d2.price <= d1.price || d3.price <= d2.price {
			continue
		}

		// Check fib extension ratios between drives.
		// Drive 1 move: d1.price - retracement low between d1 and d2
		retrace1 := s.findRetracementLow(d1.idx, d2.idx)
		move1 := d1.price - retrace1
		if move1 <= 0 {
			continue
		}

		// Drive 2 extension: (d2.price - retrace1) / move1
		ext2 := (d2.price - retrace1) / move1
		if ext2 < s.fibMin || ext2 > s.fibMax {
			continue
		}

		// Drive 2 move
		retrace2 := s.findRetracementLow(d2.idx, d3.idx)
		move2 := d2.price - retrace2
		if move2 <= 0 {
			continue
		}

		// Drive 3 extension: (d3.price - retrace2) / move2
		ext3 := (d3.price - retrace2) / move2
		if ext3 < s.fibMin || ext3 > s.fibMax {
			continue
		}

		// Pattern complete at d3. Look for bearish displacement after d3.
		for d := d3.idx; d <= i; d++ {
			if d >= len(s.bars) || math.IsNaN(s.atr[d]) || s.atr[d] == 0 {
				continue
			}
			body := s.bars[d].Open - s.bars[d].Close
			rng := s.bars[d].High - s.bars[d].Low
			if rng == 0 {
				continue
			}
			if body >= s.dispMult*s.atr[d] && body/rng >= s.bodyRatio {
				if d == i || d == i-1 {
					return true
				}
			}
		}
	}
	return false
}

// checkBullishThreeDrives: 3 progressively lower swing lows with fib extensions → bullish displacement.
func (s *ThreeDrivesStrategy) checkBullishThreeDrives(i int) bool {
	start := i - s.lookback
	if start < s.swingPeriod {
		start = s.swingPeriod
	}

	lows := s.collectRecentSwingLows(start, i-s.swingPeriod+1)
	if len(lows) < 3 {
		return false
	}

	for k := len(lows) - 1; k >= 2; k-- {
		d1 := lows[k-2]
		d2 := lows[k-1]
		d3 := lows[k]

		// Must be progressively lower
		if d2.price >= d1.price || d3.price >= d2.price {
			continue
		}

		// Drive 1 move (downward, so retrace is a high)
		retrace1 := s.findRetracementHigh(d1.idx, d2.idx)
		move1 := retrace1 - d1.price
		if move1 <= 0 {
			continue
		}

		// Drive 2 extension: (retrace1 - d2.price) / move1
		ext2 := (retrace1 - d2.price) / move1
		if ext2 < s.fibMin || ext2 > s.fibMax {
			continue
		}

		// Drive 2 move
		retrace2 := s.findRetracementHigh(d2.idx, d3.idx)
		move2 := retrace2 - d2.price
		if move2 <= 0 {
			continue
		}

		// Drive 3 extension: (retrace2 - d3.price) / move2
		ext3 := (retrace2 - d3.price) / move2
		if ext3 < s.fibMin || ext3 > s.fibMax {
			continue
		}

		// Pattern complete at d3. Look for bullish displacement after d3.
		for d := d3.idx; d <= i; d++ {
			if d >= len(s.bars) || math.IsNaN(s.atr[d]) || s.atr[d] == 0 {
				continue
			}
			body := s.bars[d].Close - s.bars[d].Open
			rng := s.bars[d].High - s.bars[d].Low
			if rng == 0 {
				continue
			}
			if body >= s.dispMult*s.atr[d] && body/rng >= s.bodyRatio {
				if d == i || d == i-1 {
					return true
				}
			}
		}
	}
	return false
}
