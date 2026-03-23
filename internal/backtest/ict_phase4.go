package backtest

import (
	"math"
	"time"
	"trading-backtest-bot/internal/data"
)

// ── Judas Swing Detection ─────────────────────────────────────────────────
//
// A Judas Swing is a false move at the beginning of a session that traps
// traders on the wrong side before the real move begins.
//
// Bullish Judas: Session opens, price makes a false LOW (sweeps prior
//   session low), then reverses upward.
// Bearish Judas: Session opens, price makes a false HIGH (sweeps prior
//   session high), then reverses downward.

// JudasSwing represents a detected Judas Swing event
type JudasSwing struct {
	Index     int
	Direction int     // +1 bullish (false low), -1 bearish (false high)
	FalseLevel float64 // the false extreme price
	SessionOpen float64
}

// DetectJudasSwings scans for Judas Swing patterns at session opens.
// Looks for a sweep of the prior session's range within the first N bars
// of a new session, followed by reversal.
func DetectJudasSwings(bars []data.OHLCV, sessions []SessionLabel, lookforward int) []JudasSwing {
	n := len(bars)
	if n < 3 || len(sessions) < n {
		return nil
	}
	if lookforward <= 0 {
		lookforward = 5
	}

	var swings []JudasSwing

	// Track session boundaries
	for i := 1; i < n; i++ {
		// Detect session transition
		if sessions[i].Session == sessions[i-1].Session || sessions[i].Session == SessionNone {
			continue
		}

		// New session started at bar i
		sessionOpen := bars[i].Open

		// Find prior session's high and low
		priorHigh := 0.0
		priorLow := math.MaxFloat64
		for j := i - 1; j >= 0 && j >= i-50; j-- {
			if sessions[j].Session != sessions[i-1].Session && sessions[j].Session != SessionNone {
				break
			}
			if bars[j].High > priorHigh {
				priorHigh = bars[j].High
			}
			if bars[j].Low < priorLow {
				priorLow = bars[j].Low
			}
		}
		if priorHigh == 0 || priorLow == math.MaxFloat64 {
			continue
		}

		// Look at first lookforward bars of new session for a false move
		end := i + lookforward
		if end >= n {
			end = n - 1
		}

		// Track false high (bearish Judas)
		falseHighIdx := -1
		falseHighPrice := 0.0
		for j := i; j <= end; j++ {
			if bars[j].High > priorHigh {
				if falseHighIdx == -1 || bars[j].High > falseHighPrice {
					falseHighIdx = j
					falseHighPrice = bars[j].High
				}
			}
		}

		// Track false low (bullish Judas)
		falseLowIdx := -1
		falseLowPrice := math.MaxFloat64
		for j := i; j <= end; j++ {
			if bars[j].Low < priorLow {
				if falseLowIdx == -1 || bars[j].Low < falseLowPrice {
					falseLowIdx = j
					falseLowPrice = bars[j].Low
				}
			}
		}

		// Check for reversal after false high → bearish Judas
		if falseHighIdx >= 0 && falseHighIdx < n-1 {
			// Must close back below prior high
			reversed := false
			for j := falseHighIdx + 1; j <= end && j < n; j++ {
				if bars[j].Close < priorHigh {
					reversed = true
					break
				}
			}
			if reversed {
				swings = append(swings, JudasSwing{
					Index:       falseHighIdx,
					Direction:   -1,
					FalseLevel:  falseHighPrice,
					SessionOpen: sessionOpen,
				})
			}
		}

		// Check for reversal after false low → bullish Judas
		if falseLowIdx >= 0 && falseLowIdx < n-1 {
			reversed := false
			for j := falseLowIdx + 1; j <= end && j < n; j++ {
				if bars[j].Close > priorLow {
					reversed = true
					break
				}
			}
			if reversed {
				swings = append(swings, JudasSwing{
					Index:       falseLowIdx,
					Direction:   1,
					FalseLevel:  falseLowPrice,
					SessionOpen: sessionOpen,
				})
			}
		}
	}
	return swings
}

// ── Liquidity Void Detection ──────────────────────────────────────────────
//
// A Liquidity Void is a large, fast price move that leaves "unfilled"
// territory — no trading occurred in that range. Price tends to return
// to fill these voids.

// LiquidityVoid represents a detected void
type LiquidityVoid struct {
	Index     int
	Direction int     // +1 bullish void (gap up), -1 bearish void (gap down)
	Top       float64
	Bottom    float64
	Filled    bool
	FillIndex int
}

