// Package ictcontext provides a comprehensive ICT (Inner Circle Trader) market
// context analyzer. Given a series of OHLCV bars it detects:
//   - AMD Phase (Accumulation / Manipulation / Distribution)
//   - Market structure: Bias, MSS, CHoCH, BOS
//   - Liquidity: nearest BSL/SSL, last sweep direction
//   - BISI / SIBI imbalance context
//   - Turtle Soup setup signal
//   - Three Drives harmonic pattern signal
//   - CISD (Change In State of Delivery) signal
//   - Premium / Discount zone
//   - Overall setup quality score (0–10) and a plain-language recommendation
package ictcontext

import (
	"fmt"
	"math"
	"strings"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── Enumerations ─────────────────────────────────────────────────────────────

// AMDPhase represents the current session phase.
type AMDPhase string

const (
	PhaseAccumulation  AMDPhase = "Accumulation"
	PhaseManipulation  AMDPhase = "Manipulation"
	PhaseDistribution  AMDPhase = "Distribution"
	PhaseUnknown       AMDPhase = "Unknown"
)

// Bias represents the current structural bias.
type Bias string

const (
	BiasBullish Bias = "Bullish"
	BiasBearish Bias = "Bearish"
	BiasNeutral Bias = "Neutral"
)

// SweepDirection describes the direction of the most recent liquidity sweep.
type SweepDirection string

const (
	SweepBSL  SweepDirection = "BSL swept (bearish trap)"
	SweepSSL  SweepDirection = "SSL swept (bullish trap)"
	SweepNone SweepDirection = "None"
)

// ── Core structs ─────────────────────────────────────────────────────────────

// MarketStructure holds structural analysis data.
type MarketStructure struct {
	Bias    Bias
	HasMSS  bool    // Market Structure Shift detected
	HasCHoCH bool   // Change of Character detected
	HasBOS  bool    // Break of Structure detected
	LastSwingHigh float64
	LastSwingLow  float64
}

// Liquidity holds liquidity-pool analysis.
type Liquidity struct {
	NearestBSL float64        // nearest buy-side liquidity level
	NearestSSL float64        // nearest sell-side liquidity level
	LastSweep  SweepDirection // direction of the most recent sweep
}

// ICTContext is the complete result of an ICT context analysis.
type ICTContext struct {
	Symbol   string
	Interval string
	BarCount int

	Phase          AMDPhase
	Structure      MarketStructure
	Liq            Liquidity
	IsBISI         bool    // Bullish Imbalance, Sell-side Inefficiency present
	IsSIBI         bool    // Sell-side Imbalance, Buy-side Inefficiency present
	TurtleSoupLong bool    // Turtle Soup long setup (SSL sweep + bullish reclaim)
	TurtleSoupShort bool   // Turtle Soup short setup (BSL sweep + bearish reclaim)
	ThreeDrivesUp   bool   // Three-Drives bullish harmonic
	ThreeDrivesDown bool   // Three-Drives bearish harmonic
	CISDLong  bool         // Change In State of Delivery — bullish
	CISDShort bool         // Change In State of Delivery — bearish

	CurrentPrice   float64
	PremiumZone    float64 // 61.8% level of last swing range
	DiscountZone   float64 // 38.2% level of last swing range
	IsInPremium    bool
	IsInDiscount   bool

	SetupQuality  float64 // 0–10 score
	Recommendation string
}

// ── Public API ────────────────────────────────────────────────────────────────

// Analyze runs the full ICT context analysis on the provided bars and returns
// an ICTContext. At least 50 bars are recommended for reliable results.
func Analyze(bars []data.OHLCV, symbol, interval string) *ICTContext {
	ctx := &ICTContext{
		Symbol:   symbol,
		Interval: interval,
		BarCount: len(bars),
	}
	if len(bars) < 10 {
		ctx.Phase = PhaseUnknown
		ctx.Recommendation = "Insufficient data (need at least 10 bars)."
		return ctx
	}

	ctx.CurrentPrice = bars[len(bars)-1].Close

	detectStructure(ctx, bars)
	detectAMDPhase(ctx, bars)
	detectLiquidity(ctx, bars)
	detectImbalances(ctx, bars)
	detectTurtleSoup(ctx, bars)
	detectThreeDrives(ctx, bars)
	detectCISD(ctx, bars)
	detectPremiumDiscount(ctx, bars)
	scoreAndRecommend(ctx)

	return ctx
}

// Format renders an ICTContext into a human-readable Telegram-Markdown string.
func Format(ctx *ICTContext) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("🧠 *ICT Context: %s %s*\n", ctx.Symbol, ctx.Interval))
	sb.WriteString(fmt.Sprintf("Price: `%.5g`   Bars: %d\n\n", ctx.CurrentPrice, ctx.BarCount))

	// AMD Phase
	phaseEmoji := map[AMDPhase]string{
		PhaseAccumulation: "🔵",
		PhaseManipulation: "🟡",
		PhaseDistribution: "🔴",
		PhaseUnknown:      "⚪",
	}
	sb.WriteString(fmt.Sprintf("*AMD Phase:* %s %s\n\n", phaseEmoji[ctx.Phase], ctx.Phase))

	// Market Structure
	sb.WriteString("*Market Structure*\n")
	biasEmoji := map[Bias]string{BiasBullish: "🟢", BiasBearish: "🔴", BiasNeutral: "⚪"}
	sb.WriteString(fmt.Sprintf("  Bias: %s %s\n", biasEmoji[ctx.Structure.Bias], ctx.Structure.Bias))
	if ctx.Structure.LastSwingHigh > 0 {
		sb.WriteString(fmt.Sprintf("  Swing High: `%.5g`\n", ctx.Structure.LastSwingHigh))
	}
	if ctx.Structure.LastSwingLow > 0 {
		sb.WriteString(fmt.Sprintf("  Swing Low:  `%.5g`\n", ctx.Structure.LastSwingLow))
	}
	signals := []string{}
	if ctx.Structure.HasBOS {
		signals = append(signals, "BOS ✅")
	}
	if ctx.Structure.HasCHoCH {
		signals = append(signals, "CHoCH ✅")
	}
	if ctx.Structure.HasMSS {
		signals = append(signals, "MSS ✅")
	}
	if len(signals) > 0 {
		sb.WriteString(fmt.Sprintf("  Signals: %s\n", strings.Join(signals, ", ")))
	}
	sb.WriteString("\n")

	// Liquidity
	sb.WriteString("*Liquidity*\n")
	if ctx.Liq.NearestBSL > 0 {
		sb.WriteString(fmt.Sprintf("  Nearest BSL: `%.5g`\n", ctx.Liq.NearestBSL))
	}
	if ctx.Liq.NearestSSL > 0 {
		sb.WriteString(fmt.Sprintf("  Nearest SSL: `%.5g`\n", ctx.Liq.NearestSSL))
	}
	if ctx.Liq.LastSweep != SweepNone {
		sb.WriteString(fmt.Sprintf("  Last Sweep: %s\n", ctx.Liq.LastSweep))
	}
	sb.WriteString("\n")

	// Premium / Discount
	zone := "Mid-Range ⚖️"
	if ctx.IsInPremium {
		zone = "Premium 🔴 (watch for shorts)"
	} else if ctx.IsInDiscount {
		zone = "Discount 🟢 (watch for longs)"
	}
	sb.WriteString(fmt.Sprintf("*Price Zone:* %s\n", zone))
	if ctx.PremiumZone > 0 {
		sb.WriteString(fmt.Sprintf("  Premium ≥ `%.5g` | Discount ≤ `%.5g`\n", ctx.PremiumZone, ctx.DiscountZone))
	}
	sb.WriteString("\n")

	// Setups
	sb.WriteString("*Active Setups*\n")
	hasSetup := false
	if ctx.IsBISI {
		sb.WriteString("  🟢 BISI — bullish imbalance present\n")
		hasSetup = true
	}
	if ctx.IsSIBI {
		sb.WriteString("  🔴 SIBI — bearish imbalance present\n")
		hasSetup = true
	}
	if ctx.TurtleSoupLong {
		sb.WriteString("  🐢 Turtle Soup LONG — SSL swept, bullish reclaim\n")
		hasSetup = true
	}
	if ctx.TurtleSoupShort {
		sb.WriteString("  🐢 Turtle Soup SHORT — BSL swept, bearish reclaim\n")
		hasSetup = true
	}
	if ctx.ThreeDrivesUp {
		sb.WriteString("  📐 Three Drives UP — bullish harmonic pattern\n")
		hasSetup = true
	}
	if ctx.ThreeDrivesDown {
		sb.WriteString("  📐 Three Drives DOWN — bearish harmonic pattern\n")
		hasSetup = true
	}
	if ctx.CISDLong {
		sb.WriteString("  🔄 CISD LONG — delivery state shifted bullish\n")
		hasSetup = true
	}
	if ctx.CISDShort {
		sb.WriteString("  🔄 CISD SHORT — delivery state shifted bearish\n")
		hasSetup = true
	}
	if !hasSetup {
		sb.WriteString("  _No high-probability setup active_\n")
	}
	sb.WriteString("\n")

	// Quality score
	bar := qualityBar(ctx.SetupQuality)
	sb.WriteString(fmt.Sprintf("*Setup Quality:* %.1f / 10 %s\n\n", ctx.SetupQuality, bar))

	// Recommendation
	sb.WriteString(fmt.Sprintf("*Recommendation:* _%s_", ctx.Recommendation))

	return sb.String()
}

