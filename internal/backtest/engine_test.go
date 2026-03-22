package backtest

import (
	"math"
	"strings"
	"testing"
	"time"
	"trading-backtest-bot/internal/data"
)

// ── Helpers ──────────────────────────────────────────────────────────────────

// generateBars creates synthetic OHLCV bars with a linear trend.
func generateBars(n int, startPrice float64, trend float64) []data.OHLCV {
	bars := make([]data.OHLCV, n)
	price := startPrice
	for i := range bars {
		bars[i] = data.OHLCV{
			Time:   time.Date(2024, 1, 1+i, 0, 0, 0, 0, time.UTC),
			Open:   price,
			High:   price * 1.01,
			Low:    price * 0.99,
			Close:  price + trend,
			Volume: 1000,
		}
		price = bars[i].Close
	}
	return bars
}

// alternatingStrategy buys on even-indexed bars and sells on odd-indexed bars.
type alternatingStrategy struct{}

func (s *alternatingStrategy) Name() string                                    { return "Alternating" }
func (s *alternatingStrategy) Description() string                             { return "buy even, sell odd" }
func (s *alternatingStrategy) Init(_ []data.OHLCV, _ map[string]float64)       {}
func (s *alternatingStrategy) Signal(i int) SignalType {
	if i%2 == 0 {
		return BuySignal
	}
	return SellSignal
}

// alwaysBuyStrategy emits a BuySignal on every bar.
type alwaysBuyStrategy struct{}

func (s *alwaysBuyStrategy) Name() string                                    { return "AlwaysBuy" }
func (s *alwaysBuyStrategy) Description() string                             { return "always buy" }
func (s *alwaysBuyStrategy) Init(_ []data.OHLCV, _ map[string]float64)       {}
func (s *alwaysBuyStrategy) Signal(i int) SignalType                          { return BuySignal }

// defaultCfg returns a basic engine config suitable for most tests.
func defaultCfg() Config {
	return Config{
		InitialCapital:  10000,
		PositionSizePct: 1.0,
		Commission:      0,
		Slippage:        0,
		StopLossPct:     0,
		TakeProfitPct:   0,
		Symbol:          "TEST",
		Interval:        "1d",
	}
}

// ── Tests ────────────────────────────────────────────────────────────────────

func TestEngine_NotEnoughData(t *testing.T) {
	for _, n := range []int{0, 1} {
		eng := NewEngine(defaultCfg())
		eng.LoadData(generateBars(n, 100, 1))
		eng.SetStrategy(&alternatingStrategy{})
		_, err := eng.Run(nil)
		if err == nil {
			t.Errorf("expected error with %d bars, got nil", n)
		}
	}
}

func TestEngine_NoStrategy(t *testing.T) {
	eng := NewEngine(defaultCfg())
	eng.LoadData(generateBars(50, 100, 1))
	// strategy intentionally not set
	_, err := eng.Run(nil)
	if err == nil {
		t.Fatal("expected error when no strategy set, got nil")
	}
}

