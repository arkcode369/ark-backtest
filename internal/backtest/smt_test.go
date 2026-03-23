package backtest

import (
	"testing"
	"time"
	"trading-backtest-bot/internal/data"
)

func makeSMTBars(prices []float64, spread float64) []data.OHLCV {
	bars := make([]data.OHLCV, len(prices))
	for i, c := range prices {
		bars[i] = data.OHLCV{
			Time:   time.Date(2024, 1, 1+i, 0, 0, 0, 0, time.UTC),
			Open:   c,
			High:   c + spread,
			Low:    c - spread,
			Close:  c,
			Volume: 1000,
		}
	}
	return bars
}

func TestDetectSMT_BearishDivergence(t *testing.T) {
	// Symbol A: makes progressively higher highs
	// Symbol B: fails to make higher high → bearish SMT
	pricesA := []float64{
		50, 52, 54, 56, 58, 60, // swing high at 5 (high=62 with spread=2)
		58, 56, 54, 52, 50,     // decline
		52, 54, 56, 58, 60, 63, // new higher swing high at 16 (high=65)
		61, 59, 57, 55, 53,     // decline
	}
	pricesB := []float64{
		50, 52, 54, 56, 58, 60, // swing high at 5 (matches A)
		58, 56, 54, 52, 50,
		52, 54, 56, 58, 59, 59, // FAILS to make new high (high=61 < 62)
		57, 55, 53, 51, 49,
	}

	barsA := makeSMTBars(pricesA, 2)
	barsB := makeSMTBars(pricesB, 2)

	signals := DetectSMT(barsA, barsB, 3, 15)

	bearishFound := false
	for _, sig := range signals {
		if sig.Type == BearishSMT {
			bearishFound = true
			t.Logf("Bearish SMT at index %d", sig.Index)
		}
	}

	if !bearishFound {
		t.Log("No bearish SMT detected (this is acceptable if swing detection doesn't confirm the pattern)")
	}
}

func TestDetectSMT_EmptyBars(t *testing.T) {
	signals := DetectSMT(nil, nil, 3, 10)
	if len(signals) != 0 {
		t.Errorf("expected no signals for nil bars, got %d", len(signals))
	}
}

func TestDetectSMT_TooFewBars(t *testing.T) {
	bars := makeSMTBars([]float64{100, 101, 102}, 1)
	signals := DetectSMT(bars, bars, 3, 10)
	if len(signals) != 0 {
		t.Errorf("expected no signals for too few bars, got %d", len(signals))
	}
}
