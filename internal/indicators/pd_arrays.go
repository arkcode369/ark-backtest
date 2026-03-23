package indicators

import (
	"math"
	"sort"
	"trading-backtest-bot/internal/data"
)

// ═══════════════════════════════════════════════════════════════════════════
// Types & Constants
// ═══════════════════════════════════════════════════════════════════════════

// PDArrayType identifies the type of PD Array
type PDArrayType int

const (
	PDFairValueGap    PDArrayType = iota // FVG
	PDOrderBlock                         // OB
	PDBreakerBlock                       // Breaker
	PDMitigationBlock                    // Mitigation Block
	PDPropulsionBlock                    // OB + FVG overlap
	PDVolumeImbalance                    // 2-candle gap (close→open)
	PDInverseFVG                         // IFVG (filled FVG flipped)
	PDImpliedFVG                         // Wick-based gap
	PDBalancePriceRange                  // BPR (bull+bear FVG overlap)
	PDVacuumBlock                        // Event-driven large gap
	PDRejectionBlock                     // Long wick rejection
	PDBreakawayGap                       // Post-consolidation gap
)

// PDDirection indicates bullish or bearish
type PDDirection int

const (
	Bullish PDDirection = 1
	Bearish PDDirection = -1
)

// PDZone represents a detected PD Array zone
type PDZone struct {
	Type      PDArrayType
	Direction PDDirection
	Index     int     // bar index where detected
	Top       float64 // upper boundary of zone
	Bottom    float64 // lower boundary of zone
	MidPoint  float64 // (Top + Bottom) / 2
	Mitigated bool    // whether zone has been filled/invalidated
	MitIndex  int     // bar index where mitigated (-1 if not)
}

// PDParams holds configuration for all PD Array detectors
type PDParams struct {
	ATRPeriod     int     // default 14
	SwingPeriod   int     // default 5
	OBImpulseMult float64 // ATR multiplier for OB impulse (default 1.5)
	VacuumMult    float64 // ATR multiplier for vacuum blocks (default 1.0)
	WickRatio     float64 // wick/body ratio for rejection blocks (default 2.0)
	Lookback      int     // premium/discount lookback (default 50)
}

// DefaultPDParams returns sensible default parameters
func DefaultPDParams() PDParams {
	return PDParams{
		ATRPeriod:     14,
		SwingPeriod:   5,
		OBImpulseMult: 1.5,
		VacuumMult:    1.0,
		WickRatio:     2.0,
		Lookback:      50,
	}
}

// PremiumDiscountResult holds premium/discount zone analysis
type PremiumDiscountResult struct {
	SwingHigh   float64
	SwingLow    float64
	Equilibrium float64 // 50%
	OTEPremium  float64 // 79%
	FibSell     float64 // 62%
	FibBuy      float64 // 38%
	OTEDiscount float64 // 21%
	InPremium   []bool  // per bar: close > equilibrium
	InDiscount  []bool  // per bar: close < equilibrium
	InOTEBuy    []bool  // close in 21%-38% zone
	InOTESell   []bool  // close in 62%-79% zone
}

// ═══════════════════════════════════════════════════════════════════════════
// Helpers
// ═══════════════════════════════════════════════════════════════════════════

func makePDZone(typ PDArrayType, dir PDDirection, idx int, top, bottom float64) PDZone {
	return PDZone{
		Type:      typ,
		Direction: dir,
		Index:     idx,
		Top:       top,
		Bottom:    bottom,
		MidPoint:  (top + bottom) / 2.0,
		Mitigated: false,
		MitIndex:  -1,
	}
}

func bodySize(bar data.OHLCV) float64 {
	return math.Abs(bar.Close - bar.Open)
}

func isBullishCandle(bar data.OHLCV) bool {
	return bar.Close > bar.Open
}

func isBearishCandle(bar data.OHLCV) bool {
	return bar.Close < bar.Open
}

func candleRange(bar data.OHLCV) float64 {
	return bar.High - bar.Low
}

func upperWick(bar data.OHLCV) float64 {
	top := math.Max(bar.Open, bar.Close)
	return bar.High - top
}

