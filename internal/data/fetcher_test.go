package data

import (
	"math"
	"testing"
)

func TestToFloat(t *testing.T) {
	tests := []struct {
		name   string
		input  interface{}
		val    float64
		ok     bool
	}{
		{"float64", 42.5, 42.5, true},
		{"nil", nil, 0, false},
		{"NaN", math.NaN(), 0, false},
		{"string", "hello", 0, false},
		{"zero", 0.0, 0.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, ok := toFloat(tt.input)
			if ok != tt.ok {
				t.Errorf("ok: expected %v, got %v", tt.ok, ok)
			}
			if ok && val != tt.val {
				t.Errorf("val: expected %f, got %f", tt.val, val)
			}
		})
	}
}

func TestSafeGet(t *testing.T) {
	s := []interface{}{1.0, 2.0, 3.0}

	if v := safeGet(s, 0); v != 1.0 {
		t.Errorf("expected 1.0, got %v", v)
	}
	if v := safeGet(s, 2); v != 3.0 {
		t.Errorf("expected 3.0, got %v", v)
	}
	if v := safeGet(s, 5); v != nil {
		t.Errorf("expected nil for out of bounds, got %v", v)
	}
	if v := safeGet(nil, 0); v != nil {
		t.Errorf("expected nil for nil slice, got %v", v)
	}
}

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		f        float64
		decimals int
		expected string
	}{
		{1234.5678, 2, "1234.57"},
		{0.001, 5, "0.00100"},
		{100.0, 0, "100"},
	}

	for _, tt := range tests {
		result := FormatNumber(tt.f, tt.decimals)
		if result != tt.expected {
			t.Errorf("FormatNumber(%f, %d) = %q, want %q", tt.f, tt.decimals, result, tt.expected)
		}
	}
}

func TestGetSymbol(t *testing.T) {
	// Exact match
	sym, ok := GetSymbol("XAUUSD")
	if !ok {
		t.Error("expected XAUUSD to be found")
	}
	if sym.Category != "Metals" {
		t.Errorf("expected Metals category, got %s", sym.Category)
	}

	// Case insensitive
	sym2, ok := GetSymbol("xauusd")
	if !ok {
		t.Error("expected lowercase xauusd to be found")
	}
	if sym.Ticker != sym2.Ticker {
		t.Error("case-insensitive lookup should return same symbol")
	}

	// Unknown symbol
	_, ok = GetSymbol("FAKESYMBOL")
	if ok {
		t.Error("expected FAKESYMBOL to not be found")
	}
}

func TestValidIntervals(t *testing.T) {
	expected := []string{"1m", "2m", "5m", "15m", "30m", "1h", "60m", "4h", "1d", "1w"}
	for _, iv := range expected {
		if _, ok := ValidIntervals[iv]; !ok {
			t.Errorf("expected interval %s to be valid", iv)
		}
	}

	if _, ok := ValidIntervals["3h"]; ok {
		t.Error("3h should not be a valid interval")
	}
}