func TestEngine_BasicLongTrade(t *testing.T) {
	// Strong uptrend: price starts at 100, gains 1.0 per bar over 100 bars.
	bars := generateBars(100, 100, 1.0)

	cfg := defaultCfg()
	eng := NewEngine(cfg)
	eng.LoadData(bars)
	eng.SetStrategy(&EMACrossStrategy{})

	res, err := eng.Run(map[string]float64{"fast": 5, "slow": 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.TotalTrades < 1 {
		t.Fatal("expected at least 1 trade in a clear uptrend")
	}

	hasLong := false
	for _, tr := range res.Trades {
		if tr.Direction == Long {
			hasLong = true
			break
		}
	}
	if !hasLong {
		t.Error("expected at least one Long trade in uptrend")
	}

	if res.FinalCapital <= cfg.InitialCapital {
		t.Errorf("expected profit in clear uptrend, got FinalCapital=%.2f (initial=%.2f)",
			res.FinalCapital, cfg.InitialCapital)
	}
}

func TestEngine_SlippageDirection(t *testing.T) {
	bars := generateBars(100, 100, 0.5)

	// Run with zero slippage
	cfg0 := defaultCfg()
	eng0 := NewEngine(cfg0)
	eng0.LoadData(bars)
	eng0.SetStrategy(&alternatingStrategy{})
	res0, err := eng0.Run(nil)
	if err != nil {
		t.Fatalf("zero-slippage run: %v", err)
	}

	// Run with high slippage
	cfgS := defaultCfg()
	cfgS.Slippage = 2.0
	engS := NewEngine(cfgS)
	engS.LoadData(bars)
	engS.SetStrategy(&alternatingStrategy{})
	resS, err := engS.Run(nil)
	if err != nil {
		t.Fatalf("slippage run: %v", err)
	}

	if resS.FinalCapital >= res0.FinalCapital {
		t.Errorf("slippage should reduce profitability: slippage=%.2f, no-slippage=%.2f",
			resS.FinalCapital, res0.FinalCapital)
	}
}

func TestEngine_Commission(t *testing.T) {
	bars := generateBars(100, 100, 0.5)

	// Run with zero commission
	cfg0 := defaultCfg()
	eng0 := NewEngine(cfg0)
	eng0.LoadData(bars)
	eng0.SetStrategy(&alternatingStrategy{})
	res0, err := eng0.Run(nil)
	if err != nil {
		t.Fatalf("zero-commission run: %v", err)
	}

	// Run with high commission
	cfgC := defaultCfg()
	cfgC.Commission = 100
	engC := NewEngine(cfgC)
	engC.LoadData(bars)
	engC.SetStrategy(&alternatingStrategy{})
	resC, err := engC.Run(nil)
	if err != nil {
		t.Fatalf("commission run: %v", err)
	}

	if resC.TotalPnL >= res0.TotalPnL {
		t.Errorf("commission should reduce PnL: with=%.2f, without=%.2f",
			resC.TotalPnL, res0.TotalPnL)
	}
}

func TestEngine_StopLoss(t *testing.T) {
	// Create bars that dip enough to trigger a tight stop loss.
	// Alternating strategy buys on even bars. We need the low to hit the SL.
	// With 1% SL, Low must be <= entryPrice * 0.99.
	// generateBars sets Low = price * 0.99, so a 1% SL should be triggered
	// when the price drops even slightly.
	bars := generateBars(100, 100, -0.2) // slight downtrend

	cfg := defaultCfg()
	cfg.StopLossPct = 0.005 // 0.5% stop loss (very tight)
	eng := NewEngine(cfg)
	eng.LoadData(bars)
	eng.SetStrategy(&alternatingStrategy{})

	res, err := eng.Run(nil)
	if err != nil {
		t.Fatalf("stop-loss run: %v", err)
	}

	hasSL := false
	for _, tr := range res.Trades {
		if tr.Reason == "SL" {
			hasSL = true
			break
		}
	}
	if !hasSL {
		t.Error("expected at least one trade to exit with SL reason")
	}
}

func TestEngine_TakeProfit(t *testing.T) {
	// Strong uptrend with tight TP should trigger take profit for long trades.
	bars := generateBars(100, 100, 1.0)

	cfg := defaultCfg()
	cfg.TakeProfitPct = 0.005 // 0.5% take profit (very tight)
	eng := NewEngine(cfg)
	eng.LoadData(bars)
	eng.SetStrategy(&alternatingStrategy{})

	res, err := eng.Run(nil)
	if err != nil {
		t.Fatalf("take-profit run: %v", err)
	}

	hasTP := false
	for _, tr := range res.Trades {
		if tr.Reason == "TP" {
			hasTP = true
			break
		}
	}
	if !hasTP {
		t.Error("expected at least one trade to exit with TP reason")
	}
}

func TestEngine_EquityCurveLength(t *testing.T) {
	n := 50
	bars := generateBars(n, 100, 0.5)

	eng := NewEngine(defaultCfg())
	eng.LoadData(bars)
	eng.SetStrategy(&alternatingStrategy{})

	res, err := eng.Run(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Equity curve: 1 initial entry + (n-1) bar updates = n entries
	expected := n
	if len(res.EquityCurve) != expected {
		t.Errorf("expected equity curve length %d, got %d", expected, len(res.EquityCurve))
	}
}

func TestEngine_MarkToMarket(t *testing.T) {
	// Use alwaysBuy so a position stays open for multiple bars.
	// Equity curve should change between bars even without closing trades.
	bars := generateBars(20, 100, 1.0)

	eng := NewEngine(defaultCfg())
	eng.LoadData(bars)
	eng.SetStrategy(&alwaysBuyStrategy{})

	res, err := eng.Run(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The equity curve should not be flat while a position is open.
	allSame := true
	first := res.EquityCurve[0]
	for _, eq := range res.EquityCurve[1:] {
		if eq != first {
			allSame = false
			break
		}
	}
	if allSame {
		t.Error("equity curve is flat; expected mark-to-market changes while position is open")
	}
}

func TestEngine_DrawdownCalculation(t *testing.T) {
	// Create a scenario: uptrend then downtrend to produce a drawdown.
	up := generateBars(30, 100, 1.0)
	down := generateBars(30, up[len(up)-1].Close, -1.5)
	bars := append(up, down[1:]...) // skip duplicate bar at junction

	cfg := defaultCfg()
	eng := NewEngine(cfg)
	eng.LoadData(bars)
	eng.SetStrategy(&alwaysBuyStrategy{})

	res, err := eng.Run(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.MaxDrawdown <= 0 {
		t.Error("expected MaxDrawdown > 0 for up-then-down scenario")
	}
	if res.MaxDrawdownPct <= 0 {
		t.Error("expected MaxDrawdownPct > 0")
	}

	// MaxDrawdownPct should be relative to peak equity, not initial capital.
	// Verify: pct = MaxDrawdown / peakEquity * 100. Since equity grew above initial,
	// the peak is > initial, so pct should be < (MaxDrawdown / initial * 100).
	pctRelativeToInitial := res.MaxDrawdown / cfg.InitialCapital * 100
	if res.MaxDrawdownPct >= pctRelativeToInitial && res.MaxDrawdown > 0 {
		// Only fail if peak was actually above initial (which it should be after uptrend)
		peakAboveInitial := false
		for _, eq := range res.EquityCurve {
			if eq > cfg.InitialCapital {
				peakAboveInitial = true
				break
			}
		}
		if peakAboveInitial {
			t.Errorf("MaxDrawdownPct (%.4f) should be relative to peak, not initial capital (%.4f)",
				res.MaxDrawdownPct, pctRelativeToInitial)
		}
	}
}

func TestFormatResult(t *testing.T) {
	bars := generateBars(50, 100, 0.5)
	cfg := defaultCfg()
	eng := NewEngine(cfg)
	eng.LoadData(bars)
	eng.SetStrategy(&alternatingStrategy{})

	res, err := eng.Run(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := FormatResult(res, cfg.InitialCapital)

	for _, substr := range []string{"Backtest Result", "Symbol", "Win Rate", "Sharpe", "Drawdown"} {
		if !strings.Contains(output, substr) {
			t.Errorf("FormatResult output missing expected substring %q", substr)
		}
	}
}

func TestTopTrades(t *testing.T) {
	trades := []Trade{
		{PnL: 500},
		{PnL: -200},
		{PnL: 300},
		{PnL: -100},
		{PnL: 1000},
	}
	r := &Result{Trades: trades}

	best, worst := TopTrades(r, 2)

	if len(best) != 2 {
		t.Fatalf("expected 2 best trades, got %d", len(best))
	}
	if len(worst) != 2 {
		t.Fatalf("expected 2 worst trades, got %d", len(worst))
	}

	// Best trades should be the highest PnL
	if best[0].PnL != 1000 {
		t.Errorf("expected best[0].PnL=1000, got %.2f", best[0].PnL)
	}
	if best[1].PnL != 500 {
		t.Errorf("expected best[1].PnL=500, got %.2f", best[1].PnL)
	}

	// Worst trades should be the lowest PnL
	if worst[0].PnL != -100 {
		t.Errorf("expected worst[0].PnL=-100, got %.2f", worst[0].PnL)
	}
	if worst[1].PnL != -200 {
		t.Errorf("expected worst[1].PnL=-200, got %.2f", worst[1].PnL)
	}
}

func TestComputeSharpe(t *testing.T) {
	// With < 2 trades, should return 0.
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	result := computeSharpe([]Trade{{PnLPct: 5}}, start, end)
	if result != 0 {
		t.Errorf("expected Sharpe=0 for <2 trades, got %.4f", result)
	}

	result = computeSharpe(nil, start, end)
	if result != 0 {
		t.Errorf("expected Sharpe=0 for nil trades, got %.4f", result)
	}

	// With known trades, should return non-zero.
	trades := []Trade{
		{PnLPct: 5.0},
		{PnLPct: 3.0},
		{PnLPct: -2.0},
		{PnLPct: 4.0},
		{PnLPct: 1.0},
	}
	sharpe := computeSharpe(trades, start, end)
	if sharpe == 0 {
		t.Error("expected non-zero Sharpe ratio for trades with variance")
	}
	if math.IsNaN(sharpe) || math.IsInf(sharpe, 0) {
		t.Errorf("Sharpe ratio should be finite, got %v", sharpe)
	}
}