func lowerWick(bar data.OHLCV) float64 {
	bottom := math.Min(bar.Open, bar.Close)
	return bottom - bar.Low
}

// overlapZone computes the intersection of two vertical ranges.
// Returns (top, bottom, ok) where ok is true if there is a valid overlap.
func overlapZone(top1, bot1, top2, bot2 float64) (float64, float64, bool) {
	oTop := math.Min(top1, top2)
	oBot := math.Max(bot1, bot2)
	if oTop > oBot {
		return oTop, oBot, true
	}
	return 0, 0, false
}

// ═══════════════════════════════════════════════════════════════════════════
// 1. FVG (Fair Value Gap)
// ═══════════════════════════════════════════════════════════════════════════

func detectFVGs(bars []data.OHLCV) []PDZone {
	n := len(bars)
	if n < 3 {
		return nil
	}
	var zones []PDZone

	for i := 1; i < n-1; i++ {
		// Bullish FVG: gap between bar[i-1].High and bar[i+1].Low
		if bars[i+1].Low > bars[i-1].High {
			z := makePDZone(PDFairValueGap, Bullish, i, bars[i+1].Low, bars[i-1].High)
			// Check mitigation forward
			for j := i + 2; j < n; j++ {
				if bars[j].Low <= z.Top {
					z.Mitigated = true
					z.MitIndex = j
					break
				}
			}
			zones = append(zones, z)
		}
		// Bearish FVG: gap between bar[i-1].Low and bar[i+1].High
		if bars[i+1].High < bars[i-1].Low {
			z := makePDZone(PDFairValueGap, Bearish, i, bars[i-1].Low, bars[i+1].High)
			// Check mitigation forward
			for j := i + 2; j < n; j++ {
				if bars[j].High >= z.Bottom {
					z.Mitigated = true
					z.MitIndex = j
					break
				}
			}
			zones = append(zones, z)
		}
	}
	return zones
}

// ═══════════════════════════════════════════════════════════════════════════
// 2. Order Block
// ═══════════════════════════════════════════════════════════════════════════

func detectOrderBlocks(bars []data.OHLCV, atr []float64, impulseMult float64) []PDZone {
	n := len(bars)
	if n < 2 {
		return nil
	}
	var zones []PDZone

	for i := 1; i < n; i++ {
		if math.IsNaN(atr[i]) || atr[i] == 0 {
			continue
		}
		impulseRange := candleRange(bars[i])

		// Bullish OB: previous candle bearish, current candle bullish impulse
		if isBearishCandle(bars[i-1]) && isBullishCandle(bars[i]) && impulseRange > atr[i]*impulseMult {
			top := bars[i-1].Open
			bottom := bars[i-1].Low
			z := makePDZone(PDOrderBlock, Bullish, i, top, bottom)
			// Mitigation: price trades through the OB
			for j := i + 1; j < n; j++ {
				if bars[j].Low <= z.Top {
					z.Mitigated = true
					z.MitIndex = j
					break
				}
			}
			zones = append(zones, z)
		}

		// Bearish OB: previous candle bullish, current candle bearish impulse
		if isBullishCandle(bars[i-1]) && isBearishCandle(bars[i]) && impulseRange > atr[i]*impulseMult {
			top := bars[i-1].High
			bottom := bars[i-1].Open
			z := makePDZone(PDOrderBlock, Bearish, i, top, bottom)
			// Mitigation: price trades through the OB
			for j := i + 1; j < n; j++ {
				if bars[j].High >= z.Bottom {
					z.Mitigated = true
					z.MitIndex = j
					break
				}
			}
			zones = append(zones, z)
		}
	}
	return zones
}

// ═══════════════════════════════════════════════════════════════════════════
// 3. Breaker Block
// ═══════════════════════════════════════════════════════════════════════════