// ── Internal detectors ────────────────────────────────────────────────────────

// detectStructure finds the last swing high/low, determines bias, and detects
// BOS, CHoCH, and MSS from the last ~50 bars.
func detectStructure(ctx *ICTContext, bars []data.OHLCV) {
	lookback := min(len(bars), 100)
	slice := bars[len(bars)-lookback:]

	swingPeriod := 5
	highs := indicators.SwingHighs(slice, swingPeriod)
	lows := indicators.SwingLows(slice, swingPeriod)

	// Collect all confirmed swing highs/lows in order
	type swing struct {
		idx   int
		price float64
		isHigh bool
	}
	var swings []swing
	for i, h := range highs {
		if !math.IsNaN(h) {
			swings = append(swings, swing{i, h, true})
		}
	}
	for i, l := range lows {
		if !math.IsNaN(l) {
			swings = append(swings, swing{i, l, false})
		}
	}
	// Sort by bar index
	for i := 1; i < len(swings); i++ {
		for j := i; j > 0 && swings[j].idx < swings[j-1].idx; j-- {
			swings[j], swings[j-1] = swings[j-1], swings[j]
		}
	}

	if len(swings) < 2 {
		ctx.Structure.Bias = BiasNeutral
		return
	}

	// Find last confirmed swing high and low
	var lastHigh, lastLow float64
	var lastHighIdx, lastLowIdx int
	for _, s := range swings {
		if s.isHigh {
			lastHigh = s.price
			lastHighIdx = s.idx
		} else {
			lastLow = s.price
			lastLowIdx = s.idx
		}
	}

	ctx.Structure.LastSwingHigh = lastHigh
	ctx.Structure.LastSwingLow = lastLow

	// Determine bias: if last swing high was formed more recently → bearish,
	// last swing low more recently → bullish.
	if lastHighIdx > lastLowIdx {
		ctx.Structure.Bias = BiasBearish
	} else {
		ctx.Structure.Bias = BiasBullish
	}

	// BOS: current close broke beyond last swing high or low
	lastClose := slice[len(slice)-1].Close
	if lastClose > lastHigh {
		ctx.Structure.HasBOS = true
		ctx.Structure.Bias = BiasBullish
	} else if lastClose < lastLow {
		ctx.Structure.HasBOS = true
		ctx.Structure.Bias = BiasBearish
	}

	// CHoCH: look for a swing sequence that breaks prior structure in opposite direction
	// Simplified: if we have at least 4 swings, check if the most recent HH->HL or LH->LL broke
	if len(swings) >= 4 {
		last := swings[len(swings)-1]
		prev := swings[len(swings)-2]
		prev2 := swings[len(swings)-3]

		// bearish CHoCH: HL formed after HH, then close below the HL origin low
		if prev2.isHigh && !prev.isHigh && last.isHigh && last.price < prev2.price {
			ctx.Structure.HasCHoCH = true
		}
		// bullish CHoCH: LH formed after LL, then close above the LH origin high
		if !prev2.isHigh && prev.isHigh && !last.isHigh && last.price > prev2.price {
			ctx.Structure.HasCHoCH = true
		}
	}

	// MSS: CHoCH + BOS together = full Market Structure Shift
	if ctx.Structure.HasCHoCH && ctx.Structure.HasBOS {
		ctx.Structure.HasMSS = true
	}
}

