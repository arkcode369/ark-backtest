// Package pdarray provides detection and statistical tracking of ICT PD Array zones.
// It identifies 15 zone types across Bullish/Bearish directions, then measures
// whether each zone was subsequently "respected" (price bounced) or "breached"
// (price closed through the zone).
package pdarray

import (
	"fmt"
	"math"
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
	BarIndex  int  // bar where zone was *formed* (or activated)
	Respected bool // zone held — price bounced as expected
	Breached  bool // zone failed — price closed through it
	Tested    bool // zone was touched at least once after formation
}

// ── Stats ────────────────────────────────────────────────────────────────────

// Stats aggregates outcomes for one (Type, Direction) pair.
type Stats struct {
	Type      PDArrayType
	Direction Direction
	Total     int // zones detected
	Respected int
	Breached  int
	Untested  int
}

func (s Stats) TestedCount() int { return s.Respected + s.Breached }

func (s Stats) RespectedPct() float64 {
	if t := s.TestedCount(); t > 0 {
		return float64(s.Respected) / float64(t) * 100
	}
	return 0
}

func (s Stats) BreachedPct() float64 {
	if t := s.TestedCount(); t > 0 {
		return float64(s.Breached) / float64(t) * 100
	}
	return 0
}

// ── AnalyzeResult ────────────────────────────────────────────────────────────