func detectBreakerBlocks(bars []data.OHLCV, orderBlocks []PDZone) []PDZone {
	n := len(bars)
	var zones []PDZone

	for _, ob := range orderBlocks {
		// Bullish Breaker: a bearish OB that gets broken above (close > OB top)
		if ob.Direction == Bearish {
			for j := ob.Index + 1; j < n; j++ {
				if bars[j].Close > ob.Top {
					z := makePDZone(PDBreakerBlock, Bullish, j, ob.Top, ob.Bottom)
					// Check mitigation: price returns into breaker zone
					for k := j + 1; k < n; k++ {
						if bars[k].Low <= z.Top {
							z.Mitigated = true
							z.MitIndex = k
							break
						}
					}
					zones = append(zones, z)
					break
				}
			}
		}
		// Bearish Breaker: a bullish OB that gets broken below (close < OB bottom)
		if ob.Direction == Bullish {
			for j := ob.Index + 1; j < n; j++ {
				if bars[j].Close < ob.Bottom {
					z := makePDZone(PDBreakerBlock, Bearish, j, ob.Top, ob.Bottom)
					// Check mitigation
					for k := j + 1; k < n; k++ {
						if bars[k].High >= z.Bottom {
							z.Mitigated = true
							z.MitIndex = k
							break
						}
					}
					zones = append(zones, z)
					break
				}
			}
		}
	}
	return zones
}

// ═══════════════════════════════════════════════════════════════════════════
// 4. Mitigation Block
// ═══════════════════════════════════════════════════════════════════════════

// detectMitigationBlocks finds order blocks that have been mitigated (traded through)
// and marks them as mitigation blocks - these are OBs that failed but may act as
// future support/resistance on a revisit.
func detectMitigationBlocks(orderBlocks []PDZone) []PDZone {
	var zones []PDZone
	for _, ob := range orderBlocks {
		if ob.Mitigated {
			z := PDZone{
				Type:      PDMitigationBlock,
				Direction: ob.Direction,
				Index:     ob.MitIndex,
				Top:       ob.Top,
				Bottom:    ob.Bottom,
				MidPoint:  ob.MidPoint,
				Mitigated: true,
				MitIndex:  ob.MitIndex,
			}
			zones = append(zones, z)
		}
	}
	return zones
}

// ═══════════════════════════════════════════════════════════════════════════
// 5. Volume Imbalance
// ═══════════════════════════════════════════════════════════════════════════

func detectVolumeImbalances(bars []data.OHLCV) []PDZone {
	n := len(bars)
	if n < 2 {
		return nil
	}
	var zones []PDZone

	for i := 1; i < n; i++ {
		// Bullish: gap up open
		if bars[i].Open > bars[i-1].Close {
			z := makePDZone(PDVolumeImbalance, Bullish, i, bars[i].Open, bars[i-1].Close)
			for j := i + 1; j < n; j++ {
				if bars[j].Low <= z.Top {
					z.Mitigated = true
					z.MitIndex = j
					break
				}
			}
			zones = append(zones, z)
		}
		// Bearish: gap down open
		if bars[i].Open < bars[i-1].Close {
			z := makePDZone(PDVolumeImbalance, Bearish, i, bars[i-1].Close, bars[i].Open)
			for j := i + 1; j < n; j++ {
				if bars[j].High >= z.Bottom {
					z.Mitigated = true
					z.MitIndex = j
					break
				}
			}
			zones = append(zones, z)
		}
	}
	return zones
}

// ═══════════════════════════════════════════════════════════════════════════
// 6. IFVG (Inverse FVG)
// ═══════════════════════════════════════════════════════════════════════════

