// Package pdarray provides detection and statistical tracking of ICT PD Array zones.
// It identifies 25 zone types across Bullish/Bearish directions, then measures
// whether each zone was subsequently "respected" (price bounced) or "breached"
// (price closed through the zone).
package pdarray

import (
	"fmt"
	"math"
	"time"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── Types & Constants ────────────────────────────────────────────────────────

// PDArrayType identifies which kind of PD Array a zone belongs to.
type PDArrayType string

const (
	TypeFVG             PDArrayType = "FVG"
	TypeOrderBlock      PDArrayType = "OB"
	TypeBreakerBlock    PDArrayType = "Breaker"
	TypeMitigationBlock PDArrayType = "Mitigation"
	TypePropulsion      PDArrayType = "Propulsion"
	TypeVolumeImbalance PDArrayType = "VolImbalance"
	TypeIFVG            PDArrayType = "IFVG"
	TypeImpliedFVG      PDArrayType = "ImpliedFVG"
	TypeBPR             PDArrayType = "BPR"
	TypePremDiscount    PDArrayType = "PremDiscount"
	TypeVacuumBlock     PDArrayType = "VacuumBlock"
	TypeRejectionBlock  PDArrayType = "Rejection"
	TypeReclaimedOB     PDArrayType = "ReclaimedOB"
	TypeBreakawayGap    PDArrayType = "BreakawayGap"
	TypeRDRB            PDArrayType = "RDRB"

	// ── ICT Liquidity Types ──────────────────────────────────────────────────
	// BSL/SSL: Buy/Sell Side Liquidity pools sitting above swing highs / below swing lows
	TypeBSL PDArrayType = "BSL"
	TypeSSL PDArrayType = "SSL"

	// LiqSweep: stop-hunt spike that immediately reverses back inside range
	// LiqRun:   breakout that closes through the level and continues
	TypeLiqSweep PDArrayType = "LiqSweep"
	TypeLiqRun   PDArrayType = "LiqRun"

	// LiqVoid: large-body displacement candle leaving a liquidity void
	TypeLiqVoid PDArrayType = "LiqVoid"

	// IRL/ERL: Internal / External Range Liquidity
	TypeIRL PDArrayType = "IRL"
	TypeERL PDArrayType = "ERL"

	// OpenFloat: institutional liquidity pools at 20/40/60-bar highs & lows
	TypeOpenFloat PDArrayType = "OpenFloat"

	// NWOG/NDOG: New Week / New Day Opening Gap
	TypeNWOG PDArrayType = "NWOG"
	TypeNDOG PDArrayType = "NDOG"
)

// Direction is the bias of a PD Array zone.
type Direction string

const (
	Bullish Direction = "Bull"
	Bearish Direction = "Bear"
)

// PDZone is a single detected PD Array zone.
type PDZone struct {
	Type      PDArrayType
	Direction Direction
	Top       float64
	Bottom    float64
	Mid       float64
	BarIndex  int // bar where zone was *formed* (or activated)

	// Multiple-touch tracking:
	// A zone starts as "active". Each time price enters the zone we record
	// a touch. A touch is Respected when price closes back inside/above(bull)
	// or inside/below(bear) the zone without closing through the opposite side.
	// A touch is Breached when price closes beyond the zone's far boundary.
	// Once Breached the zone is invalidated — no further touches are counted.
	Touches   int  // number of times price entered the zone
	Respected int  // touches that ended with a bounce (respected)
	Breached  bool // true once price closed fully through the zone
	Tested    bool // zone was touched at least once
}

// ── Stats ────────────────────────────────────────────────────────────────────

// Stats aggregates outcomes for one (Type, Direction) pair.
type Stats struct {
	Type          PDArrayType
	Direction     Direction
	Total         int // zones detected
	ZonesRespected int // zones where at least one touch was respected and never breached
	ZonesBreached  int // zones that were ultimately breached
	ZonesUntested  int // zones never touched
	TotalTouches   int // sum of all touches across all zones
	RespectedTouches int // touches that ended as bounces
	BreachedTouches  int // touches that ended as breach (= number of breached zones, 1 per zone)
}

// TestedZones returns zones that were touched at least once.
func (s Stats) TestedZones() int { return s.ZonesRespected + s.ZonesBreached }

// RespectedPct is the % of tested zones that were ultimately respected (never breached).
func (s Stats) RespectedPct() float64 {
	if t := s.TestedZones(); t > 0 {
		return float64(s.ZonesRespected) / float64(t) * 100
	}
	return 0
}

// BreachedPct is the % of tested zones that were ultimately breached.
func (s Stats) BreachedPct() float64 {
	if t := s.TestedZones(); t > 0 {
		return float64(s.ZonesBreached) / float64(t) * 100
	}
	return 0
}

// TouchRespectedPct is the % of individual touches (across all zones) that bounced.
func (s Stats) TouchRespectedPct() float64 {
	if s.TotalTouches > 0 {
		return float64(s.RespectedTouches) / float64(s.TotalTouches) * 100
	}
	return 0
}

// ── AnalyzeResult ────────────────────────────────────────────────────────────

// AnalyzeResult is the complete output of Analyze().
type AnalyzeResult struct {
	Symbol              string
	Interval            string
	Period              string
	LookForward         int     // bars tracked per zone (timeframe-calibrated)
	Stats               []Stats
	Total               int
	TotalTested         int
	TotalTouches        int
	RespectedTouches    int
	OverallRespectedPct float64 // % of tested zones never breached
	OverallTouchPct     float64 // % of individual touches that bounced
}

// ── Public Entry Point ────────────────────────────────────────────────────────

// Analyze detects all 15 PD Array types in bars, tracks their outcomes, and
// returns aggregated statistics.
func Analyze(bars []data.OHLCV, symbol, interval string) *AnalyzeResult {
	if len(bars) < 20 {
		return &AnalyzeResult{Symbol: symbol, Interval: interval}
	}

	atr := indicators.ATR(bars, 14)

	// ── Detection phase ──────────────────────────────────────────────────────
	bullFVGs, bearFVGs := detectFVGs(bars)
	bullOBs, bearOBs := detectOrderBlocks(bars, atr)

	var zones []PDZone
	zones = append(zones, bullFVGs...)
	zones = append(zones, bearFVGs...)
	zones = append(zones, bullOBs...)
	zones = append(zones, bearOBs...)
	zones = append(zones, detectBreakerBlocks(bars, bullOBs, bearOBs)...)
	zones = append(zones, detectMitigationBlocks(bars)...)
	zones = append(zones, detectPropulsionBlocks(bars, bullOBs, bearOBs, bullFVGs, bearFVGs)...)
	zones = append(zones, detectVolumeImbalances(bars)...)
	zones = append(zones, detectIFVGs(bars, bullFVGs, bearFVGs)...)
	zones = append(zones, detectImpliedFVGs(bars)...)
	zones = append(zones, detectBPRs(bars, bullFVGs, bearFVGs)...)
	zones = append(zones, detectPremiumDiscount(bars, atr)...)
	zones = append(zones, detectVacuumBlocks(bars, atr)...)
	zones = append(zones, detectRejectionBlocks(bars)...)
	zones = append(zones, detectReclaimedOBs(bars, bullOBs, bearOBs)...)
	zones = append(zones, detectBreakawayGaps(bars, atr)...)
	zones = append(zones, detectRDRB(bars, atr)...)

	// ── ICT Liquidity detectors ──────────────────────────────────────────────
	zones = append(zones, detectBSLSSL(bars, atr)...)
	zones = append(zones, detectLiquiditySweepRun(bars, atr)...)
	zones = append(zones, detectLiquidityVoid(bars, atr)...)
	zones = append(zones, detectIRLERL(bars)...)
	zones = append(zones, detectOpenFloat(bars, atr)...)
	zones = append(zones, detectNWOGNDOG(bars, atr, interval)...)

	// ── Tracking phase ───────────────────────────────────────────────────────
	lookFwd := lookForwardBars(interval)
	trackZones(bars, zones, lookFwd)

	return aggregateStats(zones, symbol, interval, lookFwd, bars)
}

// ── FVG ──────────────────────────────────────────────────────────────────────

func detectFVGs(bars []data.OHLCV) (bull, bear []PDZone) {
	for i := 2; i < len(bars); i++ {
		// Bullish FVG: low[i] > high[i-2]
		if bars[i].Low > bars[i-2].High {
			top, bot := bars[i].Low, bars[i-2].High
			if isValidZone(top, bot) {
				bull = append(bull, zone(TypeFVG, Bullish, top, bot, i))
			}
		}
		// Bearish FVG: high[i] < low[i-2]
		if bars[i].High < bars[i-2].Low {
			top, bot := bars[i-2].Low, bars[i].High
			if isValidZone(top, bot) {
				bear = append(bear, zone(TypeFVG, Bearish, top, bot, i))
			}
		}
	}
	return
}

// ── Order Block ───────────────────────────────────────────────────────────────

func detectOrderBlocks(bars []data.OHLCV, atr []float64) (bull, bear []PDZone) {
	for i := 1; i < len(bars); i++ {
		if math.IsNaN(atr[i]) || atr[i] == 0 {
			continue
		}
		rng := bars[i].High - bars[i].Low
		if rng <= 0 {
			continue
		}
		impulsive := rng > atr[i]*1.5

		prev := bars[i-1]
		cur := bars[i]

		// Bullish OB: previous bar bearish, current impulsive bullish
		if prev.Close < prev.Open && cur.Close > cur.Open && impulsive {
			top := math.Max(prev.Open, prev.Close)
			bot := math.Min(prev.Open, prev.Close)
			if isValidZone(top, bot) {
				bull = append(bull, zone(TypeOrderBlock, Bullish, top, bot, i-1))
			}
		}

		// Bearish OB: previous bar bullish, current impulsive bearish
		if prev.Close > prev.Open && cur.Close < cur.Open && impulsive {
			top := math.Max(prev.Open, prev.Close)
			bot := math.Min(prev.Open, prev.Close)
			if isValidZone(top, bot) {
				bear = append(bear, zone(TypeOrderBlock, Bearish, top, bot, i-1))
			}
		}
	}
	return
}

// ── Breaker Block ─────────────────────────────────────────────────────────────

func detectBreakerBlocks(bars []data.OHLCV, bullOBs, bearOBs []PDZone) []PDZone {
	var out []PDZone
	n := len(bars)

	// Bullish Breaker: bearish OB subsequently swept by close > ob.Top
	for _, ob := range bearOBs {
		for k := ob.BarIndex + 1; k < n; k++ {
			if bars[k].Close > ob.Top {
				out = append(out, zone(TypeBreakerBlock, Bullish, ob.Top, ob.Bottom, k))
				break
			}
		}
	}

	// Bearish Breaker: bullish OB subsequently swept by close < ob.Bottom
	for _, ob := range bullOBs {
		for k := ob.BarIndex + 1; k < n; k++ {
			if bars[k].Close < ob.Bottom {
				out = append(out, zone(TypeBreakerBlock, Bearish, ob.Top, ob.Bottom, k))
				break
			}
		}
	}
	return out
}

// ── Mitigation Block ──────────────────────────────────────────────────────────

// detectMitigationBlocks uses a 3-bar confirmation window (not just 1-bar neighbor)
// and requires the swing to have a meaningful range (>= 0.5x ATR) to filter noise.
func detectMitigationBlocks(bars []data.OHLCV) []PDZone {
	var out []PDZone
	n := len(bars)
	atr := indicators.ATR(bars, 14)

	const swingWin = 3 // bars on each side for swing confirmation
	const maxFwd = 50  // max bars to look forward for the mitigation trigger

	for i := swingWin; i < n-swingWin; i++ {
		atrVal := atr[i]
		if math.IsNaN(atrVal) || atrVal == 0 {
			continue
		}

		// ── Swing Low ────────────────────────────────────────────────────────
		isSwingLow := true
		for j := i - swingWin; j <= i+swingWin; j++ {
			if j != i && bars[j].Low <= bars[i].Low {
				isSwingLow = false
				break
			}
		}
		if isSwingLow {
			// Zone must be at least 0.5 ATR tall to matter
			zoneRng := bars[i].High - bars[i].Low
			if zoneRng >= atrVal*0.5 {
				swingLow := bars[i].Low
				end := i + maxFwd
				if end > n {
					end = n
				}
				for k := i + swingWin + 1; k < end; k++ {
					if bars[k].Close < swingLow {
						if isValidZone(bars[i].High, bars[i].Low) {
							out = append(out, zone(TypeMitigationBlock, Bullish, bars[i].High, bars[i].Low, i))
						}
						break
					}
				}
			}
		}

		// ── Swing High ───────────────────────────────────────────────────────
		isSwingHigh := true
		for j := i - swingWin; j <= i+swingWin; j++ {
			if j != i && bars[j].High >= bars[i].High {
				isSwingHigh = false
				break
			}
		}
		if isSwingHigh {
			zoneRng := bars[i].High - bars[i].Low
			if zoneRng >= atrVal*0.5 {
				swingHigh := bars[i].High
				end := i + maxFwd
				if end > n {
					end = n
				}
				for k := i + swingWin + 1; k < end; k++ {
					if bars[k].Close > swingHigh {
						if isValidZone(bars[i].High, bars[i].Low) {
							out = append(out, zone(TypeMitigationBlock, Bearish, bars[i].High, bars[i].Low, i))
						}
						break
					}
				}
			}
		}
	}
	return out
}

// ── Propulsion Block ──────────────────────────────────────────────────────────

// detectPropulsionBlocks finds OB+FVG overlaps within ±3 bars of each other.
func detectPropulsionBlocks(bars []data.OHLCV, bullOBs, bearOBs, bullFVGs, bearFVGs []PDZone) []PDZone {
	var out []PDZone
	const maxGap = 3

	// Bullish: bull OB overlaps with bull FVG
	for _, ob := range bullOBs {
		for _, fvg := range bullFVGs {
			gap := abs(ob.BarIndex - fvg.BarIndex)
			if gap > maxGap {
				continue
			}
			top := math.Min(ob.Top, fvg.Top)
			bot := math.Max(ob.Bottom, fvg.Bottom)
			if isValidZone(top, bot) {
				barIdx := ob.BarIndex
				if fvg.BarIndex > barIdx {
					barIdx = fvg.BarIndex
				}
				out = append(out, zone(TypePropulsion, Bullish, top, bot, barIdx))
			}
		}
	}

	// Bearish: bear OB overlaps with bear FVG
	for _, ob := range bearOBs {
		for _, fvg := range bearFVGs {
			gap := abs(ob.BarIndex - fvg.BarIndex)
			if gap > maxGap {
				continue
			}
			top := math.Min(ob.Top, fvg.Top)
			bot := math.Max(ob.Bottom, fvg.Bottom)
			if isValidZone(top, bot) {
				barIdx := ob.BarIndex
				if fvg.BarIndex > barIdx {
					barIdx = fvg.BarIndex
				}
				out = append(out, zone(TypePropulsion, Bearish, top, bot, barIdx))
			}
		}
	}
	return out
}

// ── Volume Imbalance ──────────────────────────────────────────────────────────

// detectVolumeImbalances detects open-vs-prevClose gaps >= 0.1 * ATR.
func detectVolumeImbalances(bars []data.OHLCV) []PDZone {
	var out []PDZone
	atr := indicators.ATR(bars, 14)

	for i := 1; i < len(bars); i++ {
		atrVal := atr[i]
		if math.IsNaN(atrVal) || atrVal == 0 {
			continue
		}
		minGap := atrVal * 0.1

		if bars[i].Open > bars[i-1].Close {
			top, bot := bars[i].Open, bars[i-1].Close
			if isValidZone(top, bot) && (top-bot) >= minGap {
				out = append(out, zone(TypeVolumeImbalance, Bullish, top, bot, i))
			}
		} else if bars[i].Open < bars[i-1].Close {
			top, bot := bars[i-1].Close, bars[i].Open
			if isValidZone(top, bot) && (top-bot) >= minGap {
				out = append(out, zone(TypeVolumeImbalance, Bearish, top, bot, i))
			}
		}
	}
	return out
}

// ── IFVG (Inverse FVG) ────────────────────────────────────────────────────────

func detectIFVGs(bars []data.OHLCV, bullFVGs, bearFVGs []PDZone) []PDZone {
	var out []PDZone
	n := len(bars)

	// Bullish FVG fully filled → Bearish IFVG
	for _, fvg := range bullFVGs {
		for k := fvg.BarIndex + 1; k < n; k++ {
			if bars[k].Close < fvg.Bottom {
				out = append(out, zone(TypeIFVG, Bearish, fvg.Top, fvg.Bottom, k))
				break
			}
		}
	}

	// Bearish FVG fully filled → Bullish IFVG
	for _, fvg := range bearFVGs {
		for k := fvg.BarIndex + 1; k < n; k++ {
			if bars[k].Close > fvg.Top {
				out = append(out, zone(TypeIFVG, Bullish, fvg.Top, fvg.Bottom, k))
				break
			}
		}
	}
	return out
}

// ── Implied FVG ───────────────────────────────────────────────────────────────

// detectImpliedFVGs detects gaps implied by wick structure.
// Requires gap >= 0.1 * ATR to filter micro-gaps that are just tick noise.
func detectImpliedFVGs(bars []data.OHLCV) []PDZone {
	var out []PDZone
	atr := indicators.ATR(bars, 14)

	for i := 1; i < len(bars); i++ {
		prev := bars[i-1]
		cur := bars[i]

		atrVal := atr[i]
		if math.IsNaN(atrVal) || atrVal == 0 {
			continue
		}
		minGap := atrVal * 0.1

		// Bullish Implied FVG: open > prev close AND open < prev high
		if cur.Open > prev.Close && cur.Open < prev.High {
			top, bot := cur.Open, prev.Close
			if isValidZone(top, bot) && (top-bot) >= minGap {
				out = append(out, zone(TypeImpliedFVG, Bullish, top, bot, i))
			}
		}

		// Bearish Implied FVG: open < prev close AND open > prev low
		if cur.Open < prev.Close && cur.Open > prev.Low {
			top, bot := prev.Close, cur.Open
			if isValidZone(top, bot) && (top-bot) >= minGap {
				out = append(out, zone(TypeImpliedFVG, Bearish, top, bot, i))
			}
		}
	}
	return out
}

// ── BPR (Balance Price Range) ─────────────────────────────────────────────────

func detectBPRs(bars []data.OHLCV, bullFVGs, bearFVGs []PDZone) []PDZone {
	var out []PDZone
	const maxGap = 50

	for _, bull := range bullFVGs {
		for _, bear := range bearFVGs {
			if abs(bull.BarIndex-bear.BarIndex) > maxGap {
				continue
			}
			bprTop := math.Min(bull.Top, bear.Top)
			bprBot := math.Max(bull.Bottom, bear.Bottom)
			if !isValidZone(bprTop, bprBot) {
				continue
			}
			// Direction = whichever FVG is more recent
			dir := Bullish
			barIdx := bull.BarIndex
			if bear.BarIndex > bull.BarIndex {
				dir = Bearish
				barIdx = bear.BarIndex
			}
			out = append(out, zone(TypeBPR, dir, bprTop, bprBot, barIdx))
		}
	}
	return out
}

// ── Premium / Discount ────────────────────────────────────────────────────────

// detectPremiumDiscount uses real swing high/low detection to create Premium
// (bearish) and Discount (bullish) zones.
//
// Logic:
//  1. Detect confirmed swing highs and swing lows using a left+right window of
//     swingLook bars on each side.
//  2. Pair each swing high with the most recent preceding swing low (or vice versa)
//     to define a range.
//  3. Also detect consolidation: when ATR is contracting (current ATR < avg ATR * 0.6)
//     the range of that consolidation itself becomes the premium/discount zone.
//  4. Equilibrium = midpoint of the range. Below = Discount (bullish), above = Premium (bearish).
func detectPremiumDiscount(bars []data.OHLCV, atr []float64) []PDZone {
	var out []PDZone
	n := len(bars)
	const swingLook = 5 // bars on each side to confirm swing

	// ── Step 1: collect swing highs and lows ──────────────────────────────────
	type swingPoint struct {
		idx   int
		price float64
		kind  string // "high" or "low"
	}
	var swings []swingPoint

	for i := swingLook; i < n-swingLook; i++ {
		isHigh := true
		isLow := true
		for j := i - swingLook; j <= i+swingLook; j++ {
			if j == i {
				continue
			}
			if bars[j].High >= bars[i].High {
				isHigh = false
			}
			if bars[j].Low <= bars[i].Low {
				isLow = false
			}
		}
		if isHigh {
			swings = append(swings, swingPoint{i, bars[i].High, "high"})
		}
		if isLow {
			swings = append(swings, swingPoint{i, bars[i].Low, "low"})
		}
	}

	// ── Step 2: pair consecutive swing high + swing low to build ranges ────────
	// Walk through swings in order, keep last seen high and last seen low.
	lastHighIdx, lastLowIdx := -1, -1
	lastHighPrice, lastLowPrice := 0.0, 0.0

	for _, sw := range swings {
		if sw.kind == "high" {
			if lastLowIdx >= 0 {
				// We have a swing low before this swing high → valid range
				swHigh := sw.price
				swLow := lastLowPrice
				rng := swHigh - swLow
				if !math.IsNaN(rng) && rng > 0 {
					atrVal := atr[sw.idx]
					if math.IsNaN(atrVal) || atrVal == 0 || rng >= atrVal*1.0 {
						equil := (swHigh + swLow) / 2
						barIdx := sw.idx
						if isValidZone(equil, swLow) {
							out = append(out, zone(TypePremDiscount, Bullish, equil, swLow, barIdx))
						}
						if isValidZone(swHigh, equil) {
							out = append(out, zone(TypePremDiscount, Bearish, swHigh, equil, barIdx))
						}
					}
				}
			}
			lastHighIdx = sw.idx
			lastHighPrice = sw.price
		} else {
			if lastHighIdx >= 0 {
				// We have a swing high before this swing low → valid range
				swHigh := lastHighPrice
				swLow := sw.price
				rng := swHigh - swLow
				if !math.IsNaN(rng) && rng > 0 {
					atrVal := atr[sw.idx]
					if math.IsNaN(atrVal) || atrVal == 0 || rng >= atrVal*1.0 {
						equil := (swHigh + swLow) / 2
						barIdx := sw.idx
						if isValidZone(equil, swLow) {
							out = append(out, zone(TypePremDiscount, Bullish, equil, swLow, barIdx))
						}
						if isValidZone(swHigh, equil) {
							out = append(out, zone(TypePremDiscount, Bearish, swHigh, equil, barIdx))
						}
					}
				}
			}
			lastLowIdx = sw.idx
			lastLowPrice = sw.price
		}
	}
	_ = lastHighIdx
	_ = lastLowIdx

	// ── Step 3: consolidation ranges ─────────────────────────────────────────
	// When ATR contracts below 60% of its 20-bar average, price is consolidating.
	// The H-L range of that consolidation period = its own premium/discount zone.
	const consolWindow = 20
	for i := consolWindow; i < n; i++ {
		if math.IsNaN(atr[i]) || atr[i] == 0 {
			continue
		}
		// Compute avg ATR over consolWindow bars
		avgATR := 0.0
		for j := i - consolWindow; j < i; j++ {
			if !math.IsNaN(atr[j]) {
				avgATR += atr[j]
			}
		}
		avgATR /= float64(consolWindow)
		if avgATR == 0 {
			continue
		}

		// Consolidation: current ATR is significantly contracted
		if atr[i] > avgATR*0.6 {
			continue // not consolidating
		}

		// Find H-L of consolidation window
		swH := bars[i-consolWindow].High
		swL := bars[i-consolWindow].Low
		for j := i - consolWindow + 1; j <= i; j++ {
			if bars[j].High > swH {
				swH = bars[j].High
			}
			if bars[j].Low < swL {
				swL = bars[j].Low
			}
		}
		rng := swH - swL
		if rng <= 0 || math.IsNaN(rng) {
			continue
		}
		equil := (swH + swL) / 2
		if isValidZone(equil, swL) {
			out = append(out, zone(TypePremDiscount, Bullish, equil, swL, i))
		}
		if isValidZone(swH, equil) {
			out = append(out, zone(TypePremDiscount, Bearish, swH, equil, i))
		}
	}

	return out
}

// ── Vacuum Block ─────────────────────────────────────────────────────────────

func detectVacuumBlocks(bars []data.OHLCV, atr []float64) []PDZone {
	var out []PDZone
	for i := 1; i < len(bars); i++ {
		if math.IsNaN(atr[i]) || atr[i] == 0 {
			continue
		}
		gapSize := math.Abs(bars[i].Open - bars[i-1].Close)
		if gapSize <= atr[i]*1.0 {
			continue
		}

		if bars[i].Open > bars[i-1].Close {
			top, bot := bars[i].Open, bars[i-1].Close
			if isValidZone(top, bot) {
				out = append(out, zone(TypeVacuumBlock, Bullish, top, bot, i))
			}
		} else {
			top, bot := bars[i-1].Close, bars[i].Open
			if isValidZone(top, bot) {
				out = append(out, zone(TypeVacuumBlock, Bearish, top, bot, i))
			}
		}
	}
	return out
}

// ── Rejection Block ───────────────────────────────────────────────────────────

func detectRejectionBlocks(bars []data.OHLCV) []PDZone {
	var out []PDZone
	for i := 0; i < len(bars); i++ {
		body := math.Abs(bars[i].Close - bars[i].Open)
		if body == 0 {
			continue
		}
		bodyTop := math.Max(bars[i].Open, bars[i].Close)
		bodyBot := math.Min(bars[i].Open, bars[i].Close)

		lowerWick := bodyBot - bars[i].Low
		if lowerWick > body*2.0 {
			if isValidZone(bodyBot, bars[i].Low) {
				out = append(out, zone(TypeRejectionBlock, Bullish, bodyBot, bars[i].Low, i))
			}
		}

		upperWick := bars[i].High - bodyTop
		if upperWick > body*2.0 {
			if isValidZone(bars[i].High, bodyTop) {
				out = append(out, zone(TypeRejectionBlock, Bearish, bars[i].High, bodyTop, i))
			}
		}
	}
	return out
}

// ── Reclaimed OB ─────────────────────────────────────────────────────────────

// detectReclaimedOBs finds OBs that were first violated then reclaimed.
// Bullish Reclaimed: bull OB → price dips through it → price rallies back through top.
// Bearish Reclaimed: bear OB → price spikes through it → price drops back through bottom.
func detectReclaimedOBs(bars []data.OHLCV, bullOBs, bearOBs []PDZone) []PDZone {
	var out []PDZone
	n := len(bars)
	const maxLook = 50

	for _, ob := range bullOBs {
		start := ob.BarIndex + 1
		end := start + maxLook
		if end > n {
			end = n
		}
		violated := false
		for k := start; k < end; k++ {
			if !violated && bars[k].Close < ob.Bottom {
				violated = true
				continue
			}
			if violated && bars[k].Close > ob.Top {
				out = append(out, zone(TypeReclaimedOB, Bullish, ob.Top, ob.Bottom, k))
				break
			}
		}
	}

	for _, ob := range bearOBs {
		start := ob.BarIndex + 1
		end := start + maxLook
		if end > n {
			end = n
		}
		violated := false
		for k := start; k < end; k++ {
			if !violated && bars[k].Close > ob.Top {
				violated = true
				continue
			}
			if violated && bars[k].Close < ob.Bottom {
				out = append(out, zone(TypeReclaimedOB, Bearish, ob.Top, ob.Bottom, k))
				break
			}
		}
	}
	return out
}

// ── Breakaway Gap ─────────────────────────────────────────────────────────────

// detectBreakawayGaps finds gaps that emerge from consolidation periods.
func detectBreakawayGaps(bars []data.OHLCV, atr []float64) []PDZone {
	var out []PDZone
	const consolWindow = 10

	for i := consolWindow + 1; i < len(bars); i++ {
		if math.IsNaN(atr[i]) || atr[i] == 0 {
			continue
		}
		gapSize := math.Abs(bars[i].Open - bars[i-1].Close)
		if gapSize <= atr[i]*0.5 {
			continue
		}

		// Check preceding consolidation: range of last consolWindow bars < atr * 2
		swingHi := bars[i-consolWindow].High
		swingLo := bars[i-consolWindow].Low
		for j := i - consolWindow + 1; j < i; j++ {
			if bars[j].High > swingHi {
				swingHi = bars[j].High
			}
			if bars[j].Low < swingLo {
				swingLo = bars[j].Low
			}
		}
		if (swingHi - swingLo) >= atr[i]*2.0 {
			continue // not consolidation
		}

		if bars[i].Open > bars[i-1].Close {
			top, bot := bars[i].Open, bars[i-1].Close
			if isValidZone(top, bot) {
				out = append(out, zone(TypeBreakawayGap, Bullish, top, bot, i))
			}
		} else {
			top, bot := bars[i-1].Close, bars[i].Open
			if isValidZone(top, bot) {
				out = append(out, zone(TypeBreakawayGap, Bearish, top, bot, i))
			}
		}
	}
	return out
}

// ── RDRB ─────────────────────────────────────────────────────────────────────

// detectRDRB looks for the Redelivered-Rebalanced PD Array pattern.
// Bullish RDRB: impulse up → retrace with bearish FVG → impulse up beyond first high.
// Bearish RDRB: impulse down → retrace with bullish FVG → impulse down below first low.
func detectRDRB(bars []data.OHLCV, atr []float64) []PDZone {
	var out []PDZone
	n := len(bars)
	const maxRetrace = 10

	for i := 1; i < n-maxRetrace-1; i++ {
		if math.IsNaN(atr[i]) || atr[i] == 0 {
			continue
		}

		// ── Bullish RDRB: impulse up at bar i ────────────────────────────────
		bullBody := bars[i].Close - bars[i].Open
		if bullBody >= atr[i]*1.5 {
			phase1High := bars[i].High
			// Scan retrace phase: look for a bearish FVG during retrace
			fvgFound := false
			var fvgTop, fvgBot float64
			var fvgBar int
			for j := i + 1; j < i+maxRetrace && j+2 < n; j++ {
				// Bearish FVG during retrace: high[j] < low[j-2]
				if bars[j].High < bars[j-2].Low {
					ft := bars[j-2].Low
					fb := bars[j].High
					if isValidZone(ft, fb) {
						fvgTop, fvgBot, fvgBar = ft, fb, j
						fvgFound = true
					}
				}
				// Phase 3: new impulse up beyond phase1High
				if fvgFound && bars[j].Close > phase1High {
					out = append(out, zone(TypeRDRB, Bullish, fvgTop, fvgBot, fvgBar))
					break
				}
			}
		}

		// ── Bearish RDRB: impulse down at bar i ──────────────────────────────
		bearBody := bars[i].Open - bars[i].Close
		if bearBody >= atr[i]*1.5 {
			phase1Low := bars[i].Low
			fvgFound := false
			var fvgTop, fvgBot float64
			var fvgBar int
			for j := i + 1; j < i+maxRetrace && j+2 < n; j++ {
				// Bullish FVG during retrace: low[j] > high[j-2]
				if bars[j].Low > bars[j-2].High {
					ft := bars[j].Low
					fb := bars[j-2].High
					if isValidZone(ft, fb) {
						fvgTop, fvgBot, fvgBar = ft, fb, j
						fvgFound = true
					}
				}
				// Phase 3: new impulse down below phase1Low
				if fvgFound && bars[j].Close < phase1Low {
					out = append(out, zone(TypeRDRB, Bearish, fvgTop, fvgBot, fvgBar))
					break
				}
			}
		}
	}
	return out
}

// ── BSL / SSL (Buy Side & Sell Side Liquidity) ────────────────────────────────

// detectBSLSSL identifies liquidity pools sitting above swing highs (BSL) and
// below swing lows (SSL). BSL zones are Bearish (institution will sell into them);
// SSL zones are Bullish (institution will buy from them).
//
// Equal Highs/Lows: when 2+ bars share a high/low within ATR*0.1 tolerance the
// pool is considered stronger; this is encoded in Mid (pool count, not price midpoint).
func detectBSLSSL(bars []data.OHLCV, atr []float64) []PDZone {
	var out []PDZone
	n := len(bars)
	const swingLen = 10 // bars on each side to confirm swing
	const thinMargin = 0.001 // minimum zone height as fraction of price

	for i := swingLen; i < n-swingLen; i++ {
		atrVal := atr[i]
		if math.IsNaN(atrVal) || atrVal == 0 {
			continue
		}
		tolerance := atrVal * 0.1

		// ── Swing High → BSL pool above ──────────────────────────────────────
		isHigh := true
		for j := i - swingLen; j <= i+swingLen; j++ {
			if j != i && bars[j].High >= bars[i].High {
				isHigh = false
				break
			}
		}
		if isHigh {
			swHigh := bars[i].High
			zoneTop := swHigh + atrVal*0.1
			zoneBot := swHigh - atrVal*0.05
			if zoneBot <= 0 {
				zoneBot = swHigh * (1 - thinMargin)
			}
			if isValidZone(zoneTop, zoneBot) {
				// Count equal highs for pool strength
				poolCount := 1.0
				for k := i - swingLen; k <= i+swingLen; k++ {
					if k != i && math.Abs(bars[k].High-swHigh) <= tolerance {
						poolCount++
					}
				}
				z := zone(TypeBSL, Bearish, zoneTop, zoneBot, i)
				z.Mid = poolCount // encode pool strength
				out = append(out, z)
			}
		}

		// ── Swing Low → SSL pool below ────────────────────────────────────────
		isLow := true
		for j := i - swingLen; j <= i+swingLen; j++ {
			if j != i && bars[j].Low <= bars[i].Low {
				isLow = false
				break
			}
		}
		if isLow {
			swLow := bars[i].Low
			zoneTop := swLow + atrVal*0.05
			zoneBot := swLow - atrVal*0.1
			if zoneBot <= 0 {
				zoneBot = swLow * (1 - thinMargin)
			}
			if isValidZone(zoneTop, zoneBot) {
				poolCount := 1.0
				for k := i - swingLen; k <= i+swingLen; k++ {
					if k != i && math.Abs(bars[k].Low-swLow) <= tolerance {
						poolCount++
					}
				}
				z := zone(TypeSSL, Bullish, zoneTop, zoneBot, i)
				z.Mid = poolCount
				out = append(out, z)
			}
		}
	}
	return out
}

// ── Liquidity Sweep & Liquidity Run ───────────────────────────────────────────

// detectLiquiditySweepRun identifies:
//   - LiqSweep: spike through a swing level that closes back inside range
//     (stop-hunt / reversal signal)
//   - LiqRun: close that stays through the level (continuation / trend move)
//
// Direction convention:
//   - SSL Sweep (low sweeps swing low, close back above) → Bullish
//   - BSL Sweep (high sweeps swing high, close back below) → Bearish
//   - BSL Run (close above swing high) → Bullish
//   - SSL Run (close below swing low) → Bearish
func detectLiquiditySweepRun(bars []data.OHLCV, atr []float64) []PDZone {
	var out []PDZone
	n := len(bars)
	const swingLen = 10

	for i := swingLen; i < n-swingLen; i++ {
		atrVal := atr[i]
		if math.IsNaN(atrVal) || atrVal == 0 {
			continue
		}

		// Collect the most recent confirmed swing high before bar i
		swingHigh := math.NaN()
		for j := i - 1; j >= swingLen; j-- {
			isH := true
			for k := j - swingLen; k <= j+swingLen && k < n; k++ {
				if k != j && bars[k].High >= bars[j].High {
					isH = false
					break
				}
			}
			if isH {
				swingHigh = bars[j].High
				break
			}
		}

		// Collect the most recent confirmed swing low before bar i
		swingLow := math.NaN()
		for j := i - 1; j >= swingLen; j-- {
			isL := true
			for k := j - swingLen; k <= j+swingLen && k < n; k++ {
				if k != j && bars[k].Low <= bars[j].Low {
					isL = false
					break
				}
			}
			if isL {
				swingLow = bars[j].Low
				break
			}
		}

		bar := bars[i]

		// ── BSL Sweep: high above swing high, close back below it ────────────
		if !math.IsNaN(swingHigh) {
			if bar.High > swingHigh && bar.Close < swingHigh {
				// Thin zone: the wick area between swing high and bar high
				top := bar.High
				bot := swingHigh
				if isValidZone(top, bot) {
					out = append(out, zone(TypeLiqSweep, Bearish, top, bot, i))
				} else {
					// Ensure minimum zone height
					out = append(out, zone(TypeLiqSweep, Bearish, swingHigh+atrVal*0.05, swingHigh-atrVal*0.02, i))
				}
			}
			// BSL Run: close above swing high (continuation bullish)
			if bar.Close > swingHigh {
				top := bar.Close
				bot := swingHigh
				if !isValidZone(top, bot) {
					bot = swingHigh - atrVal*0.02
				}
				if isValidZone(top, bot) {
					out = append(out, zone(TypeLiqRun, Bullish, top, bot, i))
				}
			}
		}

		// ── SSL Sweep: low below swing low, close back above it ──────────────
		if !math.IsNaN(swingLow) {
			if bar.Low < swingLow && bar.Close > swingLow {
				// Thin zone: wick area below swing low
				top := swingLow
				bot := bar.Low
				if isValidZone(top, bot) {
					out = append(out, zone(TypeLiqSweep, Bullish, top, bot, i))
				} else {
					out = append(out, zone(TypeLiqSweep, Bullish, swingLow+atrVal*0.02, swingLow-atrVal*0.05, i))
				}
			}
			// SSL Run: close below swing low (continuation bearish)
			if bar.Close < swingLow {
				top := swingLow
				bot := bar.Close
				if !isValidZone(top, bot) {
					top = swingLow + atrVal*0.02
				}
				if isValidZone(top, bot) {
					out = append(out, zone(TypeLiqRun, Bearish, top, bot, i))
				}
			}
		}
	}
	return out
}

// ── Liquidity Void ────────────────────────────────────────────────────────────

// detectLiquidityVoid finds displacement candles where:
//   - candle range > ATR * 2.0  AND
//   - body > range * 0.7
//
// The void zone is the body of the candle (min(open,close) .. max(open,close)).
// Direction: Bullish = close > open, Bearish = open > close.
func detectLiquidityVoid(bars []data.OHLCV, atr []float64) []PDZone {
	var out []PDZone
	for i := 0; i < len(bars); i++ {
		atrVal := atr[i]
		if math.IsNaN(atrVal) || atrVal == 0 {
			continue
		}
		candle := bars[i].High - bars[i].Low
		if candle <= 0 {
			continue
		}
		if candle <= atrVal*2.0 {
			continue
		}
		body := math.Abs(bars[i].Close - bars[i].Open)
		if body <= candle*0.7 {
			continue
		}

		top := math.Max(bars[i].Open, bars[i].Close)
		bot := math.Min(bars[i].Open, bars[i].Close)
		if !isValidZone(top, bot) {
			continue
		}

		dir := Bullish
		if bars[i].Open > bars[i].Close {
			dir = Bearish
		}
		out = append(out, zone(TypeLiqVoid, dir, top, bot, i))
	}
	return out
}

// ── IRL / ERL (Internal / External Range Liquidity) ───────────────────────────

// detectIRLERL computes:
//   - IRL (Internal Range Liquidity): equilibrium zones inside the current
//     20-bar trading range (discount half = Bullish, premium half = Bearish)
//   - ERL (External Range Liquidity): the 40-bar swing high/low levels that
//     price is targeting outside the inner range. When price reaches an ERL
//     the zone is confirmed (Bullish = ERL high touched, Bearish = ERL low touched).
func detectIRLERL(bars []data.OHLCV) []PDZone {
	var out []PDZone
	n := len(bars)
	const swingLen = 20

	for i := swingLen * 2; i < n; i++ {
		// ── Inner range (IRL) ─────────────────────────────────────────────────
		innerHigh := bars[i-swingLen].High
		innerLow := bars[i-swingLen].Low
		for j := i - swingLen + 1; j <= i; j++ {
			if bars[j].High > innerHigh {
				innerHigh = bars[j].High
			}
			if bars[j].Low < innerLow {
				innerLow = bars[j].Low
			}
		}
		innerRng := innerHigh - innerLow
		if innerRng <= 0 {
			continue
		}
		equil := (innerHigh + innerLow) / 2

		// Only emit IRL zones when price is near equilibrium (within 10% of range)
		if math.Abs(bars[i].Close-equil) < innerRng*0.1 {
			// Discount half (below equil) = Bullish IRL
			if isValidZone(equil, innerLow) {
				out = append(out, zone(TypeIRL, Bullish, equil, innerLow, i))
			}
			// Premium half (above equil) = Bearish IRL
			if isValidZone(innerHigh, equil) {
				out = append(out, zone(TypeIRL, Bearish, innerHigh, equil, i))
			}
		}

		// ── Outer range (ERL) ─────────────────────────────────────────────────
		outerHigh := bars[i-swingLen*2].High
		outerLow := bars[i-swingLen*2].Low
		for j := i - swingLen*2 + 1; j <= i; j++ {
			if bars[j].High > outerHigh {
				outerHigh = bars[j].High
			}
			if bars[j].Low < outerLow {
				outerLow = bars[j].Low
			}
		}

		atr14 := indicators.ATR(bars, 14)
		atrVal := atr14[i]
		if math.IsNaN(atrVal) || atrVal == 0 {
			continue
		}

		// ERL high touched: price reaching outside the inner high toward outer high
		if bars[i].High >= outerHigh {
			top := outerHigh + atrVal*0.05
			bot := outerHigh - atrVal*0.05
			if isValidZone(top, bot) {
				out = append(out, zone(TypeERL, Bullish, top, bot, i))
			}
		}
		// ERL low touched: price reaching outside the inner low toward outer low
		if bars[i].Low <= outerLow {
			top := outerLow + atrVal*0.05
			bot := outerLow - atrVal*0.05
			if isValidZone(top, bot) {
				out = append(out, zone(TypeERL, Bearish, top, bot, i))
			}
		}
	}
	return out
}

// ── Open Float Liquidity Pools ────────────────────────────────────────────────

// detectOpenFloat detects institutional open-float liquidity pools at 20/40/60-bar
// rolling highs and lows.
//
// When price reaches a new N-bar high/low, a thin zone is emitted to track
// whether price respects or breaks through that pool level.
//
// Zone Mid is set to a lookback-significance multiplier:
//
//	20-bar → 1.0 (short-term float)
//	40-bar → 1.5 (intermediate float)
//	60-bar → 2.0 (longer-term float, most significant)
//
// Direction: Bullish = price made new high (bull continuation pool),
// Bearish = price made new low (bear continuation pool).
func detectOpenFloat(bars []data.OHLCV, atr []float64) []PDZone {
	var out []PDZone
	n := len(bars)
	lookbacks := []struct {
		period int
		sig    float64 // significance multiplier stored in Mid
	}{
		{20, 1.0},
		{40, 1.5},
		{60, 2.0},
	}

	for i := 60; i < n; i++ {
		atrVal := atr[i]
		if math.IsNaN(atrVal) || atrVal == 0 {
			continue
		}

		for _, lb := range lookbacks {
			p := lb.period
			if i < p {
				continue
			}

			// Rolling high/low of the previous p bars (not including bar i)
			prevHigh := bars[i-p].High
			prevLow := bars[i-p].Low
			for j := i - p + 1; j < i; j++ {
				if bars[j].High > prevHigh {
					prevHigh = bars[j].High
				}
				if bars[j].Low < prevLow {
					prevLow = bars[j].Low
				}
			}

			// Bullish: current bar makes a new N-bar high
			if bars[i].High >= prevHigh {
				top := bars[i].High + atrVal*0.1
				bot := bars[i].High - atrVal*0.1
				if isValidZone(top, bot) {
					z := zone(TypeOpenFloat, Bullish, top, bot, i)
					z.Mid = lb.sig
					out = append(out, z)
				}
			}

			// Bearish: current bar makes a new N-bar low
			if bars[i].Low <= prevLow {
				top := bars[i].Low + atrVal*0.1
				bot := bars[i].Low - atrVal*0.1
				if isValidZone(top, bot) {
					z := zone(TypeOpenFloat, Bearish, top, bot, i)
					z.Mid = lb.sig
					out = append(out, z)
				}
			}
		}
	}
	return out
}

// ── NWOG / NDOG (New Week / New Day Opening Gap) ──────────────────────────────

// detectNWOGNDOG detects New Week Opening Gap (NWOG) and New Day Opening Gap (NDOG).
//
// NWOG: gap between Friday's close and the next Monday's open.
//   - For daily bars: a Monday bar (weekday) where |open - prev_close| > ATR * 0.05.
//   - For intraday bars: uses the actual hour to find the Friday 17:00 close vs
//     Monday/Sunday 18:00 open gap window.
//
// NDOG: gap between the 17:00 (5pm) close and the 18:00 (6pm) open of the next session.
//   - For daily bars: any gap between close[i-1] and open[i] on a different calendar day.
//   - For intraday bars: uses actual bar timestamps.
//
// Gap direction:
//   - Up-gap (open > prev close) → Bullish zone (acts as support)
//   - Down-gap (open < prev close) → Bearish zone (acts as resistance)
//
// C.E. (Consequent Encroachment) = Mid = (top+bot)/2  ← already handled by zone().
func detectNWOGNDOG(bars []data.OHLCV, atr []float64, interval string) []PDZone {
	var out []PDZone
	n := len(bars)
	if n < 2 {
		return out
	}

	isDaily := interval == "1d" || interval == "d"
	isIntraday := !isDaily && interval != "1w" && interval != "w" && interval != "wk" &&
		interval != "1mo" && interval != "mo"

	for i := 1; i < n; i++ {
		atrVal := atr[i]
		if math.IsNaN(atrVal) || atrVal == 0 {
			continue
		}

		prevClose := bars[i-1].Close
		curOpen := bars[i].Open
		gapSize := curOpen - prevClose // positive = up-gap, negative = down-gap

		if isDaily {
			// ── Daily bars: NWOG on Monday, NDOG on any trading day ──────────
			prevTime := bars[i-1].Time
			curTime := bars[i].Time

			// NWOG: current bar is Monday (or the first bar after weekend)
			prevWeekday := prevTime.Weekday()
			curWeekday := curTime.Weekday()
			isWeekGap := (curWeekday == time.Monday) ||
				(prevWeekday == time.Friday && (curWeekday == time.Monday || curWeekday == time.Sunday))

			minGap := atrVal * 0.05
			if math.Abs(gapSize) >= minGap {
				if isWeekGap {
					top := math.Max(curOpen, prevClose)
					bot := math.Min(curOpen, prevClose)
					if isValidZone(top, bot) {
						dir := Bullish
						if gapSize < 0 {
							dir = Bearish
						}
						out = append(out, zone(TypeNWOG, dir, top, bot, i))
					}
				} else {
					// NDOG: any day-to-day gap
					top := math.Max(curOpen, prevClose)
					bot := math.Min(curOpen, prevClose)
					if isValidZone(top, bot) {
						dir := Bullish
						if gapSize < 0 {
							dir = Bearish
						}
						out = append(out, zone(TypeNDOG, dir, top, bot, i))
					}
				}
			}

		} else if isIntraday {
			// ── Intraday bars: use actual timestamps ──────────────────────────
			prevTime := bars[i-1].Time
			curTime := bars[i].Time

			prevDay := prevTime.Weekday()
			curDay := curTime.Weekday()
			prevHour := prevTime.Hour()
			curHour := curTime.Hour()

			// NWOG: Friday close (17:xx) → Sunday/Monday open (18:xx or first bar)
			isFridayClose := (prevDay == time.Friday) && (prevHour >= 17)
			isWeekOpen := (curDay == time.Sunday || curDay == time.Monday) && (curHour >= 17 || curHour <= 2)

			minGap := atrVal * 0.05
			if isFridayClose && isWeekOpen && math.Abs(gapSize) >= minGap {
				top := math.Max(curOpen, prevClose)
				bot := math.Min(curOpen, prevClose)
				if isValidZone(top, bot) {
					dir := Bullish
					if gapSize < 0 {
						dir = Bearish
					}
					out = append(out, zone(TypeNWOG, dir, top, bot, i))
				}
			}

			// NDOG: 17:xx close → 18:xx open (different calendar day)
			is5pmClose := prevHour == 17
			is6pmOpen := curHour == 18
			isDayChange := prevDay != curDay || (prevDay == curDay && prevHour > curHour)

			if is5pmClose && is6pmOpen && isDayChange && math.Abs(gapSize) >= minGap {
				top := math.Max(curOpen, prevClose)
				bot := math.Min(curOpen, prevClose)
				if isValidZone(top, bot) {
					dir := Bullish
					if gapSize < 0 {
						dir = Bearish
					}
					out = append(out, zone(TypeNDOG, dir, top, bot, i))
				}
			}
		}
	}
	return out
}

// ── Zone Tracking ─────────────────────────────────────────────────────────────

// maxTouches: max sentuhan per zona — setelah ini zona dianggap "habis".
const maxTouches = 3

// lookForwardBars returns how many bars forward a zone should be tracked.
//
// Window per timeframe (user-calibrated):
//
//	1m  → 15   bars  (15 menit)
//	2m  → 8          (≈ 15 menit)
//	5m  → 6          (30 menit)
//	15m → 4          (1 jam)
//	30m → 4          (2 jam)
//	1h  → 12         (12 jam)
//	2h  → 6          (12 jam)
//	4h  → 42         (1 minggu = 7 hari × 6 bar/hari)
//	1d  → 10         (2 minggu)
//	1w  → 8          (2 bulan ≈ 8 minggu)
//	1mo → 3          (1 kuartal)
func lookForwardBars(interval string) int {
	switch interval {
	case "1m":
		return 15
	case "2m":
		return 8
	case "5m":
		return 6
	case "15m":
		return 4
	case "30m":
		return 4
	case "1h", "60m":
		return 12
	case "2h":
		return 6
	case "4h":
		return 42
	case "1d", "d":
		return 10
	case "1w", "w", "wk":
		return 8
	case "1mo", "mo":
		return 3
	default:
		// Fallback: assume 1h-equivalent
		return 12
	}
}

// trackZones implements a multiple-touch model with expiry limits:
//
//	For each zone we scan up to lookForward bars after formation.
//	Each time price enters the zone we count a "touch" (max maxTouches).
//	A touch resolves when price closes:
//	  - back outside the zone on the entry side → Respected touch (+1 Respected)
//	  - through the far boundary               → Breached (zone dead, stop)
//	After maxTouches the zone is considered expired regardless of outcome.
//	Zones never touched within lookForward bars are marked Untested.
func trackZones(bars []data.OHLCV, zones []PDZone, lookForward int) {
	n := len(bars)
	for i := range zones {
		z := &zones[i]
		if !isValidZone(z.Top, z.Bottom) {
			continue
		}

		inTouch := false
		endBar := z.BarIndex + 1 + lookForward
		if endBar > n {
			endBar = n
		}

		for k := z.BarIndex + 1; k < endBar; k++ {
			// Stop if we've hit the max-touch cap
			if z.Touches >= maxTouches {
				break
			}

			bar := bars[k]

			if z.Direction == Bullish {
				if !inTouch && bar.Low <= z.Top {
					inTouch = true
					z.Tested = true
					z.Touches++
				}
				if inTouch {
					if bar.Close < z.Bottom {
						// Closed below zone → BREACHED, zone is dead
						z.Breached = true
						inTouch = false
						break
					}
					if bar.Close > z.Top {
						// Bounced back above zone top → RESPECTED touch
						z.Respected++
						inTouch = false
						// zone stays active for next touch (up to maxTouches)
					}
					// close still inside zone → remain in touch
				}

			} else { // Bearish
				if !inTouch && bar.High >= z.Bottom {
					inTouch = true
					z.Tested = true
					z.Touches++
				}
				if inTouch {
					if bar.Close > z.Top {
						// Closed above zone → BREACHED, zone is dead
						z.Breached = true
						inTouch = false
						break
					}
					if bar.Close < z.Bottom {
						// Bounced back below zone bottom → RESPECTED touch
						z.Respected++
						inTouch = false
					}
					// close still inside zone → remain in touch
				}
			}
		}
	}
}

// ── Aggregation ───────────────────────────────────────────────────────────────

// orderedTypes defines the canonical order of (type, direction) pairs for display.
var orderedTypes = []struct {
	t PDArrayType
	d Direction
}{
	// Imbalance group
	{TypeFVG, Bullish}, {TypeFVG, Bearish},
	{TypeImpliedFVG, Bullish}, {TypeImpliedFVG, Bearish},
	{TypeVolumeImbalance, Bullish}, {TypeVolumeImbalance, Bearish},
	{TypeVacuumBlock, Bullish}, {TypeVacuumBlock, Bearish},
	{TypeBPR, Bullish}, {TypeBPR, Bearish},
	// Order flow group
	{TypeOrderBlock, Bullish}, {TypeOrderBlock, Bearish},
	{TypePropulsion, Bullish}, {TypePropulsion, Bearish},
	{TypeBreakerBlock, Bullish}, {TypeBreakerBlock, Bearish},
	{TypeMitigationBlock, Bullish}, {TypeMitigationBlock, Bearish},
	{TypeRejectionBlock, Bullish}, {TypeRejectionBlock, Bearish},
	{TypeReclaimedOB, Bullish}, {TypeReclaimedOB, Bearish},
	// Structural group
	{TypeIFVG, Bullish}, {TypeIFVG, Bearish},
	{TypePremDiscount, Bullish}, {TypePremDiscount, Bearish},
	{TypeBreakawayGap, Bullish}, {TypeBreakawayGap, Bearish},
	{TypeRDRB, Bullish}, {TypeRDRB, Bearish},
	{TypeIRL, Bullish}, {TypeIRL, Bearish},
	{TypeERL, Bullish}, {TypeERL, Bearish},
	{TypeOpenFloat, Bullish}, {TypeOpenFloat, Bearish},
	{TypeNWOG, Bullish}, {TypeNWOG, Bearish},
	{TypeNDOG, Bullish}, {TypeNDOG, Bearish},
	// Liquidity group
	{TypeBSL, Bullish}, {TypeBSL, Bearish},
	{TypeSSL, Bullish}, {TypeSSL, Bearish},
	{TypeLiqSweep, Bullish}, {TypeLiqSweep, Bearish},
	{TypeLiqRun, Bullish}, {TypeLiqRun, Bearish},
	{TypeLiqVoid, Bullish}, {TypeLiqVoid, Bearish},
}

type statKey struct {
	t PDArrayType
	d Direction
}

func aggregateStats(zones []PDZone, symbol, interval string, lookFwd int, bars []data.OHLCV) *AnalyzeResult {
	sm := make(map[statKey]*Stats, len(orderedTypes))
	for _, td := range orderedTypes {
		k := statKey{td.t, td.d}
		sm[k] = &Stats{Type: td.t, Direction: td.d}
	}

	for _, z := range zones {
		k := statKey{z.Type, z.Direction}
		s, ok := sm[k]
		if !ok {
			continue
		}
		s.Total++
		s.TotalTouches += z.Touches
		s.RespectedTouches += z.Respected

		switch {
		case !z.Tested:
			s.ZonesUntested++
		case z.Breached:
			// Zone was ultimately breached (may have had respected touches before)
			s.ZonesBreached++
			s.BreachedTouches++
		default:
			// Tested but never breached → zone respected overall
			s.ZonesRespected++
		}
	}

	var statsList []Stats
	for _, td := range orderedTypes {
		statsList = append(statsList, *sm[statKey{td.t, td.d}])
	}

	total := len(zones)
	totalTested, totalZonesRespected, totalTouches, totalRespectedTouches := 0, 0, 0, 0
	for _, z := range zones {
		totalTouches += z.Touches
		totalRespectedTouches += z.Respected
		if z.Tested {
			totalTested++
			if !z.Breached {
				totalZonesRespected++
			}
		}
	}

	// Overall: % of tested zones never breached
	overallPct := 0.0
	if totalTested > 0 {
		overallPct = float64(totalZonesRespected) / float64(totalTested) * 100
	}
	// Overall touch-level respect rate
	overallTouchPct := 0.0
	if totalTouches > 0 {
		overallTouchPct = float64(totalRespectedTouches) / float64(totalTouches) * 100
	}
	_ = overallTouchPct // used in formatting

	period := ""
	if len(bars) >= 2 {
		period = fmt.Sprintf("%s → %s",
			bars[0].Time.Format("2006-01-02"),
			bars[len(bars)-1].Time.Format("2006-01-02"),
		)
	}

	return &AnalyzeResult{
		Symbol:              symbol,
		Interval:            interval,
		Period:              period,
		LookForward:         lookFwd,
		Stats:               statsList,
		Total:               total,
		TotalTested:         totalTested,
		TotalTouches:        totalTouches,
		RespectedTouches:    totalRespectedTouches,
		OverallRespectedPct: overallPct,
		OverallTouchPct:     overallTouchPct,
	}
}

// ── Formatting ────────────────────────────────────────────────────────────────

// FormatResult renders the AnalyzeResult as a Telegram-friendly Markdown string.
func FormatResult(r *AnalyzeResult) string {
	if r == nil {
		return "❌ No result."
	}
	if r.Total == 0 {
		return fmt.Sprintf("📊 *PD Array Success Rate*\n*%s | %s*\n\n⚠️ No zones detected. Try a longer period.", r.Symbol, r.Interval)
	}

	testedPct := 0.0
	if r.Total > 0 {
		testedPct = float64(r.TotalTested) / float64(r.Total) * 100
	}

	out := fmt.Sprintf("📊 *PD Array Success Rate*\n*%s | %s | %s*\n", r.Symbol, r.Interval, r.Period)
	out += fmt.Sprintf("Zones: %d detected | %d tested (%.1f%%) | %d total touches\n",
		r.Total, r.TotalTested, testedPct, r.TotalTouches)
	out += fmt.Sprintf("_Tracking: %d bars forward | max %d touches per zone_\n", r.LookForward, maxTouches)
	out += "_Zone% = zona yg tidak pernah ditembus | Touch% = tiap sentuhan_\n"

	// Build quick lookup
	sm := make(map[statKey]Stats)
	for _, s := range r.Stats {
		sm[statKey{s.Type, s.Direction}] = s
	}

	// ── Imbalance group ──────────────────────────────────────────────────────
	imbalanceTypes := []PDArrayType{TypeFVG, TypeImpliedFVG, TypeVolumeImbalance, TypeVacuumBlock, TypeBPR}
	out += "\n━━━ *IMBALANCE ZONES* ━━━\n"
	out += formatGroupTable(sm, imbalanceTypes)

	// ── Order Flow group ─────────────────────────────────────────────────────
	orderFlowTypes := []PDArrayType{TypeOrderBlock, TypePropulsion, TypeBreakerBlock, TypeMitigationBlock, TypeRejectionBlock, TypeReclaimedOB}
	out += "\n━━━ *ORDER FLOW ZONES* ━━━\n"
	out += formatGroupTable(sm, orderFlowTypes)

	// ── Liquidity group ──────────────────────────────────────────────────────
	liquidityTypes := []PDArrayType{TypeBSL, TypeSSL, TypeLiqSweep, TypeLiqRun, TypeLiqVoid}
	out += "\n━━━ *LIQUIDITY ZONES* ━━━\n"
	out += formatGroupTable(sm, liquidityTypes)

	// ── Structural group ─────────────────────────────────────────────────────
	structuralTypes := []PDArrayType{TypeIFVG, TypePremDiscount, TypeBreakawayGap, TypeRDRB, TypeIRL, TypeERL, TypeOpenFloat, TypeNWOG, TypeNDOG}
	out += "\n━━━ *STRUCTURAL ZONES* ━━━\n"
	out += formatGroupTable(sm, structuralTypes)

	// ── Overall ──────────────────────────────────────────────────────────────
	out += "\n━━━ *OVERALL* ━━━\n"
	out += fmt.Sprintf("Zone: ✅%.1f%% respected | ❌%.1f%% breached\n",
		r.OverallRespectedPct, 100.0-r.OverallRespectedPct)
	out += fmt.Sprintf("Touch: ✅%.1f%% bounce | ❌%.1f%% thru (%d/%d touches)\n",
		r.OverallTouchPct, 100.0-r.OverallTouchPct,
		r.RespectedTouches, r.TotalTouches)

	best, worst := findBestWorst(r.Stats)
	if best.TestedZones() >= 3 {
		out += fmt.Sprintf("🏆 Best Zone: %s %s (%.1f%% | touch %.1f%%)\n",
			best.Direction, best.Type, best.RespectedPct(), best.TouchRespectedPct())
	}
	if worst.TestedZones() >= 3 {
		out += fmt.Sprintf("⚠️  Weakest: %s %s (%.1f%% | touch %.1f%%)\n",
			worst.Direction, worst.Type, worst.RespectedPct(), worst.TouchRespectedPct())
	}

	return out
}

// formatGroupTable renders a two-column (Bull | Bear) table for a set of types.
// Each cell shows: zone% (zones respected/tested) and touch% (touches bounced/total)
func formatGroupTable(sm map[statKey]Stats, types []PDArrayType) string {
	out := fmt.Sprintf("%-12s  %-26s %-26s\n", "", "Bullish", "Bearish")
	for _, t := range types {
		bull := sm[statKey{t, Bullish}]
		bear := sm[statKey{t, Bearish}]
		out += fmt.Sprintf("%-12s  %-26s %-26s\n",
			string(t),
			formatCell(bull),
			formatCell(bear),
		)
	}
	return out
}

// formatCell shows zone-level AND touch-level stats:
// ✅Z:67% T:72% (5z/7 | 9t/13)
// Z = zone respected%, T = touch respected%
// 5z/7 = 5 zones respected out of 7 tested
// 9t/13 = 9 bounced touches out of 13 total touches
func formatCell(s Stats) string {
	if s.Total == 0 {
		return "— (0 detected)"
	}
	tz := s.TestedZones()
	if tz == 0 {
		return fmt.Sprintf("— (%d untested)", s.Total)
	}
	return fmt.Sprintf("Z:%.0f%% T:%.0f%% (%dz/%d|%dt/%d)",
		s.RespectedPct(), s.TouchRespectedPct(),
		s.ZonesRespected, tz,
		s.RespectedTouches, s.TotalTouches,
	)
}

func findBestWorst(stats []Stats) (best, worst Stats) {
	initialized := false
	for _, s := range stats {
		if s.TestedZones() < 3 {
			continue
		}
		if !initialized {
			best, worst = s, s
			initialized = true
			continue
		}
		if s.RespectedPct() > best.RespectedPct() {
			best = s
		}
		if s.RespectedPct() < worst.RespectedPct() {
			worst = s
		}
	}
	return
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func zone(t PDArrayType, d Direction, top, bot float64, barIdx int) PDZone {
	return PDZone{
		Type:     t,
		Direction: d,
		Top:      top,
		Bottom:   bot,
		Mid:      (top + bot) / 2,
		BarIndex: barIdx,
	}
}

func isValidZone(top, bot float64) bool {
	return !math.IsNaN(top) && !math.IsNaN(bot) && top > bot
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
