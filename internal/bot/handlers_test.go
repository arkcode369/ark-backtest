package bot

import (
	"testing"
)

func TestParseOptions(t *testing.T) {
	tests := []struct {
		name   string
		parts  []string
		expect map[string]string
	}{
		{"empty", nil, map[string]string{}},
		{"single", []string{"fast=9"}, map[string]string{"fast": "9"}},
		{"multiple", []string{"fast=9", "slow=21"}, map[string]string{"fast": "9", "slow": "21"}},
		{"case insensitive key", []string{"FAST=9"}, map[string]string{"fast": "9"}},
		{"no equals", []string{"foobar"}, map[string]string{}},
		{"value with equals", []string{"key=a=b"}, map[string]string{"key": "a=b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseOptions(tt.parts)
			if len(result) != len(tt.expect) {
				t.Errorf("expected %d opts, got %d", len(tt.expect), len(result))
			}
			for k, v := range tt.expect {
				if result[k] != v {
					t.Errorf("key %s: expected %s, got %s", k, v, result[k])
				}
			}
		})
	}
}

func TestGetOptFloat(t *testing.T) {
	opts := map[string]string{"capital": "50000", "bad": "abc"}

	if v := getOptFloat(opts, "capital", 10000); v != 50000 {
		t.Errorf("expected 50000, got %f", v)
	}
	if v := getOptFloat(opts, "missing", 10000); v != 10000 {
		t.Errorf("expected default 10000, got %f", v)
	}
	if v := getOptFloat(opts, "bad", 10000); v != 10000 {
		t.Errorf("expected default for bad parse, got %f", v)
	}
}

func TestGetOptStr(t *testing.T) {
	opts := map[string]string{"period": "1y"}
	if v := getOptStr(opts, "period", "2y"); v != "1y" {
		t.Errorf("expected 1y, got %s", v)
	}
	if v := getOptStr(opts, "missing", "2y"); v != "2y" {
		t.Errorf("expected default 2y, got %s", v)
	}
}

func TestDefaultPeriod(t *testing.T) {
	tests := map[string]string{
		"1m":  "7d",
		"5m":  "60d",
		"15m": "60d",
		"30m": "60d",
		"1h":  "1y",
		"60m": "1y",
		"1d":  "2y",
		"1w":  "2y",
	}
	for interval, expected := range tests {
		if got := defaultPeriod(interval); got != expected {
			t.Errorf("defaultPeriod(%s) = %s, want %s", interval, got, expected)
		}
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"My Strategy", "my_strategy"},
		{"EMA Cross 9/21", "ema_cross_921"},
		{"test!@#$%^&*()", "test"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := sanitizeFilename(tt.input); got != tt.expected {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestValidateParams(t *testing.T) {
	// Valid params
	if err := validateParams(map[string]float64{"fast": 9, "slow": 21}); err != nil {
		t.Errorf("expected nil, got %v", err)
	}

	// Zero period
	if err := validateParams(map[string]float64{"period": 0}); err == nil {
		t.Error("expected error for zero period")
	}

	// Period < 2
	if err := validateParams(map[string]float64{"fast": 1}); err == nil {
		t.Error("expected error for period < 2")
	}

	// Period too large
	if err := validateParams(map[string]float64{"slow": 99999}); err == nil {
		t.Error("expected error for period > 10000")
	}

	// Non-period positive value
	if err := validateParams(map[string]float64{"multiplier": 3.0}); err != nil {
		t.Errorf("expected nil for non-period param, got %v", err)
	}
}

func TestCopyParams(t *testing.T) {
	src := map[string]float64{"a": 1, "b": 2}
	dst := copyParams(src)

	if len(dst) != len(src) {
		t.Errorf("expected same length")
	}
	// Modify dst shouldn't affect src
	dst["a"] = 99
	if src["a"] == 99 {
		t.Error("copyParams returned a reference, not a copy")
	}
}

func TestConvertAIResponse(t *testing.T) {
	// Headers
	result := convertAIResponse("## MY HEADER\nsome text")
	if result == "" {
		t.Error("expected non-empty result")
	}

	// Bold removal
	result = convertAIResponse("This is **bold** text")
	if result != "This is bold text" {
		t.Errorf("expected bold markers removed, got %q", result)
	}

	// Code block
	result = convertAIResponse("```go\nfmt.Println()\n```")
	if result == "" {
		t.Error("expected non-empty for code block")
	}
}

func TestUserRateLimiter(t *testing.T) {
	rl := NewUserRateLimiter()

	// First call should be allowed
	if !rl.Allow(123, "price") {
		t.Error("first call should be allowed")
	}

	// Immediate second call should be denied
	if rl.Allow(123, "price") {
		t.Error("immediate second call should be denied")
	}

	// Different user should be allowed
	if !rl.Allow(456, "price") {
		t.Error("different user should be allowed")
	}

	// Different category should be allowed
	if !rl.Allow(123, "backtest") {
		t.Error("different category should be allowed")
	}

	// Unknown category should always be allowed
	if !rl.Allow(123, "unknown") {
		t.Error("unknown category should be allowed")
	}
}

func TestGenerateCombinations(t *testing.T) {
	ranges := map[string][3]float64{
		"fast": {5, 10, 5}, // 5, 10
		"slow": {20, 25, 5}, // 20, 25
	}
	defaults := map[string]float64{"fast": 9, "slow": 21, "extra": 100}

	combos := generateCombinations(ranges, defaults)
	if len(combos) != 4 { // 2 x 2
		t.Errorf("expected 4 combinations, got %d", len(combos))
	}

	// Each combo should have the extra default
	for _, c := range combos {
		if c["extra"] != 100 {
			t.Error("default param should be preserved")
		}
	}
}