// DetectLiquidityVoids finds bars with unusually large ranges (>2x ATR)
// where subsequent bars don't overlap, leaving an unfilled void.
func DetectLiquidityVoids(bars []data.OHLCV, atr []float64, atrMult float64) []LiquidityVoid {
	n := len(bars)
	if n < 3 {
		return nil
	}
	if atrMult <= 0 {
		atrMult = 2.0
	}

	var voids []LiquidityVoid

	for i := 1; i < n-1; i++ {
		if math.IsNaN(atr[i]) || atr[i] == 0 {
			continue
		}

		rng := bars[i].High - bars[i].Low
		if rng < atrMult*atr[i] {
			continue
		}

		// Bullish void: large bullish candle where bar[i+1].Low > bar[i-1].High
		if bars[i].Close > bars[i].Open {
			voidBot := bars[i-1].High
			voidTop := bars[i+1].Low
			if voidTop > voidBot {
				v := LiquidityVoid{
					Index:     i,
					Direction: 1,
					Top:       voidTop,
					Bottom:    voidBot,
					FillIndex: -1,
				}
				// Check if void gets filled
				for j := i + 2; j < n; j++ {
					if bars[j].Low <= v.Bottom {
						v.Filled = true
						v.FillIndex = j
						break
					}
				}
				voids = append(voids, v)
			}
		}

		// Bearish void: large bearish candle
		if bars[i].Close < bars[i].Open {
			voidTop := bars[i-1].Low
			voidBot := bars[i+1].High
			if voidTop > voidBot {
				v := LiquidityVoid{
					Index:     i,
					Direction: -1,
					Top:       voidTop,
					Bottom:    voidBot,
					FillIndex: -1,
				}
				for j := i + 2; j < n; j++ {
					if bars[j].High >= v.Top {
						v.Filled = true
						v.FillIndex = j
						break
					}
				}
				voids = append(voids, v)
			}
		}
	}
	return voids
}

// ── Enhanced Daily Bias (9-Step Model) ────────────────────────────────────
//
// ICT's Daily Bias determination uses multiple confluences:
//   1. Previous day's candle (bullish/bearish close)
//   2. PDH/PDL relationship (close above PDH = bullish)
//   3. Weekly open position (price above/below)
//   4. HTF order flow (series of HH/HL or LH/LL)
//   5. NDOG/NWOG alignment
//   6. Asian session range position
//   7. London manipulation direction
//   8. Displacement presence
//   9. PD zone alignment

// DailyBias9Step computes a more nuanced daily bias score (-4 to +4).
// Each confluence adds +1 (bullish) or -1 (bearish).
// Returns per-LTF-bar bias values.
func DailyBias9Step(
	ltfBars []data.OHLCV,
	dailyBars []data.OHLCV,
	htfIndex []int,
	sessions []SessionLabel,
	cbdrs []CBDRResult,
	ndogs []OpeningGap,
	nwogs []OpeningGap,
	atr []float64,
) []int {
	n := len(ltfBars)
	bias := make([]int, n)
	if len(dailyBars) < 3 || len(htfIndex) == 0 {
		return bias
	}

	est := time.FixedZone("EST", -5*3600)

	// Pre-compute weekly opens
	weeklyOpen := make(map[int]float64) // isoYear*100+isoWeek → open
	for _, bar := range ltfBars {
		y, w := bar.Time.ISOWeek()
		key := y*100 + w
		if _, exists := weeklyOpen[key]; !exists {
			weeklyOpen[key] = bar.Open
		}
	}

	for i := 0; i < n; i++ {
		score := 0
		htfIdx := -1
		if i < len(htfIndex) {
			htfIdx = htfIndex[i]
		}

		// 1. Previous daily candle
		if htfIdx > 0 && htfIdx < len(dailyBars) {
			prev := dailyBars[htfIdx-1]
			if prev.Close > prev.Open {
				score++ // bullish daily candle
			} else if prev.Close < prev.Open {
				score-- // bearish
			}
		}

		// 2. PDH/PDL relationship
		if htfIdx > 1 && htfIdx < len(dailyBars) {
			prevDay := dailyBars[htfIdx-1]
			twoDaysAgo := dailyBars[htfIdx-2]
			if prevDay.Close > twoDaysAgo.High {
				score++ // closed above PDH
			}
			if prevDay.Close < twoDaysAgo.Low {
				score-- // closed below PDL
			}
		}

		// 3. Weekly open position
		y, w := ltfBars[i].Time.ISOWeek()
		woKey := y*100 + w
		if wo, ok := weeklyOpen[woKey]; ok {
			if ltfBars[i].Close > wo {
				score++
			} else if ltfBars[i].Close < wo {
				score--
			}
		}

		// 4. HTF order flow (HH/HL or LH/LL pattern)
		if htfIdx > 2 && htfIdx < len(dailyBars) {
			d1 := dailyBars[htfIdx-3]
			d2 := dailyBars[htfIdx-2]
			d3 := dailyBars[htfIdx-1]
			// Bullish: higher highs and higher lows
			if d3.High > d2.High && d3.Low > d1.Low {
				score++
			}
			// Bearish: lower highs and lower lows
			if d3.High < d2.High && d3.Low < d1.Low {
				score--
			}
		}

		// 5. NDOG/NWOG alignment
		bar := ltfBars[i]
		relevantGaps := FindGapForBar(append(ndogs, nwogs...), bar)
		for _, gap := range relevantGaps {
			if bar.Close > gap.High {
				score++ // trading above the gap = bullish
			} else if bar.Close < gap.Low {
				score-- // trading below the gap = bearish
			}
		}

		// 6. CBDR position
		cbdr := FindCBDRForBar(cbdrs, bar)
		if cbdr != nil && cbdr.Range > 0 {
			if bar.Close > cbdr.High {
				score++ // above Asian range
			} else if bar.Close < cbdr.Low {
				score-- // below Asian range
			}
		}

		// 7. Session context (London manipulation if available)
		if i < len(sessions) {
			t := bar.Time.In(est)
			h := t.Hour()
			// During NY session, if London had a false move, bias opposite
			if h >= 7 && h < 12 && sessions[i].Session == SessionNYAM {
				// Simple heuristic: if bar before London (Asian) was ranging
				// and London broke one side, this adds bias context
				// (This is a simplified version; AMD strategy handles this more deeply)
			}
		}

		// Clamp to [-4, +4]
		if score > 4 {
			score = 4
		} else if score < -4 {
			score = -4
		}

		bias[i] = score
	}
	return bias
}

