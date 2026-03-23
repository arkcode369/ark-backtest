package backtest

import (
	"math"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// SMTType identifies the divergence direction
type SMTType int

const (
	BullishSMT SMTType = 1  // One makes new low, other doesn't → reversal up
	BearishSMT SMTType = -1 // One makes new high, other doesn't → reversal down
)

// SMTSignal represents a detected SMT divergence at a specific bar
type SMTSignal struct {
	Index   int
	Type    SMTType
	SymbolA string
	SymbolB string
}

// DetectSMT analyzes two time-aligned bar series for SMT divergence.
// It detects when one symbol makes a new swing high/low within the lookback
// window but the other symbol fails to do so.
//
// Parameters:
// - barsA, barsB: time-aligned OHLCV bars (must be same length)
// - swingPeriod: period for swing point detection
// - lookback: number of bars to look back for comparing highs/lows
func DetectSMT(barsA, barsB []data.OHLCV, swingPeriod, lookback int) []SMTSignal {
	n := min(len(barsA), len(barsB))
	if n < swingPeriod*2+1 {
		return nil
	}

	swingHighsA := indicators.SwingHighs(barsA[:n], swingPeriod)
	swingLowsA := indicators.SwingLows(barsA[:n], swingPeriod)
	swingHighsB := indicators.SwingHighs(barsB[:n], swingPeriod)
	swingLowsB := indicators.SwingLows(barsB[:n], swingPeriod)

	var signals []SMTSignal

	for i := swingPeriod + lookback; i < n-swingPeriod; i++ {
		// Check for bearish SMT: Symbol A makes new swing high, Symbol B doesn't
		if !math.IsNaN(swingHighsA[i]) {
			// Find previous swing high of A in lookback
			prevHighA := math.NaN()
			for j := i - 1; j >= i-lookback && j >= swingPeriod; j-- {
				if !math.IsNaN(swingHighsA[j]) {
					prevHighA = swingHighsA[j]
					break
				}
			}
			if !math.IsNaN(prevHighA) && swingHighsA[i] > prevHighA {
				// A made a higher high. Check if B also made a higher high
				bHigherHigh := false
				// Look for B's swing high near bar i
				for j := i - swingPeriod; j <= i+swingPeriod && j < n; j++ {
					if !math.IsNaN(swingHighsB[j]) {
						// Check if B also exceeded its previous swing high
						prevHighB := math.NaN()
						for k := j - 1; k >= j-lookback && k >= swingPeriod; k-- {
							if !math.IsNaN(swingHighsB[k]) {
								prevHighB = swingHighsB[k]
								break
							}
						}
						if !math.IsNaN(prevHighB) && swingHighsB[j] > prevHighB {
							bHigherHigh = true
						}
						break
					}
				}
				if !bHigherHigh {
					signals = append(signals, SMTSignal{
						Index: i,
						Type:  BearishSMT,
					})
				}
			}
		}

		// Check for bullish SMT: Symbol A makes new swing low, Symbol B doesn't
		if !math.IsNaN(swingLowsA[i]) {
			prevLowA := math.NaN()
			for j := i - 1; j >= i-lookback && j >= swingPeriod; j-- {
				if !math.IsNaN(swingLowsA[j]) {
					prevLowA = swingLowsA[j]
					break
				}
			}
			if !math.IsNaN(prevLowA) && swingLowsA[i] < prevLowA {
				// A made a lower low. Check if B also made a lower low
				bLowerLow := false
				for j := i - swingPeriod; j <= i+swingPeriod && j < n; j++ {
					if !math.IsNaN(swingLowsB[j]) {
						prevLowB := math.NaN()
						for k := j - 1; k >= j-lookback && k >= swingPeriod; k-- {
							if !math.IsNaN(swingLowsB[k]) {
								prevLowB = swingLowsB[k]
								break
							}
						}
						if !math.IsNaN(prevLowB) && swingLowsB[j] < prevLowB {
							bLowerLow = true
						}
						break
					}
				}
				if !bLowerLow {
					signals = append(signals, SMTSignal{
						Index: i,
						Type:  BullishSMT,
					})
				}
			}
		}
	}
	return signals
}
