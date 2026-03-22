package indicators

import (
	"math"
	"testing"
	"trading-backtest-bot/internal/data"
)

// almostEqual checks whether two floats are within a given tolerance.
func almostEqual(a, b, tolerance float64) bool {
	return math.Abs(a-b) < tolerance
}

const tol = 1e-9

// ── SMA ──────────────────────────────────────────────────────────────────────

func TestSMA_KnownValues(t *testing.T) {
	closes := []float64{1, 2, 3, 4, 5}
	result := SMA(closes, 3)

	if !math.IsNaN(result[0]) {
		t.Errorf("SMA[0] expected NaN, got %f", result[0])
	}
	if !math.IsNaN(result[1]) {
		t.Errorf("SMA[1] expected NaN, got %f", result[1])
	}
	expected := []float64{2, 3, 4}
	for i, exp := range expected {
		idx := i + 2
		if !almostEqual(result[idx], exp, tol) {
			t.Errorf("SMA[%d] expected %f, got %f", idx, exp, result[idx])
		}
	}
}

func TestSMA_PeriodOne(t *testing.T) {
	closes := []float64{10, 20, 30}
	result := SMA(closes, 1)
	for i, c := range closes {
		if !almostEqual(result[i], c, tol) {
			t.Errorf("SMA period=1 [%d] expected %f, got %f", i, c, result[i])
		}
	}
}

func TestSMA_EmptyInput(t *testing.T) {
	result := SMA([]float64{}, 3)
	if len(result) != 0 {
		t.Errorf("SMA empty input expected len 0, got %d", len(result))
	}
}

func TestSMA_PeriodGreaterThanLen(t *testing.T) {
	closes := []float64{1, 2}
	result := SMA(closes, 5)
	for i, v := range result {
		if !math.IsNaN(v) {
			t.Errorf("SMA[%d] expected NaN, got %f", i, v)
		}
	}
}

// ── EMA ──────────────────────────────────────────────────────────────────────

func TestEMA_FirstValueEqualsFirstClose(t *testing.T) {
	closes := []float64{10, 20, 30, 40, 50}
	result := EMA(closes, 3)
	if !almostEqual(result[0], 10, tol) {
		t.Errorf("EMA[0] expected 10, got %f", result[0])
	}
}

func TestEMA_KnownValues(t *testing.T) {
	closes := []float64{10, 20, 30, 40, 50}
	result := EMA(closes, 3)
	// k = 2/(3+1) = 0.5
	// EMA[0] = 10
	// EMA[1] = 20*0.5 + 10*0.5 = 15
	// EMA[2] = 30*0.5 + 15*0.5 = 22.5
	// EMA[3] = 40*0.5 + 22.5*0.5 = 31.25
	// EMA[4] = 50*0.5 + 31.25*0.5 = 40.625
	expected := []float64{10, 15, 22.5, 31.25, 40.625}
	for i, exp := range expected {
		if !almostEqual(result[i], exp, tol) {
			t.Errorf("EMA[%d] expected %f, got %f", i, exp, result[i])
		}
	}
}

func TestEMA_PeriodOne(t *testing.T) {
	// k = 2/(1+1) = 1.0, so EMA[i] = closes[i]*1 + prev*0 = closes[i]
	closes := []float64{5, 10, 15}
	result := EMA(closes, 1)
	for i, c := range closes {
		if !almostEqual(result[i], c, tol) {
			t.Errorf("EMA period=1 [%d] expected %f, got %f", i, c, result[i])
		}
	}
}

// ── RSI ──────────────────────────────────────────────────────────────────────

func TestRSI_AllUp(t *testing.T) {
	closes := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	result := RSI(closes, 5)
	// After warmup, with only gains and no losses, RSI should be 100.
	for i := 5; i < len(result); i++ {
		if !almostEqual(result[i], 100, tol) {
			t.Errorf("RSI all-up [%d] expected 100, got %f", i, result[i])
		}
	}
}