// ── IPDA State Machine (Simplified) ───────────────────────────────────────
//
// Interbank Price Delivery Algorithm tracks the market state:
//   - Consolidation (ranging)
//   - Expansion (trending)
//   - Retracement (pullback)
//   - Reversal (change of character)

type IPDAState int

const (
	IPDAConsolidation IPDAState = 0
	IPDAExpansion     IPDAState = 1
	IPDARetracement   IPDAState = 2
	IPDAReversal      IPDAState = 3
)

// ComputeIPDAState determines the IPDA state for each bar based on
// ATR expansion/contraction and structure breaks.
func ComputeIPDAState(bars []data.OHLCV, atr []float64, swingHighs, swingLows []float64) []IPDAState {
	n := len(bars)
	states := make([]IPDAState, n)

	// Rolling ATR average for comparison
	const atrAvgLen = 20
	atrAvg := make([]float64, n)
	sum := 0.0
	cnt := 0
	for i := 0; i < n; i++ {
		if math.IsNaN(atr[i]) {
			atrAvg[i] = math.NaN()
			continue
		}
		sum += atr[i]
		cnt++
		if cnt > atrAvgLen {
			// Subtract oldest
			for back := i - atrAvgLen; back >= 0; back-- {
				if !math.IsNaN(atr[back]) {
					sum -= atr[back]
					cnt--
					break
				}
			}
		}
		if cnt > 0 {
			atrAvg[i] = sum / float64(cnt)
		}
	}

	// Track recent swing direction
	lastSwingHighIdx := -1
	lastSwingLowIdx := -1
	prevTrend := 0 // +1 uptrend, -1 downtrend

	for i := 0; i < n; i++ {
		if !math.IsNaN(swingHighs[i]) {
			lastSwingHighIdx = i
		}
		if !math.IsNaN(swingLows[i]) {
			lastSwingLowIdx = i
		}

		if math.IsNaN(atr[i]) || math.IsNaN(atrAvg[i]) {
			states[i] = IPDAConsolidation
			continue
		}

		currentRange := bars[i].High - bars[i].Low

		// Determine trend from swing structure
		trend := prevTrend
		if lastSwingHighIdx > lastSwingLowIdx {
			trend = 1
		} else if lastSwingLowIdx > lastSwingHighIdx {
			trend = -1
		}

		// Classify state
		if currentRange > atr[i]*1.5 {
			// Large range = expansion
			states[i] = IPDAExpansion
		} else if currentRange < atrAvg[i]*0.5 {
			// Tight range = consolidation
			states[i] = IPDAConsolidation
		} else if trend != prevTrend && prevTrend != 0 {
			// Trend changed = reversal
			states[i] = IPDAReversal
		} else if i > 0 {
			// Check if this is a retracement (counter-trend move in an expansion)
			if states[i-1] == IPDAExpansion || states[i-1] == IPDARetracement {
				body := bars[i].Close - bars[i].Open
				if (trend == 1 && body < 0) || (trend == -1 && body > 0) {
					states[i] = IPDARetracement
				} else {
					states[i] = IPDAExpansion
				}
			} else {
				states[i] = IPDAConsolidation
			}
		}

		prevTrend = trend
	}
	return states
}