// detectInverseFVGs finds FVGs that have been fully filled and flips their direction.
// A bullish FVG that gets fully broken downward becomes a bearish IFVG.
// A bearish FVG that gets fully broken upward becomes a bullish IFVG.
func detectInverseFVGs(bars []data.OHLCV, fvgs []PDZone) []PDZone {
	n := len(bars)
	var zones []PDZone

	for _, fvg := range fvgs {
		if fvg.Type != PDFairValueGap {
			continue
		}

		// Bullish IFVG: a bearish FVG that gets fully broken upward
		if fvg.Direction == Bearish {
			for j := fvg.Index + 2; j < n; j++ {
				// Fully broken: price closed above the top of the bearish FVG
				if bars[j].Close > fvg.Top {
					z := makePDZone(PDInverseFVG, Bullish, j, fvg.Top, fvg.Bottom)
					// Check mitigation
					for k := j + 1; k < n; k++ {
						if bars[k].Low <= z.Top {
							z.Mitigated = true
							z.MitIndex = k
							break
						}
					}
					zones = append(zones, z)
					break
				}
			}
		}

		// Bearish IFVG: a bullish FVG that gets fully broken downward
		if fvg.Direction == Bullish {
			for j := fvg.Index + 2; j < n; j++ {
				// Fully broken: price closed below the bottom of the bullish FVG
				if bars[j].Close < fvg.Bottom {
					z := makePDZone(PDInverseFVG, Bearish, j, fvg.Top, fvg.Bottom)
					// Check mitigation
					for k := j + 1; k < n; k++ {
						if bars[k].High >= z.Bottom {
							z.Mitigated = true
							z.MitIndex = k
							break
						}
					}
					zones = append(zones, z)
					break
				}
			}
		}
	}
	return zones
}

// ═══════════════════════════════════════════════════════════════════════════
// 7. Implied FVG
// ═══════════════════════════════════════════════════════════════════════════

func detectImpliedFVGs(bars []data.OHLCV) []PDZone {
	n := len(bars)
	if n < 2 {
		return nil
	}
	var zones []PDZone

	for i := 1; i < n; i++ {
		// Bullish Implied FVG: open gaps up from previous close but within previous wick
		if bars[i].Open > bars[i-1].Close && bars[i].Open < bars[i-1].High {
			top := bars[i].Open
			bottom := bars[i-1].Close
			if top > bottom {
				z := makePDZone(PDImpliedFVG, Bullish, i, top, bottom)
				for j := i + 1; j < n; j++ {
					if bars[j].Low <= z.Top {
						z.Mitigated = true
						z.MitIndex = j
						break
					}
				}
				zones = append(zones, z)
			}
		}
		// Bearish Implied FVG: open gaps down from previous close but within previous wick
		if bars[i].Open < bars[i-1].Close && bars[i].Open > bars[i-1].Low {
			top := bars[i-1].Close
			bottom := bars[i].Open
			if top > bottom {
				z := makePDZone(PDImpliedFVG, Bearish, i, top, bottom)
				for j := i + 1; j < n; j++ {
					if bars[j].High >= z.Bottom {
						z.Mitigated = true
						z.MitIndex = j
						break
					}
				}
				zones = append(zones, z)
			}
		}
	}
	return zones
}

// ═══════════════════════════════════════════════════════════════════════════
// 8. BPR (Balance Price Range)
// ═══════════════════════════════════════════════════════════════════════════

// detectBPRs finds overlapping bullish and bearish FVGs.
// The overlap area is the Balance Price Range.
func detectBPRs(fvgs []PDZone) []PDZone {
	var bullFVGs, bearFVGs []PDZone
	for _, fvg := range fvgs {
		if fvg.Type != PDFairValueGap {
			continue
		}
		if fvg.Direction == Bullish {
			bullFVGs = append(bullFVGs, fvg)
		} else {
			bearFVGs = append(bearFVGs, fvg)
		}
	}

	var zones []PDZone
	for _, bull := range bullFVGs {
		for _, bear := range bearFVGs {
			oTop, oBot, ok := overlapZone(bull.Top, bull.Bottom, bear.Top, bear.Bottom)
			if ok {
				// Use the later index as the detection index
				idx := bull.Index
				if bear.Index > idx {
					idx = bear.Index
				}
				// Direction is neutral; we default to Bullish for the BPR zone
				z := makePDZone(PDBalancePriceRange, Bullish, idx, oTop, oBot)
				// BPR mitigated when either constituent FVG is mitigated
				if bull.Mitigated || bear.Mitigated {
					z.Mitigated = true
					mi := bull.MitIndex
					if bear.MitIndex > 0 && (mi < 0 || bear.MitIndex < mi) {
						mi = bear.MitIndex
					}
					z.MitIndex = mi
				}
				zones = append(zones, z)
			}
		}
	}
	return zones
}