// detectAMDPhase detects whether the recent bars look like Accumulation,
// Manipulation, or Distribution by measuring range compression and directional
// momentum in the most recent ~20 bars.
func detectAMDPhase(ctx *ICTContext, bars []data.OHLCV) {
	lookback := min(len(bars), 20)
	slice := bars[len(bars)-lookback:]

	// Calculate average range (High-Low) to measure compression/expansion
	var totalRange, firstHalfRange, secondHalfRange float64
	half := lookback / 2
	for i, b := range slice {
		r := b.High - b.Low
		totalRange += r
		if i < half {
			firstHalfRange += r
		} else {
			secondHalfRange += r
		}
	}
	_ = totalRange

	// Accumulation: compressed range in the first half
	// Manipulation: spike / expansion in the second half
	// Distribution: expansion + reversal
	firstAvg := firstHalfRange / float64(half)
	secondAvg := secondHalfRange / float64(lookback-half)

	lastBar := slice[len(slice)-1]
	prevBar := slice[len(slice)-2]

	expansionRatio := secondAvg / (firstAvg + 1e-10)
	bullishMomentum := lastBar.Close > prevBar.Close

	switch {
	case expansionRatio < 1.2:
		// Range is still compressed — accumulating
		ctx.Phase = PhaseAccumulation
	case expansionRatio >= 1.2 && expansionRatio < 2.0:
		// Starting to expand — likely manipulation (stop hunt / fakeout)
		ctx.Phase = PhaseManipulation
	case expansionRatio >= 2.0:
		// Strong expansion — distribution / trend
		if bullishMomentum {
			ctx.Phase = PhaseDistribution
		} else {
			ctx.Phase = PhaseManipulation
		}
	default:
		ctx.Phase = PhaseUnknown
	}
}

