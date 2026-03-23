package data

import (
	"testing"
	"time"
)

func TestAlignHTFToLTF_Basic(t *testing.T) {
	// Daily bars
	htf := []OHLCV{
		{Time: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)},
		{Time: time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC)},
		{Time: time.Date(2024, 1, 17, 0, 0, 0, 0, time.UTC)},
	}

	// Hourly bars
	ltf := []OHLCV{
		{Time: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)}, // during day 0
		{Time: time.Date(2024, 1, 15, 14, 0, 0, 0, time.UTC)}, // during day 0
		{Time: time.Date(2024, 1, 16, 9, 0, 0, 0, time.UTC)},  // during day 1
		{Time: time.Date(2024, 1, 16, 15, 0, 0, 0, time.UTC)}, // during day 1
		{Time: time.Date(2024, 1, 17, 11, 0, 0, 0, time.UTC)}, // during day 2
	}

	result := AlignHTFToLTF(ltf, htf)

	expected := []int{0, 0, 1, 1, 2}
	for i, exp := range expected {
		if result[i] != exp {
			t.Errorf("ltf bar %d: expected htf index %d, got %d", i, exp, result[i])
		}
	}
}

func TestAlignHTFToLTF_Empty(t *testing.T) {
	ltf := []OHLCV{{Time: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)}}
	result := AlignHTFToLTF(ltf, nil)
	if result[0] != -1 {
		t.Errorf("expected -1 for empty HTF, got %d", result[0])
	}
}

func TestAlignHTFToLTF_BeforeAllHTF(t *testing.T) {
	htf := []OHLCV{{Time: time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC)}}
	ltf := []OHLCV{{Time: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)}} // before HTF

	result := AlignHTFToLTF(ltf, htf)
	if result[0] != -1 {
		t.Errorf("expected -1 for LTF before all HTF bars, got %d", result[0])
	}
}