// ═══════════════════════════════════════════════════════════════════════════
// 9. Premium / Discount
// ═══════════════════════════════════════════════════════════════════════════

// ComputePremiumDiscount computes ROLLING premium/discount levels per bar.
// For each bar i, it finds the highest high and lowest low in
// [max(0, i-lookback+1) .. i], then classifies the bar's close within
// that range's fibonacci levels. This means each bar has its OWN
// swing high/low context — not a single static window.
//
// The top-level SwingHigh/SwingLow/Equilibrium/OTE fields reflect
// the LAST bar's window (for convenience), but the per-bar boolean
// arrays are the real output.
func ComputePremiumDiscount(bars []data.OHLCV, lookback int) PremiumDiscountResult {
	n := len(bars)
	res := PremiumDiscountResult{
		InPremium:  make([]bool, n),
		InDiscount: make([]bool, n),
		InOTEBuy:   make([]bool, n),
		InOTESell:  make([]bool, n),
	}
	if n == 0 || lookback <= 0 {
		return res
	}

	for i := 0; i < n; i++ {
		// Rolling window: [start .. i]
		start := i - lookback + 1
		if start < 0 {
			start = 0
		}

		highest := bars[start].High
		lowest := bars[start].Low
		for j := start + 1; j <= i; j++ {
			if bars[j].High > highest {
				highest = bars[j].High
			}
			if bars[j].Low < lowest {
				lowest = bars[j].Low
			}
		}

		rng := highest - lowest
		if rng == 0 {
			continue
		}

		eq := lowest + rng*0.5
		oteDisc := lowest + rng*0.21
		fibBuy := lowest + rng*0.38
		fibSell := lowest + rng*0.62
		otePrem := lowest + rng*0.79

		c := bars[i].Close
		res.InPremium[i] = c > eq
		res.InDiscount[i] = c < eq
		res.InOTEBuy[i] = c >= oteDisc && c <= fibBuy
		res.InOTESell[i] = c >= fibSell && c <= otePrem

		// Store last bar's levels for the top-level fields
		if i == n-1 {
			res.SwingHigh = highest
			res.SwingLow = lowest
			res.Equilibrium = eq
			res.OTEDiscount = oteDisc
			res.FibBuy = fibBuy
			res.FibSell = fibSell
			res.OTEPremium = otePrem
		}
	}

	return res
}

// ═══════════════════════════════════════════════════════════════════════════
// 10. Vacuum Block
// ═══════════════════════════════════════════════════════════════════════════

func detectVacuumBlocks(bars []data.OHLCV, atr []float64, vacuumMult float64) []PDZone {
	n := len(bars)
	if n < 2 {
		return nil
	}
	var zones []PDZone

	for i := 1; i < n; i++ {
		if math.IsNaN(atr[i]) || atr[i] == 0 {
			continue
		}

		// Bullish vacuum: large gap up open
		if bars[i].Open > bars[i-1].Close {
			gapSize := bars[i].Open - bars[i-1].Close
			if gapSize > atr[i]*vacuumMult {
				z := makePDZone(PDVacuumBlock, Bullish, i, bars[i].Open, bars[i-1].Close)
				for j := i + 1; j < n; j++ {
					if bars[j].Low <= z.Top {
						z.Mitigated = true
						z.MitIndex = j
						break
					}
				}
				zones = append(zones, z)
			}
		}
		// Bearish vacuum: large gap down open
		if bars[i].Open < bars[i-1].Close {
			gapSize := bars[i-1].Close - bars[i].Open
			if gapSize > atr[i]*vacuumMult {
				z := makePDZone(PDVacuumBlock, Bearish, i, bars[i-1].Close, bars[i].Open)
				for j := i + 1; j < n; j++ {
					if bars[j].High >= z.Bottom {
						z.Mitigated = true
						z.MitIndex = j
						break
					}
				}
				zones = append(zones, z)
			}
		}
	}
	return zones
}

// ═══════════════════════════════════════════════════════════════════════════
// 11. Rejection Block
// ═══════════════════════════════════════════════════════════════════════════