// detectLiquidity identifies the nearest BSL/SSL levels and most recent sweep.
func detectLiquidity(ctx *ICTContext, bars []data.OHLCV) {
	lookback := min(len(bars), 60)
	slice := bars[len(bars)-lookback:]
	lastClose := slice[len(slice)-1].Close

	// Collect swing highs (BSL) and swing lows (SSL) from the lookback window
	swingPeriod := 5
	highs := indicators.SwingHighs(slice, swingPeriod)
	lows := indicators.SwingLows(slice, swingPeriod)

	// Find nearest BSL above current price
	nearestBSL := math.MaxFloat64
	for _, h := range highs {
		if !math.IsNaN(h) && h > lastClose && h < nearestBSL {
			nearestBSL = h
		}
	}
	if nearestBSL == math.MaxFloat64 {
		nearestBSL = 0
	}

	// Find nearest SSL below current price
	nearestSSL := 0.0
	for _, l := range lows {
		if !math.IsNaN(l) && l < lastClose && l > nearestSSL {
			nearestSSL = l
		}
	}

	ctx.Liq.NearestBSL = nearestBSL
	ctx.Liq.NearestSSL = nearestSSL

	// Detect last sweep: look at the last 10 bars for a wick-through + close-back
	sweepWindow := min(len(slice), 10)
	recent := slice[len(slice)-sweepWindow:]
	for i := 1; i < len(recent); i++ {
		b := recent[i]
		prev := recent[i-1]

		// BSL sweep: wick above previous swing high then close below it
		if b.High > prev.High && b.Close < prev.High {
			ctx.Liq.LastSweep = SweepBSL
		}
		// SSL sweep: wick below previous swing low then close above it
		if b.Low < prev.Low && b.Close > prev.Low {
			ctx.Liq.LastSweep = SweepSSL
		}
	}

	if ctx.Liq.LastSweep == "" {
		ctx.Liq.LastSweep = SweepNone
	}
}

// detectImbalances identifies BISI (Bullish Imbalance, Sell-side Inefficiency)
// and SIBI (Sell-side Imbalance, Buy-side Inefficiency) in the last 30 bars.
// A bullish FVG (3-bar gap up) in discount = BISI. A bearish FVG in premium = SIBI.
func detectImbalances(ctx *ICTContext, bars []data.OHLCV) {
	lookback := min(len(bars), 30)
	slice := bars[len(bars)-lookback:]

	swingRange := ctx.Structure.LastSwingHigh - ctx.Structure.LastSwingLow
	if swingRange <= 0 {
		return
	}
	equilibrium := ctx.Structure.LastSwingLow + swingRange*0.5

	for i := 2; i < len(slice); i++ {
		a, _, c := slice[i-2], slice[i-1], slice[i]
		// Bullish FVG: gap between a.Low and c.High (a.Low > c.High is gap up)
		// Actually: bullish FVG => c.Low > a.High
		if c.Low > a.High {
			// Gap up — if it's in the discount zone it's BISI
			midGap := (c.Low + a.High) / 2
			if midGap < equilibrium {
				ctx.IsBISI = true
			}
		}
		// Bearish FVG: c.High < a.Low
		if c.High < a.Low {
			midGap := (c.High + a.Low) / 2
			if midGap > equilibrium {
				ctx.IsSIBI = true
			}
		}
	}
}