func TestRSI_AllDown(t *testing.T) {
	closes := []float64{10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	result := RSI(closes, 5)
	// After warmup, with only losses and no gains, RSI should be 0.
	for i := 5; i < len(result); i++ {
		if !almostEqual(result[i], 0, tol) {
			t.Errorf("RSI all-down [%d] expected 0, got %f", i, result[i])
		}
	}
}

func TestRSI_PeriodGreaterThanLen(t *testing.T) {
	closes := []float64{1, 2, 3}
	result := RSI(closes, 10)
	for i, v := range result {
		if !math.IsNaN(v) {
			t.Errorf("RSI[%d] expected NaN, got %f", i, v)
		}
	}
}

func TestRSI_WarmupIsNaN(t *testing.T) {
	closes := []float64{44, 44.34, 44.09, 43.61, 44.33, 44.83, 45.10, 45.42, 45.84}
	result := RSI(closes, 5)
	for i := 0; i < 5; i++ {
		if !math.IsNaN(result[i]) {
			t.Errorf("RSI[%d] expected NaN during warmup, got %f", i, result[i])
		}
	}
	// After warmup the value should be a valid number.
	if math.IsNaN(result[5]) {
		t.Error("RSI[5] expected a valid number, got NaN")
	}
}

// ── MACD ─────────────────────────────────────────────────────────────────────

func TestMACD_LineEqualsEMADifference(t *testing.T) {
	closes := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	fast, slow, sig := 5, 10, 3
	result := MACD(closes, fast, slow, sig)

	emaFast := EMA(closes, fast)
	emaSlow := EMA(closes, slow)

	for i := range closes {
		expected := emaFast[i] - emaSlow[i]
		if !almostEqual(result.MACD[i], expected, tol) {
			t.Errorf("MACD line [%d] expected %f, got %f", i, expected, result.MACD[i])
		}
	}
}

func TestMACD_HistogramEqualsMACD_MinusSignal(t *testing.T) {
	closes := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	result := MACD(closes, 5, 10, 3)

	for i := range closes {
		expected := result.MACD[i] - result.Signal[i]
		if !almostEqual(result.Histogram[i], expected, tol) {
			t.Errorf("MACD histogram [%d] expected %f, got %f", i, expected, result.Histogram[i])
		}
	}
}

func TestMACD_SignalIsEMAOfMACDLine(t *testing.T) {
	closes := []float64{5, 10, 15, 20, 25, 30, 35, 40, 45, 50}
	result := MACD(closes, 3, 5, 3)
	signalExpected := EMA(result.MACD, 3)
	for i := range closes {
		if !almostEqual(result.Signal[i], signalExpected[i], tol) {
			t.Errorf("MACD signal [%d] expected %f, got %f", i, signalExpected[i], result.Signal[i])
		}
	}
}

// ── Bollinger Bands ──────────────────────────────────────────────────────────

func TestBollingerBands_MiddleEqualsSMA(t *testing.T) {
	closes := []float64{10, 20, 30, 40, 50, 60, 70}
	period := 3
	bb := BollingerBands(closes, period, 2.0)
	sma := SMA(closes, period)

	for i := range closes {
		if math.IsNaN(sma[i]) && math.IsNaN(bb.Middle[i]) {
			continue
		}
		if !almostEqual(bb.Middle[i], sma[i], tol) {
			t.Errorf("BB middle [%d] expected %f, got %f", i, sma[i], bb.Middle[i])
		}
	}
}

func TestBollingerBands_UpperGreaterMiddleGreaterLower(t *testing.T) {
	closes := []float64{10, 20, 30, 40, 50, 60, 70}
	bb := BollingerBands(closes, 3, 2.0)

	for i := 2; i < len(closes); i++ {
		if bb.Upper[i] <= bb.Middle[i] {
			t.Errorf("BB[%d] upper (%f) should be > middle (%f)", i, bb.Upper[i], bb.Middle[i])
		}
		if bb.Middle[i] <= bb.Lower[i] {
			t.Errorf("BB[%d] middle (%f) should be > lower (%f)", i, bb.Middle[i], bb.Lower[i])
		}
	}
}

func TestBollingerBands_ConstantData(t *testing.T) {
	closes := []float64{5, 5, 5, 5, 5}
	bb := BollingerBands(closes, 3, 2.0)
	// For constant data, std dev = 0, so upper = middle = lower.
	for i := 2; i < len(closes); i++ {
		if !almostEqual(bb.Upper[i], bb.Middle[i], tol) {
			t.Errorf("BB constant [%d] upper (%f) should equal middle (%f)", i, bb.Upper[i], bb.Middle[i])
		}
		if !almostEqual(bb.Lower[i], bb.Middle[i], tol) {
			t.Errorf("BB constant [%d] lower (%f) should equal middle (%f)", i, bb.Lower[i], bb.Middle[i])
		}
	}
}

func TestBollingerBands_WarmupIsNaN(t *testing.T) {
	closes := []float64{10, 20, 30, 40}
	bb := BollingerBands(closes, 3, 2.0)
	for i := 0; i < 2; i++ {
		if !math.IsNaN(bb.Upper[i]) || !math.IsNaN(bb.Lower[i]) {
			t.Errorf("BB[%d] expected NaN during warmup", i)
		}
	}
}

// ── ATR ──────────────────────────────────────────────────────────────────────

func TestATR_FirstBarTR(t *testing.T) {
	bars := []data.OHLCV{
		{High: 50, Low: 40, Close: 45},
		{High: 52, Low: 41, Close: 48},
		{High: 53, Low: 42, Close: 50},
		{High: 55, Low: 43, Close: 52},
	}
	result := ATR(bars, 2)
	// First ATR value at index period-1 = 1
	// TR[0] = 50 - 40 = 10
	// TR[1] = max(52-41, |52-45|, |41-45|) = max(11, 7, 4) = 11
	// ATR[1] = (10 + 11) / 2 = 10.5
	if !almostEqual(result[1], 10.5, tol) {
		t.Errorf("ATR[1] expected 10.5, got %f", result[1])
	}
}

func TestATR_WarmupIsNaN(t *testing.T) {
	bars := []data.OHLCV{
		{High: 50, Low: 40, Close: 45},
		{High: 52, Low: 41, Close: 48},
		{High: 53, Low: 42, Close: 50},
	}
	result := ATR(bars, 3)
	// Indices 0 and 1 should be NaN.
	for i := 0; i < 2; i++ {
		if !math.IsNaN(result[i]) {
			t.Errorf("ATR[%d] expected NaN, got %f", i, result[i])
		}
	}
	if math.IsNaN(result[2]) {
		t.Error("ATR[2] expected a valid number, got NaN")
	}
}

func TestATR_EmptyBars(t *testing.T) {
	result := ATR([]data.OHLCV{}, 3)
	if len(result) != 0 {
		t.Errorf("ATR empty expected len 0, got %d", len(result))
	}
}

// ── Supertrend ───────────────────────────────────────────────────────────────

func TestSupertrend_StrongUptrend(t *testing.T) {
	// Create a strong uptrend: prices steadily rise.
	bars := make([]data.OHLCV, 20)
	for i := 0; i < 20; i++ {
		base := float64(100 + i*5)
		bars[i] = data.OHLCV{
			Open:  base,
			High:  base + 2,
			Low:   base - 2,
			Close: base + 1,
		}
	}
	result := Supertrend(bars, 5, 2.0)
	// After warmup, a strong uptrend should yield direction = 1.
	for i := 10; i < 20; i++ {
		if result.Direction[i] != 1 {
			t.Errorf("Supertrend direction [%d] expected 1 (bullish), got %d", i, result.Direction[i])
		}
	}
}

func TestSupertrend_DirectionChanges(t *testing.T) {
	// Go up then sharply down.
	bars := make([]data.OHLCV, 30)
	for i := 0; i < 15; i++ {
		base := float64(100 + i*5)
		bars[i] = data.OHLCV{
			Open: base, High: base + 2, Low: base - 2, Close: base + 1,
		}
	}
	for i := 15; i < 30; i++ {
		base := float64(175 - (i-15)*5)
		bars[i] = data.OHLCV{
			Open: base, High: base + 2, Low: base - 2, Close: base - 1,
		}
	}
	result := Supertrend(bars, 5, 2.0)

	// Verify that direction is not constant -- there should be at least one flip.
	seenPositive := false
	seenNegative := false
	for i := 5; i < 30; i++ {
		if result.Direction[i] == 1 {
			seenPositive = true
		}
		if result.Direction[i] == -1 {
			seenNegative = true
		}
	}
	if !seenPositive || !seenNegative {
		t.Error("Supertrend expected direction changes between bullish and bearish")
	}
}

// ── Stochastic ───────────────────────────────────────────────────────────────

func TestStochastic_CloseAtHigh(t *testing.T) {
	// When close equals the highest high, K should be 100.
	bars := []data.OHLCV{
		{High: 50, Low: 30, Close: 40},
		{High: 50, Low: 30, Close: 45},
		{High: 50, Low: 30, Close: 50}, // close = high
	}
	result := Stochastic(bars, 3, 1)
	// K[2] = (50-30)/(50-30)*100 = 100
	if !almostEqual(result.K[2], 100, tol) {
		t.Errorf("Stochastic K at high expected 100, got %f", result.K[2])
	}
}

func TestStochastic_CloseAtLow(t *testing.T) {
	// When close equals the lowest low, K should be 0.
	bars := []data.OHLCV{
		{High: 50, Low: 30, Close: 40},
		{High: 50, Low: 30, Close: 35},
		{High: 50, Low: 30, Close: 30}, // close = low
	}
	result := Stochastic(bars, 3, 1)
	// K[2] = (30-30)/(50-30)*100 = 0
	if !almostEqual(result.K[2], 0, tol) {
		t.Errorf("Stochastic K at low expected 0, got %f", result.K[2])
	}
}

func TestStochastic_WarmupIsNaN(t *testing.T) {
	bars := []data.OHLCV{
		{High: 50, Low: 30, Close: 40},
		{High: 55, Low: 32, Close: 45},
		{High: 60, Low: 35, Close: 50},
		{High: 58, Low: 33, Close: 48},
	}
	result := Stochastic(bars, 3, 2)
	// K warmup: indices 0,1 should be NaN
	for i := 0; i < 2; i++ {
		if !math.IsNaN(result.K[i]) {
			t.Errorf("Stochastic K[%d] expected NaN, got %f", i, result.K[i])
		}
	}
	// D warmup: kPeriod-1 + dPeriod-1 = 2 + 1 = 3, so indices 0,1,2 should be NaN
	for i := 0; i < 3; i++ {
		if !math.IsNaN(result.D[i]) {
			t.Errorf("Stochastic D[%d] expected NaN, got %f", i, result.D[i])
		}
	}
}

func TestStochastic_DIsAverageOfK(t *testing.T) {
	bars := []data.OHLCV{
		{High: 50, Low: 30, Close: 40},
		{High: 55, Low: 32, Close: 45},
		{High: 60, Low: 35, Close: 50},
		{High: 58, Low: 33, Close: 48},
		{High: 62, Low: 36, Close: 55},
	}
	result := Stochastic(bars, 3, 2)
	// D[i] should be the average of K[i-1] and K[i] (dPeriod=2) for valid indices.
	for i := 3; i < len(bars); i++ {
		if math.IsNaN(result.D[i]) {
			continue
		}
		expectedD := (result.K[i-1] + result.K[i]) / 2.0
		if !almostEqual(result.D[i], expectedD, tol) {
			t.Errorf("Stochastic D[%d] expected %f, got %f", i, expectedD, result.D[i])
		}
	}
}

// ── Donchian ─────────────────────────────────────────────────────────────────

func TestDonchian_UpperIsHighestHigh(t *testing.T) {
	bars := []data.OHLCV{
		{High: 50, Low: 30, Close: 40},
		{High: 55, Low: 32, Close: 45},
		{High: 60, Low: 35, Close: 50},
		{High: 52, Low: 33, Close: 48},
	}
	result := Donchian(bars, 3)
	// At index 2: highest high of bars[0..2] = 60
	if !almostEqual(result.Upper[2], 60, tol) {
		t.Errorf("Donchian upper[2] expected 60, got %f", result.Upper[2])
	}
	// At index 3: highest high of bars[1..3] = 60
	if !almostEqual(result.Upper[3], 60, tol) {
		t.Errorf("Donchian upper[3] expected 60, got %f", result.Upper[3])
	}
}

func TestDonchian_LowerIsLowestLow(t *testing.T) {
	bars := []data.OHLCV{
		{High: 50, Low: 30, Close: 40},
		{High: 55, Low: 32, Close: 45},
		{High: 60, Low: 35, Close: 50},
		{High: 52, Low: 33, Close: 48},
	}
	result := Donchian(bars, 3)
	// At index 2: lowest low of bars[0..2] = 30
	if !almostEqual(result.Lower[2], 30, tol) {
		t.Errorf("Donchian lower[2] expected 30, got %f", result.Lower[2])
	}
	// At index 3: lowest low of bars[1..3] = 32
	if !almostEqual(result.Lower[3], 32, tol) {
		t.Errorf("Donchian lower[3] expected 32, got %f", result.Lower[3])
	}
}

func TestDonchian_MiddleIsAverage(t *testing.T) {
	bars := []data.OHLCV{
		{High: 50, Low: 30, Close: 40},
		{High: 55, Low: 32, Close: 45},
		{High: 60, Low: 35, Close: 50},
	}
	result := Donchian(bars, 3)
	expectedMiddle := (60.0 + 30.0) / 2.0 // 45
	if !almostEqual(result.Middle[2], expectedMiddle, tol) {
		t.Errorf("Donchian middle[2] expected %f, got %f", expectedMiddle, result.Middle[2])
	}
}

func TestDonchian_WarmupIsNaN(t *testing.T) {
	bars := []data.OHLCV{
		{High: 50, Low: 30, Close: 40},
		{High: 55, Low: 32, Close: 45},
		{High: 60, Low: 35, Close: 50},
	}
	result := Donchian(bars, 3)
	for i := 0; i < 2; i++ {
		if !math.IsNaN(result.Upper[i]) || !math.IsNaN(result.Lower[i]) || !math.IsNaN(result.Middle[i]) {
			t.Errorf("Donchian[%d] expected NaN during warmup", i)
		}
	}
}

// ── ExtractClose ─────────────────────────────────────────────────────────────

func TestExtractClose(t *testing.T) {
	bars := []data.OHLCV{
		{Close: 10},
		{Close: 20},
		{Close: 30},
	}
	result := ExtractClose(bars)
	expected := []float64{10, 20, 30}
	for i, exp := range expected {
		if !almostEqual(result[i], exp, tol) {
			t.Errorf("ExtractClose[%d] expected %f, got %f", i, exp, result[i])
		}
	}
}

func TestExtractClose_Empty(t *testing.T) {
	result := ExtractClose([]data.OHLCV{})
	if len(result) != 0 {
		t.Errorf("ExtractClose empty expected len 0, got %d", len(result))
	}
}

// ── Last ─────────────────────────────────────────────────────────────────────

func TestLast_NormalSlice(t *testing.T) {
	s := []float64{1, 2, 3, 4, 5}
	if v := Last(s); !almostEqual(v, 5, tol) {
		t.Errorf("Last expected 5, got %f", v)
	}
}

func TestLast_WithTrailingNaN(t *testing.T) {
	s := []float64{1, 2, 3, math.NaN(), math.NaN()}
	if v := Last(s); !almostEqual(v, 3, tol) {
		t.Errorf("Last expected 3, got %f", v)
	}
}

func TestLast_AllNaN(t *testing.T) {
	s := []float64{math.NaN(), math.NaN()}
	if v := Last(s); !math.IsNaN(v) {
		t.Errorf("Last all-NaN expected NaN, got %f", v)
	}
}

func TestLast_EmptySlice(t *testing.T) {
	if v := Last([]float64{}); !math.IsNaN(v) {
		t.Errorf("Last empty expected NaN, got %f", v)
	}
}
