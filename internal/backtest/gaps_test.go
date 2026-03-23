package backtest

import (
	"testing"
	"time"
	"trading-backtest-bot/internal/data"
)

func TestDetectNDOG_BasicGap(t *testing.T) {
	// Two days of bars with a gap between them
	bars := []data.OHLCV{
		// Day 1 bars
		{Time: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC), Open: 100, High: 105, Low: 98, Close: 102},
		{Time: time.Date(2024, 1, 15, 14, 0, 0, 0, time.UTC), Open: 102, High: 106, Low: 100, Close: 104},
		// Day 2 bars - opens at 108 (gap up from 104)
		{Time: time.Date(2024, 1, 16, 10, 0, 0, 0, time.UTC), Open: 108, High: 112, Low: 106, Close: 110},
		{Time: time.Date(2024, 1, 16, 14, 0, 0, 0, time.UTC), Open: 110, High: 113, Low: 108, Close: 111},
	}

	gaps := DetectNDOG(bars)
	if len(gaps) != 1 {
		t.Fatalf("expected 1 NDOG, got %d", len(gaps))
	}

	gap := gaps[0]
	if gap.Type != NDOG {
		t.Errorf("expected NDOG type, got %d", gap.Type)
	}
	if !gap.IsBullish {
		t.Error("expected bullish gap (open 108 > close 104)")
	}
	if gap.High != 108 {
		t.Errorf("gap high: expected 108, got %f", gap.High)
	}
	if gap.Low != 104 {
		t.Errorf("gap low: expected 104, got %f", gap.Low)
	}
}

func TestDetectNWOG_WeekendGap(t *testing.T) {
	// Friday close to Monday open
	bars := []data.OHLCV{
		// Friday
		{Time: time.Date(2024, 1, 12, 0, 0, 0, 0, time.UTC), Open: 100, High: 105, Low: 98, Close: 103},
		// Monday - gap up
		{Time: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), Open: 107, High: 112, Low: 105, Close: 110},
	}

	gaps := DetectNWOG(bars)
	if len(gaps) != 1 {
		t.Fatalf("expected 1 NWOG, got %d", len(gaps))
	}

	gap := gaps[0]
	if gap.Type != NWOG {
		t.Errorf("expected NWOG type, got %d", gap.Type)
	}
	if !gap.IsBullish {
		t.Error("expected bullish NWOG")
	}
	if gap.High != 107 {
		t.Errorf("gap high: expected 107, got %f", gap.High)
	}
	if gap.Low != 103 {
		t.Errorf("gap low: expected 103, got %f", gap.Low)
	}
}

func TestDetectNDOG_NoGap(t *testing.T) {
	// Continuous bars with no gap
	bars := []data.OHLCV{
		{Time: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC), Open: 100, High: 105, Low: 98, Close: 104},
		{Time: time.Date(2024, 1, 16, 10, 0, 0, 0, time.UTC), Open: 104, High: 108, Low: 102, Close: 106},
	}

	gaps := DetectNDOG(bars)
	if len(gaps) != 0 {
		t.Errorf("expected no gaps when open equals previous close, got %d", len(gaps))
	}
}
