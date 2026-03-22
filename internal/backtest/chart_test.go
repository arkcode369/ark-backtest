package backtest

import (
	"strings"
	"testing"
)

func TestFormatEquityCurve(t *testing.T) {
	t.Run("basic uptrend", func(t *testing.T) {
		curve := []float64{10000, 10100, 10200, 10300, 10500, 10800, 10700, 10900, 11000}
		chart := FormatEquityCurve(curve, 20, 8)
		if chart == "" {
			t.Error("expected non-empty chart")
		}
		if !strings.Contains(chart, "10000") || !strings.Contains(chart, "11000") {
			t.Error("chart should contain start/end equity values")
		}
	})

	t.Run("too short", func(t *testing.T) {
		curve := []float64{10000}
		chart := FormatEquityCurve(curve, 20, 8)
		if chart != "" {
			t.Error("expected empty chart for single point")
		}
	})

	t.Run("flat line", func(t *testing.T) {
		curve := []float64{10000, 10000, 10000, 10000}
		chart := FormatEquityCurve(curve, 20, 8)
		if chart == "" {
			t.Error("expected non-empty chart for flat line")
		}
	})

	t.Run("downtrend", func(t *testing.T) {
		curve := []float64{10000, 9800, 9600, 9500, 9200}
		chart := FormatEquityCurve(curve, 20, 8)
		if chart == "" {
			t.Error("expected non-empty chart")
		}
		if !strings.Contains(chart, "\u25bc") { // down arrow
			t.Error("downtrend chart should contain down arrow")
		}
	})
}

func TestDownsample(t *testing.T) {
	data := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	result := downsample(data, 5)
	if len(result) != 5 {
		t.Errorf("expected 5 samples, got %d", len(result))
	}

	// First bucket should average [1,2] = 1.5
	if result[0] < 1 || result[0] > 2.5 {
		t.Errorf("first bucket out of expected range: %f", result[0])
	}
}