func detectRejectionBlocks(bars []data.OHLCV, swingHighs, swingLows []float64, wickRatio float64) []PDZone {
	n := len(bars)
	var zones []PDZone

	for i := 0; i < n; i++ {
		body := bodySize(bars[i])
		if body == 0 {
			// Doji - skip to avoid division issues; use a tiny epsilon
			body = 1e-10
		}
		lw := lowerWick(bars[i])
		uw := upperWick(bars[i])

		// Bullish Rejection Block: long lower wick at a swing low
		if lw > wickRatio*body && !math.IsNaN(swingLows[i]) {
			bottom := bars[i].Low
			top := math.Min(bars[i].Open, bars[i].Close)
			z := makePDZone(PDRejectionBlock, Bullish, i, top, bottom)
			for j := i + 1; j < n; j++ {
				if bars[j].Low <= z.Top {
					z.Mitigated = true
					z.MitIndex = j
					break
				}
			}
			zones = append(zones, z)
		}

		// Bearish Rejection Block: long upper wick at a swing high
		if uw > wickRatio*body && !math.IsNaN(swingHighs[i]) {
			top := bars[i].High
			bottom := math.Max(bars[i].Open, bars[i].Close)
			z := makePDZone(PDRejectionBlock, Bearish, i, top, bottom)
			for j := i + 1; j < n; j++ {
				if bars[j].High >= z.Bottom {
					z.Mitigated = true
					z.MitIndex = j
					break
				}
			}
			zones = append(zones, z)
		}
	}
	return zones
}

// ═══════════════════════════════════════════════════════════════════════════
// 12. Propulsion Block (OB + FVG overlap)
// ═══════════════════════════════════════════════════════════════════════════

func detectPropulsionBlocks(orderBlocks, fvgs []PDZone) []PDZone {
	var zones []PDZone

	for _, ob := range orderBlocks {
		for _, fvg := range fvgs {
			if fvg.Type != PDFairValueGap {
				continue
			}
			// Must be same direction and close in bar index
			if ob.Direction != fvg.Direction {
				continue
			}
			idxDiff := ob.Index - fvg.Index
			if idxDiff < 0 {
				idxDiff = -idxDiff
			}
			if idxDiff > 2 {
				continue
			}

			oTop, oBot, ok := overlapZone(ob.Top, ob.Bottom, fvg.Top, fvg.Bottom)
			if ok {
				idx := ob.Index
				if fvg.Index > idx {
					idx = fvg.Index
				}
				z := makePDZone(PDPropulsionBlock, ob.Direction, idx, oTop, oBot)
				// Inherit mitigation from either
				if ob.Mitigated || fvg.Mitigated {
					z.Mitigated = true
					mi := ob.MitIndex
					if fvg.MitIndex > 0 && (mi < 0 || fvg.MitIndex < mi) {
						mi = fvg.MitIndex
					}
					z.MitIndex = mi
				}
				zones = append(zones, z)
			}
		}
	}
	return zones
}

// ═══════════════════════════════════════════════════════════════════════════
// 13. Breakaway Gap
// ═══════════════════════════════════════════════════════════════════════════