// detectTurtleSoup identifies Turtle Soup setups in the last 20 bars.
// Long: price sweeps a recent swing low, then closes back above it (SSL sweep + reclaim).
// Short: price sweeps a recent swing high, then closes back below it (BSL sweep + reclaim).
func detectTurtleSoup(ctx *ICTContext, bars []data.OHLCV) {
	lookback := min(len(bars), 20)
	slice := bars[len(bars)-lookback:]

	swingPeriod := 3
	highs := indicators.SwingHighs(slice, swingPeriod)
	lows := indicators.SwingLows(slice, swingPeriod)

	for i := swingPeriod + 1; i < len(slice)-1; i++ {
		b := slice[i]

		// Long: wick below a confirmed swing low, then close above it
		if !math.IsNaN(lows[i-1]) {
			swingLow := lows[i-1]
			if b.Low < swingLow && b.Close > swingLow {
				ctx.TurtleSoupLong = true
			}
		}

		// Short: wick above a confirmed swing high, then close below it
		if !math.IsNaN(highs[i-1]) {
			swingHigh := highs[i-1]
			if b.High > swingHigh && b.Close < swingHigh {
				ctx.TurtleSoupShort = true
			}
		}
	}
}

// detectThreeDrives identifies a Three Drives harmonic pattern in the recent bars.
// Looks for 3 consecutive higher highs (bearish) or 3 consecutive lower lows (bullish)
// with alternating pullbacks.
func detectThreeDrives(ctx *ICTContext, bars []data.OHLCV) {
	lookback := min(len(bars), 60)
	slice := bars[len(bars)-lookback:]

	swingPeriod := 4
	highs := indicators.SwingHighs(slice, swingPeriod)
	lows := indicators.SwingLows(slice, swingPeriod)

	// Collect confirmed swing highs in order
	type pt struct{ idx int; price float64 }
	var shPts, slPts []pt
	for i, h := range highs {
		if !math.IsNaN(h) {
			shPts = append(shPts, pt{i, h})
		}
	}
	for i, l := range lows {
		if !math.IsNaN(l) {
			slPts = append(slPts, pt{i, l})
		}
	}

	// Bearish Three Drives: 3 consecutive higher swing highs
	if len(shPts) >= 3 {
		n := len(shPts)
		h1, h2, h3 := shPts[n-3], shPts[n-2], shPts[n-1]
		if h2.price > h1.price && h3.price > h2.price {
			// Check each drive is roughly equal (within 20%)
			d1 := h2.price - h1.price
			d2 := h3.price - h2.price
			if d2 > 0 && math.Abs(d2-d1)/d1 < 0.20 {
				ctx.ThreeDrivesDown = true
			}
		}
	}

	// Bullish Three Drives: 3 consecutive lower swing lows
	if len(slPts) >= 3 {
		n := len(slPts)
		l1, l2, l3 := slPts[n-3], slPts[n-2], slPts[n-1]
		if l2.price < l1.price && l3.price < l2.price {
			d1 := l1.price - l2.price
			d2 := l2.price - l3.price
			if d2 > 0 && math.Abs(d2-d1)/d1 < 0.20 {
				ctx.ThreeDrivesUp = true
			}
		}
	}
}

// detectCISD identifies a Change In State of Delivery (CISD) — a candlestick
// that opens/closes in the opposite direction from the prior displacement.
// A bullish CISD = bearish displacement candle followed by a bullish engulfing.
// A bearish CISD = bullish displacement candle followed by a bearish engulfing.
func detectCISD(ctx *ICTContext, bars []data.OHLCV) {
	lookback := min(len(bars), 15)
	slice := bars[len(bars)-lookback:]

	for i := 2; i < len(slice); i++ {
		prev2 := slice[i-2]
		prev := slice[i-1]
		cur := slice[i]

		prev2Body := math.Abs(prev2.Close - prev2.Open)
		prevBody := math.Abs(prev.Close - prev.Open)
		curBody := math.Abs(cur.Close - cur.Open)

		// Displacement: large body candle (>1.5x average)
		avgBody := (prev2Body + prevBody) / 2
		isDisplacementDown := prev.Close < prev.Open && prevBody > avgBody*1.5
		isDisplacementUp := prev.Close > prev.Open && prevBody > avgBody*1.5

		// Bullish CISD: bearish displacement followed by bullish engulfing
		if isDisplacementDown && cur.Close > cur.Open && curBody > prevBody*0.8 {
			ctx.CISDLong = true
		}
		// Bearish CISD: bullish displacement followed by bearish engulfing
		if isDisplacementUp && cur.Close < cur.Open && curBody > prevBody*0.8 {
			ctx.CISDShort = true
		}
	}
}