// AnalyzeResult is the complete output of Analyze().
type AnalyzeResult struct {
	Symbol              string
	Interval            string
	Period              string
	Stats               []Stats
	Total               int
	TotalTested         int
	OverallRespectedPct float64
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

	// ── Tracking phase ───────────────────────────────────────────────────────
	trackZones(bars, zones)

	return aggregateStats(zones, symbol, interval, bars)
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

func detectMitigationBlocks(bars []data.OHLCV) []PDZone {
	var out []PDZone
	n := len(bars)

	for i := 1; i < n-1; i++ {
		// Swing low: lower than both neighbors
		if bars[i].Low < bars[i-1].Low && bars[i].Low < bars[i+1].Low {
			swingLow := bars[i].Low
			for k := i + 2; k < n; k++ {
				if bars[k].Close < swingLow {
					if isValidZone(bars[i].High, bars[i].Low) {
						out = append(out, zone(TypeMitigationBlock, Bullish, bars[i].High, bars[i].Low, i))
					}
					break
				}
			}
		}

		// Swing high: higher than both neighbors
		if bars[i].High > bars[i-1].High && bars[i].High > bars[i+1].High {
			swingHigh := bars[i].High
			for k := i + 2; k < n; k++ {
				if bars[k].Close > swingHigh {
					if isValidZone(bars[i].High, bars[i].Low) {
						out = append(out, zone(TypeMitigationBlock, Bearish, bars[i].High, bars[i].Low, i))
					}
					break
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

func detectVolumeImbalances(bars []data.OHLCV) []PDZone {
	var out []PDZone
	for i := 1; i < len(bars); i++ {
		if bars[i].Open > bars[i-1].Close {
			top, bot := bars[i].Open, bars[i-1].Close
			if isValidZone(top, bot) {
				out = append(out, zone(TypeVolumeImbalance, Bullish, top, bot, i))
			}
		} else if bars[i].Open < bars[i-1].Close {
			top, bot := bars[i-1].Close, bars[i].Open
			if isValidZone(top, bot) {
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

func detectImpliedFVGs(bars []data.OHLCV) []PDZone {
	var out []PDZone
	for i := 1; i < len(bars); i++ {
		prev := bars[i-1]
		cur := bars[i]

		// Bullish Implied FVG: open > prev close AND open < prev high
		if cur.Open > prev.Close && cur.Open < prev.High {
			top, bot := cur.Open, prev.Close
			if isValidZone(top, bot) {
				out = append(out, zone(TypeImpliedFVG, Bullish, top, bot, i))
			}
		}

		// Bearish Implied FVG: open < prev close AND open > prev low
		if cur.Open < prev.Close && cur.Open > prev.Low {
			top, bot := prev.Close, cur.Open
			if isValidZone(top, bot) {
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

// detectPremiumDiscount creates periodic Premium (bearish) and Discount (bullish)
// zones from the 20-bar swing range. A new set of zones is emitted every 20 bars
// whenever the swing range is valid and non-trivial.
func detectPremiumDiscount(bars []data.OHLCV, atr []float64) []PDZone {
	var out []PDZone
	const window = 20

	for i := window; i < len(bars); i += window {
		swingHigh := bars[i-window].High
		swingLow := bars[i-window].Low
		for j := i - window + 1; j < i; j++ {
			if bars[j].High > swingHigh {
				swingHigh = bars[j].High
			}
			if bars[j].Low < swingLow {
				swingLow = bars[j].Low
			}
		}
		rng := swingHigh - swingLow
		if math.IsNaN(rng) || rng <= 0 {
			continue
		}
		// Require the range to be meaningful relative to ATR
		if !math.IsNaN(atr[i]) && atr[i] > 0 && rng < atr[i]*0.5 {
			continue
		}
		equil := (swingHigh + swingLow) / 2

		// Discount zone (bullish): below equilibrium
		if isValidZone(equil, swingLow) {
			out = append(out, zone(TypePremDiscount, Bullish, equil, swingLow, i))
		}
		// Premium zone (bearish): above equilibrium
		if isValidZone(swingHigh, equil) {
			out = append(out, zone(TypePremDiscount, Bearish, swingHigh, equil, i))
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

// ── Zone Tracking ─────────────────────────────────────────────────────────────

// trackZones iterates the bars after each zone's formation bar and determines
// if the zone was respected (bounce) or breached (close through).
func trackZones(bars []data.OHLCV, zones []PDZone) {
	n := len(bars)
	for i := range zones {
		z := &zones[i]
		if !isValidZone(z.Top, z.Bottom) {
			continue
		}

		for k := z.BarIndex + 1; k < n; k++ {
			bar := bars[k]
			if z.Direction == Bullish {
				if bar.Low <= z.Top {
					z.Tested = true
					if bar.Close >= z.Bottom {
						z.Respected = true
					} else {
						z.Breached = true
					}
					break
				}
			} else {
				if bar.High >= z.Bottom {
					z.Tested = true
					if bar.Close <= z.Top {
						z.Respected = true
					} else {
						z.Breached = true
					}
					break
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
}

type statKey struct {
	t PDArrayType
	d Direction
}

func aggregateStats(zones []PDZone, symbol, interval string, bars []data.OHLCV) *AnalyzeResult {
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
		switch {
		case z.Respected:
			s.Respected++
		case z.Breached:
			s.Breached++
		default:
			s.Untested++
		}
	}

	var statsList []Stats
	for _, td := range orderedTypes {
		statsList = append(statsList, *sm[statKey{td.t, td.d}])
	}

	total := len(zones)
	totalTested, totalRespected := 0, 0
	for _, z := range zones {
		if z.Tested {
			totalTested++
			if z.Respected {
				totalRespected++
			}
		}
	}

	overallPct := 0.0
	if totalTested > 0 {
		overallPct = float64(totalRespected) / float64(totalTested) * 100
	}

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
		Stats:               statsList,
		Total:               total,
		TotalTested:         totalTested,
		OverallRespectedPct: overallPct,
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

	out := fmt.Sprintf("📊 *PD Array Success Rate*\n*%s | %s | %s*\nZones: %d detected | %d tested (%.1f%%)\n",
		r.Symbol, r.Interval, r.Period,
		r.Total, r.TotalTested, testedPct,
	)

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

	// ── Structural group ─────────────────────────────────────────────────────
	structuralTypes := []PDArrayType{TypeIFVG, TypePremDiscount, TypeBreakawayGap, TypeRDRB}
	out += "\n━━━ *STRUCTURAL ZONES* ━━━\n"
	out += formatGroupTable(sm, structuralTypes)

	// ── Overall ──────────────────────────────────────────────────────────────
	out += "\n━━━ *OVERALL* ━━━\n"
	overallBreached := 100.0 - r.OverallRespectedPct
	out += fmt.Sprintf("✅ Respected: %.1f%% | ❌ Breached: %.1f%%\n", r.OverallRespectedPct, overallBreached)

	best, worst := findBestWorst(r.Stats)
	if best.TestedCount() >= 3 {
		out += fmt.Sprintf("🏆 Best: %s %s (%.1f%%)\n", best.Direction, best.Type, best.RespectedPct())
	}
	if worst.TestedCount() >= 3 {
		out += fmt.Sprintf("⚠️  Weakest: %s %s (%.1f%%)\n", worst.Direction, worst.Type, worst.RespectedPct())
	}

	return out
}

// formatGroupTable renders a two-column (Bull | Bear) table for a set of types.
func formatGroupTable(sm map[statKey]Stats, types []PDArrayType) string {
	// Header
	out := fmt.Sprintf("%-12s  %-22s %-22s\n", "", "Bullish", "Bearish")
	for _, t := range types {
		bull := sm[statKey{t, Bullish}]
		bear := sm[statKey{t, Bearish}]
		out += fmt.Sprintf("%-12s  %-22s %-22s\n",
			string(t),
			formatCell(bull),
			formatCell(bear),
		)
	}
	return out
}

func formatCell(s Stats) string {
	if s.Total == 0 {
		return "— (0 detected)"
	}
	tc := s.TestedCount()
	if tc == 0 {
		return fmt.Sprintf("— (%d untested)", s.Total)
	}
	return fmt.Sprintf("✅%.1f%% (%d/%d)", s.RespectedPct(), tc, s.Total)
}

func findBestWorst(stats []Stats) (best, worst Stats) {
	initialized := false
	for _, s := range stats {
		if s.TestedCount() < 3 {
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