// detectBreakawayGaps finds large gaps that occur after tight consolidation.
// Consolidation is defined as a period where ATR is below its own average.
func detectBreakawayGaps(bars []data.OHLCV, atr []float64) []PDZone {
	n := len(bars)
	if n < 2 {
		return nil
	}
	var zones []PDZone

	// Compute a simple rolling average of ATR (20-period) for consolidation detection
	const consolidationLookback = 20
	atrAvg := make([]float64, n)
	for i := range atrAvg {
		atrAvg[i] = math.NaN()
	}
	sum := 0.0
	count := 0
	for i := 0; i < n; i++ {
		if math.IsNaN(atr[i]) {
			continue
		}
		sum += atr[i]
		count++
		if count > consolidationLookback {
			// Find the oldest valid ATR to subtract
			for back := i - consolidationLookback; back >= 0; back-- {
				if !math.IsNaN(atr[back]) {
					sum -= atr[back]
					count--
					break
				}
			}
		}
		if count >= consolidationLookback {
			atrAvg[i] = sum / float64(count)
		}
	}

	for i := 1; i < n; i++ {
		if math.IsNaN(atr[i]) || math.IsNaN(atrAvg[i]) || atr[i] == 0 {
			continue
		}

		// Consolidation check: current ATR was below its average (tight range)
		if i > 0 && !math.IsNaN(atr[i-1]) && !math.IsNaN(atrAvg[i-1]) && atr[i-1] >= atrAvg[i-1] {
			continue
		}

		// Bullish breakaway: gap up
		if bars[i].Open > bars[i-1].Close {
			gapSize := bars[i].Open - bars[i-1].Close
			if gapSize > atr[i] {
				z := makePDZone(PDBreakawayGap, Bullish, i, bars[i].Open, bars[i-1].Close)
				for j := i + 1; j < n; j++ {
					if bars[j].Low <= z.Top {
						z.Mitigated = true
						z.MitIndex = j
						break
					}
				}
				zones = append(zones, z)
			}
		}
		// Bearish breakaway: gap down
		if bars[i].Open < bars[i-1].Close {
			gapSize := bars[i-1].Close - bars[i].Open
			if gapSize > atr[i] {
				z := makePDZone(PDBreakawayGap, Bearish, i, bars[i-1].Close, bars[i].Open)
				for j := i + 1; j < n; j++ {
					if bars[j].High >= z.Bottom {
						z.Mitigated = true
						z.MitIndex = j
						break
					}
				}
				zones = append(zones, z)
			}
		}
	}
	return zones
}

// ═══════════════════════════════════════════════════════════════════════════
// Main Entry Point
// ═══════════════════════════════════════════════════════════════════════════

// DetectAllPDArrays runs all PD Array detectors on the given bars and returns
// all detected zones, sorted by bar index. This is the main entry point.
func DetectAllPDArrays(bars []data.OHLCV, params PDParams) []PDZone {
	if len(bars) < 3 {
		return nil
	}

	// Pre-compute shared indicators
	atr := ATR(bars, params.ATRPeriod)
	swingHighs := SwingHighs(bars, params.SwingPeriod)
	swingLows := SwingLows(bars, params.SwingPeriod)

	// 1. FVGs
	fvgs := detectFVGs(bars)

	// 2. Order Blocks
	obs := detectOrderBlocks(bars, atr, params.OBImpulseMult)

	// 3. Breaker Blocks (depends on OBs)
	breakers := detectBreakerBlocks(bars, obs)

	// 4. Mitigation Blocks (depends on OBs)
	mitigations := detectMitigationBlocks(obs)

	// 5. Volume Imbalances
	volumeImbs := detectVolumeImbalances(bars)

	// 6. Inverse FVGs (depends on FVGs)
	ifvgs := detectInverseFVGs(bars, fvgs)

	// 7. Implied FVGs
	impliedFVGs := detectImpliedFVGs(bars)

	// 8. BPRs (depends on FVGs)
	bprs := detectBPRs(fvgs)

	// 9. Vacuum Blocks
	vacuums := detectVacuumBlocks(bars, atr, params.VacuumMult)

	// 10. Rejection Blocks
	rejections := detectRejectionBlocks(bars, swingHighs, swingLows, params.WickRatio)

	// 11. Propulsion Blocks (depends on OBs + FVGs)
	propulsions := detectPropulsionBlocks(obs, fvgs)

	// 12. Breakaway Gaps
	breakaways := detectBreakawayGaps(bars, atr)

	// Merge all zones
	var all []PDZone
	all = append(all, fvgs...)
	all = append(all, obs...)
	all = append(all, breakers...)
	all = append(all, mitigations...)
	all = append(all, volumeImbs...)
	all = append(all, ifvgs...)
	all = append(all, impliedFVGs...)
	all = append(all, bprs...)
	all = append(all, vacuums...)
	all = append(all, rejections...)
	all = append(all, propulsions...)
	all = append(all, breakaways...)

	// Sort by bar index
	sort.Slice(all, func(i, j int) bool {
		if all[i].Index != all[j].Index {
			return all[i].Index < all[j].Index
		}
		return all[i].Type < all[j].Type
	})

	return all
}