// detectPremiumDiscount computes the premium/discount zones from the last swing
// range and determines whether current price is in premium (>61.8%) or discount (<38.2%).
func detectPremiumDiscount(ctx *ICTContext, bars []data.OHLCV) {
	high := ctx.Structure.LastSwingHigh
	low := ctx.Structure.LastSwingLow
	if high <= low || low == 0 {
		return
	}

	rng := high - low
	ctx.PremiumZone = low + rng*0.618
	ctx.DiscountZone = low + rng*0.382
	price := ctx.CurrentPrice

	ctx.IsInPremium = price >= ctx.PremiumZone
	ctx.IsInDiscount = price <= ctx.DiscountZone
}

// scoreAndRecommend assigns a setup quality score and recommendation.
func scoreAndRecommend(ctx *ICTContext) {
	score := 0.0

	// Structure: 2 pts
	if ctx.Structure.HasMSS {
		score += 2.0
	} else if ctx.Structure.HasCHoCH {
		score += 1.0
	} else if ctx.Structure.HasBOS {
		score += 0.5
	}

	// Liquidity sweep aligned with bias: 2 pts
	switch {
	case ctx.Structure.Bias == BiasBullish && ctx.Liq.LastSweep == SweepSSL:
		score += 2.0
	case ctx.Structure.Bias == BiasBearish && ctx.Liq.LastSweep == SweepBSL:
		score += 2.0
	case ctx.Liq.LastSweep != SweepNone:
		score += 0.5
	}

	// Price zone aligned with bias: 1.5 pts
	if ctx.Structure.Bias == BiasBullish && ctx.IsInDiscount {
		score += 1.5
	} else if ctx.Structure.Bias == BiasBearish && ctx.IsInPremium {
		score += 1.5
	}

	// Imbalance: 1 pt each
	if ctx.IsBISI && ctx.Structure.Bias == BiasBullish {
		score += 1.0
	}
	if ctx.IsSIBI && ctx.Structure.Bias == BiasBearish {
		score += 1.0
	}

	// Setups: 0.75 pt each
	if ctx.TurtleSoupLong && ctx.Structure.Bias == BiasBullish {
		score += 0.75
	}
	if ctx.TurtleSoupShort && ctx.Structure.Bias == BiasBearish {
		score += 0.75
	}
	if ctx.ThreeDrivesUp && ctx.Structure.Bias == BiasBullish {
		score += 0.75
	}
	if ctx.ThreeDrivesDown && ctx.Structure.Bias == BiasBearish {
		score += 0.75
	}
	if ctx.CISDLong && ctx.Structure.Bias == BiasBullish {
		score += 0.5
	}
	if ctx.CISDShort && ctx.Structure.Bias == BiasBearish {
		score += 0.5
	}

	// AMD Phase: distribution aligned with bias adds 0.5
	if ctx.Phase == PhaseDistribution {
		score += 0.5
	}

	if score > 10 {
		score = 10
	}
	ctx.SetupQuality = math.Round(score*10) / 10

	// Recommendation
	switch {
	case score >= 7:
		if ctx.Structure.Bias == BiasBullish {
			ctx.Recommendation = "High-quality LONG setup. Look for entries at discount / FVG / OB with tight stops below SSL."
		} else {
			ctx.Recommendation = "High-quality SHORT setup. Look for entries at premium / FVG / OB with tight stops above BSL."
		}
	case score >= 4:
		if ctx.Structure.Bias == BiasBullish {
			ctx.Recommendation = "Moderate bullish confluence. Wait for additional confirmation (sweep, FVG fill, or MSS) before entering."
		} else if ctx.Structure.Bias == BiasBearish {
			ctx.Recommendation = "Moderate bearish confluence. Wait for additional confirmation before entering."
		} else {
			ctx.Recommendation = "Mixed signals. No clear directional edge — stay out or reduce size."
		}
	default:
		ctx.Recommendation = "Low confluence. Avoid trading — wait for clearer structure / liquidity alignment."
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func qualityBar(score float64) string {
	filled := int(math.Round(score))
	if filled < 0 {
		filled = 0
	}
	if filled > 10 {
		filled = 10
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", 10-filled)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
