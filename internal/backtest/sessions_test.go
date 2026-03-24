package backtest

import (
	"testing"
	"time"
	"trading-backtest-bot/internal/data"
)

func TestLabelSessions_KillZones(t *testing.T) {
	// EST times - London kill zone is 02:00-05:00 EST = 07:00-10:00 UTC
	// NY AM kill zone is 07:00-10:00 EST = 12:00-15:00 UTC
	bars := []data.OHLCV{
		{Time: time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC)},  // 03:00 EST -> London KZ
		{Time: time.Date(2024, 1, 15, 13, 0, 0, 0, time.UTC)}, // 08:00 EST -> NY AM KZ
		{Time: time.Date(2024, 1, 15, 19, 0, 0, 0, time.UTC)}, // 14:00 EST -> NY PM
		{Time: time.Date(2024, 1, 15, 23, 30, 0, 0, time.UTC)}, // 18:30 EST -> Asian
		{Time: time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC)}, // 11:00 EST -> None
	}

	labels := LabelSessions(bars)

	if labels[0].Session != SessionLondon || !labels[0].IsKillZone {
		t.Errorf("bar 0: expected London kill zone, got session=%d killzone=%v", labels[0].Session, labels[0].IsKillZone)
	}
	if labels[1].Session != SessionNYAM || !labels[1].IsKillZone {
		t.Errorf("bar 1: expected NY AM kill zone, got session=%d killzone=%v", labels[1].Session, labels[1].IsKillZone)
	}
	if labels[3].Session != SessionAsian {
		t.Errorf("bar 3: expected Asian session, got session=%d", labels[3].Session)
	}
	if labels[4].Session != SessionNone {
		t.Errorf("bar 4: expected None, got session=%d", labels[4].Session)
	}
}

func TestComputeCBDR_BasicRange(t *testing.T) {
	// Create bars in the Asian session window (18:00-23:59 EST = 23:00-04:59 UTC)
	est := estLoc
	bars := []data.OHLCV{
		{Time: time.Date(2024, 1, 15, 18, 0, 0, 0, est), Open: 100, High: 105, Low: 98, Close: 102},
		{Time: time.Date(2024, 1, 15, 19, 0, 0, 0, est), Open: 102, High: 108, Low: 100, Close: 106},
		{Time: time.Date(2024, 1, 15, 20, 0, 0, 0, est), Open: 106, High: 110, Low: 103, Close: 107},
	}

	results := ComputeCBDR(bars)
	if len(results) == 0 {
		t.Fatal("expected at least one CBDR result")
	}

	cbdr := results[0]
	if cbdr.High != 110 {
		t.Errorf("CBDR high: expected 110, got %f", cbdr.High)
	}
	if cbdr.Low != 98 {
		t.Errorf("CBDR low: expected 98, got %f", cbdr.Low)
	}
	expectedRange := 12.0
	if cbdr.Range != expectedRange {
		t.Errorf("CBDR range: expected %f, got %f", expectedRange, cbdr.Range)
	}
	if cbdr.StdDev1Up != 110+12 {
		t.Errorf("StdDev1Up: expected %f, got %f", 122.0, cbdr.StdDev1Up)
	}
	if cbdr.StdDev1Down != 98-12 {
		t.Errorf("StdDev1Down: expected %f, got %f", 86.0, cbdr.StdDev1Down)
	}
}
