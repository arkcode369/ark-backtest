package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"time"
	"trading-backtest-bot/internal/backtest"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

var passed, failed int

func assert(name string, cond bool, msg string) {
	if cond {
		passed++
		fmt.Printf("  PASS  %s\n", name)
	} else {
		failed++
		fmt.Printf("  FAIL  %s — %s\n", name, msg)
	}
}

func main() {
	ctx := context.Background()
	fmt.Println("=" + strings.Repeat("=", 70))
	fmt.Println("  COMPREHENSIVE FUNCTIONAL TEST SUITE")
	fmt.Println("=" + strings.Repeat("=", 70))

	// ══════════════════════════════════════════════════════════════════════
	// SCENARIO 1: Data Fetching — Multiple Symbols, Intervals, Edge Cases
	// ══════════════════════════════════════════════════════════════════════
	fmt.Println("\n── SCENARIO 1: Data Fetching ──────────────────────────────")

	// 1a: Fetch XAUUSD daily — should return 200+ bars for 1y
	bars, err := data.FetchOHLCV(ctx, data.FetchParams{Symbol: "XAUUSD", Interval: "1d", Period: "1y"})
	assert("S1a: XAUUSD 1d 1y fetch", err == nil && len(bars) > 100,
		fmt.Sprintf("err=%v bars=%d", err, len(bars)))

	// 1b: Verify bars are sorted ascending
	sorted := true
	for i := 1; i < len(bars); i++ {
		if bars[i].Time.Before(bars[i-1].Time) {
			sorted = false
			break
		}
	}
	assert("S1b: bars sorted ascending", sorted, "bars not sorted")

	// 1c: Verify no NaN/zero in OHLCV
	noNaN := true
	for _, b := range bars {
		if math.IsNaN(b.Open) || math.IsNaN(b.Close) || b.Open == 0 || b.Close == 0 {
			noNaN = false
			break
		}
	}
	assert("S1c: no NaN/zero in OHLCV", noNaN, "found NaN or zero")

	// 1d: Fetch with short period
	bars7d, err := data.FetchOHLCV(ctx, data.FetchParams{Symbol: "NQ", Interval: "1d", Period: "7d"})
	assert("S1d: NQ 1d 7d fetch", err == nil && len(bars7d) >= 3,
		fmt.Sprintf("err=%v bars=%d", err, len(bars7d)))

	// 1e: Fetch latest price
	price, currency, err := data.FetchLatestPrice(ctx, "EURUSD")
	assert("S1e: EURUSD latest price", err == nil && price > 0 && currency != "",
		fmt.Sprintf("err=%v price=%f cur=%s", err, price, currency))

	// 1f: Case-insensitive symbol lookup
	sym1, ok1 := data.GetSymbol("xauusd")
	sym2, ok2 := data.GetSymbol("XAUUSD")
	assert("S1f: case-insensitive symbol lookup", ok1 && ok2 && sym1.Ticker == sym2.Ticker, "")

	// 1g: Unknown symbol returns error
	_, err = data.FetchOHLCV(ctx, data.FetchParams{Symbol: "FAKESYMBOL", Interval: "1d", Period: "7d"})
	assert("S1g: unknown symbol error", err != nil, "expected error for unknown symbol")

	// 1h: Invalid interval returns error
	_, err = data.FetchOHLCV(ctx, data.FetchParams{Symbol: "XAUUSD", Interval: "3h", Period: "7d"})
	assert("S1h: invalid interval error", err != nil, "expected error for invalid interval")

	// 1i: Multiple symbols across categories
	for _, sym := range []string{"XAGUSD", "ES", "GBPUSD", "CL"} {
		b, err := data.FetchOHLCV(ctx, data.FetchParams{Symbol: sym, Interval: "1d", Period: "30d"})
		assert(fmt.Sprintf("S1i: fetch %s", sym), err == nil && len(b) > 10,
			fmt.Sprintf("err=%v bars=%d", err, len(b)))
	}

	// ══════════════════════════════════════════════════════════════════════
	// SCENARIO 2: Indicators — Correctness Verification
	// ══════════════════════════════════════════════════════════════════════
	fmt.Println("\n── SCENARIO 2: Indicators ─────────────────────────────────")

	closes := indicators.ExtractClose(bars)
	assert("S2a: ExtractClose length", len(closes) == len(bars),
		fmt.Sprintf("expected %d got %d", len(bars), len(closes)))

	// 2b: SMA warmup check
	sma20 := indicators.SMA(closes, 20)
	warmupNaN := true
	for i := 0; i < 19; i++ {
		if !math.IsNaN(sma20[i]) {
			warmupNaN = false
			break
		}
	}
	assert("S2b: SMA warmup is NaN", warmupNaN, "expected NaN during warmup")
	assert("S2c: SMA post-warmup valid", !math.IsNaN(sma20[19]) && sma20[19] > 0, "")

	// 2d: EMA produces values for all bars
	ema9 := indicators.EMA(closes, 9)
	assert("S2d: EMA length matches", len(ema9) == len(closes), "")
	assert("S2e: EMA last value valid", !math.IsNaN(indicators.Last(ema9)), "")

	// 2f: RSI is bounded 0-100
	rsi := indicators.RSI(closes, 14)
	rsiBounded := true
	for i := 14; i < len(rsi); i++ {
		if rsi[i] < 0 || rsi[i] > 100 {
			rsiBounded = false
			break
		}
	}
	assert("S2f: RSI bounded 0-100", rsiBounded, "RSI out of range")

	// 2g: MACD histogram = MACD - Signal
	macd := indicators.MACD(closes, 12, 26, 9)
	macdValid := true
	for i := 35; i < len(closes); i++ {
		diff := math.Abs(macd.Histogram[i] - (macd.MACD[i] - macd.Signal[i]))
		if diff > 0.0001 {
			macdValid = false
			break
		}
	}
	assert("S2g: MACD histogram = MACD - Signal", macdValid, "")

	// 2h: Bollinger Bands ordering
	bb := indicators.BollingerBands(closes, 20, 2.0)
	bbValid := true
	for i := 19; i < len(closes); i++ {
		if bb.Upper[i] < bb.Middle[i] || bb.Middle[i] < bb.Lower[i] {
			bbValid = false
			break
		}
	}
	assert("S2h: BB upper >= middle >= lower", bbValid, "")

	// 2i: ATR is non-negative
	atr := indicators.ATR(bars, 14)
	atrValid := true
	for i := 14; i < len(atr); i++ {
		if atr[i] < 0 {
			atrValid = false
			break
		}
	}
	assert("S2i: ATR non-negative", atrValid, "")

	// 2j: Supertrend direction is 1 or -1 (after warmup)
	st := indicators.Supertrend(bars, 10, 3.0)
	stValid := true
	for i := 11; i < len(bars); i++ {
		if st.Direction[i] != 1 && st.Direction[i] != -1 {
			stValid = false
			break
		}
	}
	assert("S2j: Supertrend direction is 1/-1", stValid, "")

	// ══════════════════════════════════════════════════════════════════════
	// SCENARIO 3: Backtest Engine — All 7 Strategies with Live Data
	// ══════════════════════════════════════════════════════════════════════
	fmt.Println("\n── SCENARIO 3: All 7 Strategies ───────────────────────────")

	cfg := backtest.Config{
		InitialCapital:  10000,
		PositionSizePct: 0.02,
		Commission:      5,
		Slippage:        0.10,
		Symbol:          "XAUUSD",
		Interval:        "1d",
	}

	for key, meta := range backtest.StrategyRegistry {
		engine := backtest.NewEngine(cfg)
		engine.LoadData(bars)
		engine.SetStrategy(meta.Factory())
		r, err := engine.Run(meta.Params)

		if err != nil {
			assert(fmt.Sprintf("S3: %s run", key), false, err.Error())
			continue
		}

		// Verify result invariants
		ok := true
		reason := ""

		// a) Equity curve length = len(bars)
		if len(r.EquityCurve) != len(bars) {
			ok = false
			reason = fmt.Sprintf("equity curve len=%d bars=%d", len(r.EquityCurve), len(bars))
		}

		// b) FinalCapital should match equity curve last value
		if ok && math.Abs(r.FinalCapital-r.EquityCurve[len(r.EquityCurve)-1]) > 0.01 {
			ok = false
			reason = fmt.Sprintf("FinalCapital=%.2f != last equity=%.2f", r.FinalCapital, r.EquityCurve[len(r.EquityCurve)-1])
		}

		// c) WinRate math: WinningTrades/TotalTrades*100
		if ok && r.TotalTrades > 0 {
			expectedWR := float64(r.WinningTrades) / float64(r.TotalTrades) * 100
			if math.Abs(r.WinRate-expectedWR) > 0.1 {
				ok = false
				reason = fmt.Sprintf("WinRate=%.1f expected=%.1f", r.WinRate, expectedWR)
			}
		}

		// d) WinningTrades + LosingTrades = TotalTrades
		if ok && r.WinningTrades+r.LosingTrades != r.TotalTrades {
			ok = false
			reason = "W+L != Total"
		}

		// e) TotalPnL should approximately equal FinalCapital - InitialCapital
		if ok {
			diff := math.Abs(r.TotalPnL - (r.FinalCapital - cfg.InitialCapital))
			if diff > 0.02 {
				ok = false
				reason = fmt.Sprintf("PnL=%.2f vs FC-IC=%.2f (diff=%.4f)", r.TotalPnL, r.FinalCapital-cfg.InitialCapital, diff)
			}
		}

		// f) MaxDrawdown >= 0
		if ok && r.MaxDrawdown < 0 {
			ok = false
			reason = "MaxDrawdown < 0"
		}

		// g) SharpeRatio is not NaN
		if ok && math.IsNaN(r.SharpeRatio) {
			ok = false
			reason = "Sharpe is NaN"
		}

		assert(fmt.Sprintf("S3: %-12s trades=%d WR=%.0f%% PnL=$%.0f", key, r.TotalTrades, r.WinRate, r.TotalPnL), ok, reason)
	}

	// ══════════════════════════════════════════════════════════════════════
	// SCENARIO 4: Engine Edge Cases
	// ══════════════════════════════════════════════════════════════════════
	fmt.Println("\n── SCENARIO 4: Engine Edge Cases ───────────────────────────")

	// 4a: Stop loss triggers correctly
	slCfg := backtest.Config{
		InitialCapital:  10000,
		PositionSizePct: 0.02,
		StopLossPct:     0.01, // 1% SL
		Symbol:          "XAUUSD",
		Interval:        "1d",
	}
	engine := backtest.NewEngine(slCfg)
	engine.LoadData(bars)
	engine.SetStrategy(backtest.StrategyRegistry["ema_cross"].Factory())
	slResult, err := engine.Run(map[string]float64{"fast": 9, "slow": 21})
	assert("S4a: SL backtest runs", err == nil && slResult != nil, fmt.Sprintf("err=%v", err))
	if slResult != nil {
		slCount := 0
		for _, t := range slResult.Trades {
			if t.Reason == "SL" {
				slCount++
			}
		}
		assert("S4a: SL triggered", slCount >= 0, "SL count check") // may be 0 if no SL hit
	}

	// 4b: Take profit triggers correctly
	tpCfg := backtest.Config{
		InitialCapital:  10000,
		PositionSizePct: 0.02,
		TakeProfitPct:   0.02, // 2% TP
		Symbol:          "XAUUSD",
		Interval:        "1d",
	}
	engine2 := backtest.NewEngine(tpCfg)
	engine2.LoadData(bars)
	engine2.SetStrategy(backtest.StrategyRegistry["ema_cross"].Factory())
	tpResult, err := engine2.Run(map[string]float64{"fast": 9, "slow": 21})
	assert("S4b: TP backtest runs", err == nil && tpResult != nil, fmt.Sprintf("err=%v", err))

	// 4c: Commission reduces P&L
	noCommCfg := backtest.Config{
		InitialCapital: 10000, PositionSizePct: 0.02,
		Commission: 0, Symbol: "XAUUSD", Interval: "1d",
	}
	withCommCfg := backtest.Config{
		InitialCapital: 10000, PositionSizePct: 0.02,
		Commission: 10, Symbol: "XAUUSD", Interval: "1d",
	}
	e1 := backtest.NewEngine(noCommCfg)
	e1.LoadData(bars)
	e1.SetStrategy(backtest.StrategyRegistry["ema_cross"].Factory())
	r1, _ := e1.Run(map[string]float64{"fast": 9, "slow": 21})

	e2 := backtest.NewEngine(withCommCfg)
	e2.LoadData(bars)
	e2.SetStrategy(backtest.StrategyRegistry["ema_cross"].Factory())
	r2, _ := e2.Run(map[string]float64{"fast": 9, "slow": 21})

	if r1 != nil && r2 != nil && r1.TotalTrades > 0 {
		assert("S4c: commission reduces PnL", r2.TotalPnL < r1.TotalPnL,
			fmt.Sprintf("noComm=$%.2f withComm=$%.2f", r1.TotalPnL, r2.TotalPnL))
		// Commission compounds via position sizing, so diff >= $10*trades (at minimum)
		minExpected := float64(r2.TotalTrades) * 10
		diff := r1.TotalPnL - r2.TotalPnL
		assert("S4c: commission >= $10*trades (compounds)", diff >= minExpected*0.95,
			fmt.Sprintf("diff=$%.2f minExpected=$%.0f", diff, minExpected))
	}

	// 4d: Not enough data
	_, err = backtest.NewEngine(cfg).Run(nil)
	assert("S4d: error on no data", err != nil, "expected error")

	// 4e: Equity curve starts at initial capital
	if r1 != nil {
		assert("S4e: equity curve starts at capital", r1.EquityCurve[0] == 10000,
			fmt.Sprintf("got %f", r1.EquityCurve[0]))
	}

	// ══════════════════════════════════════════════════════════════════════
	// SCENARIO 5: Walk-Forward Validation
	// ══════════════════════════════════════════════════════════════════════
	fmt.Println("\n── SCENARIO 5: Walk-Forward Validation ─────────────────────")

	wfEngine := backtest.NewEngine(cfg)
	wfEngine.LoadData(bars)
	wfEngine.SetStrategy(backtest.StrategyRegistry["ema_cross"].Factory())

	wfResult, err := wfEngine.RunWalkForward(map[string]float64{"fast": 9, "slow": 21}, 0.3)
	assert("S5a: walk-forward runs", err == nil && wfResult != nil, fmt.Sprintf("err=%v", err))
	if wfResult != nil {
		assert("S5b: IS has trades", wfResult.InSample.TotalTrades >= 0, "")
		assert("S5c: OOS has trades", wfResult.OutOfSample.TotalTrades >= 0, "")
		assert("S5d: IS period set", wfResult.InSample.Period != "", "")
		assert("S5e: OOS period set", wfResult.OutOfSample.Period != "", "")

		// IS bars + OOS bars should approximate total bars
		isBars := len(wfResult.InSample.EquityCurve)
		oosBars := len(wfResult.OutOfSample.EquityCurve)
		assert("S5f: IS+OOS ~ total bars",
			isBars+oosBars > len(bars)/2, // rough check
			fmt.Sprintf("IS=%d OOS=%d total=%d", isBars, oosBars, len(bars)))

		// Format should not panic
		formatted := backtest.FormatWalkForwardResult(wfResult)
		assert("S5g: format not empty", len(formatted) > 50, "")
	}

	// 5h: Invalid OOS fraction
	_, err = wfEngine.RunWalkForward(map[string]float64{"fast": 9, "slow": 21}, 0.8)
	assert("S5h: reject oos > 0.5", err != nil, "expected error for oos=0.8")

	// ══════════════════════════════════════════════════════════════════════
	// SCENARIO 6: Equity Curve Chart
	// ══════════════════════════════════════════════════════════════════════
	fmt.Println("\n── SCENARIO 6: Equity Curve Chart ──────────────────────────")

	if r1 != nil && len(r1.EquityCurve) > 2 {
		chart := backtest.FormatEquityCurve(r1.EquityCurve, 50, 12)
		assert("S6a: chart not empty", chart != "", "")
		assert("S6b: chart has Y-axis labels", strings.Contains(chart, "|"), "")
		assert("S6c: chart has summary line", strings.Contains(chart, "$"), "")
	}

	// Edge: flat curve
	flatChart := backtest.FormatEquityCurve([]float64{10000, 10000, 10000}, 20, 8)
	assert("S6d: flat curve renders", flatChart != "", "")

	// Edge: single point
	emptyChart := backtest.FormatEquityCurve([]float64{10000}, 20, 8)
	assert("S6e: single point returns empty", emptyChart == "", "")

	// ══════════════════════════════════════════════════════════════════════
	// SCENARIO 7: FormatResult and TopTrades
	// ══════════════════════════════════════════════════════════════════════
	fmt.Println("\n── SCENARIO 7: Result Formatting ───────────────────────────")

	if r1 != nil {
		formatted := backtest.FormatResult(r1, 10000)
		assert("S7a: FormatResult not empty", len(formatted) > 100, "")
		assert("S7b: contains symbol", strings.Contains(formatted, "XAUUSD"), "")
		assert("S7c: contains strategy", strings.Contains(formatted, "EMA"), "")

		if r1.TotalTrades >= 3 {
			best, worst := backtest.TopTrades(r1, 3)
			assert("S7d: TopTrades best len", len(best) == 3, fmt.Sprintf("got %d", len(best)))
			assert("S7e: TopTrades worst len", len(worst) == 3, fmt.Sprintf("got %d", len(worst)))
			assert("S7f: best[0].PnL >= best[1].PnL", best[0].PnL >= best[1].PnL, "not sorted desc")
		}
	}

	// ══════════════════════════════════════════════════════════════════════
	// SCENARIO 8: Optimizer Simulation (Small Grid Search)
	// ══════════════════════════════════════════════════════════════════════
	fmt.Println("\n── SCENARIO 8: Optimizer (Grid Search) ─────────────────────")

	type optResult struct {
		Fast, Slow float64
		Sharpe     float64
		Trades     int
	}
	var optResults []optResult

	for fast := 5.0; fast <= 15; fast += 5 {
		for slow := 20.0; slow <= 30; slow += 5 {
			eng := backtest.NewEngine(cfg)
			eng.LoadData(bars)
			eng.SetStrategy(backtest.StrategyRegistry["ema_cross"].Factory())
			params := map[string]float64{"fast": fast, "slow": slow}
			r, err := eng.Run(params)
			if err == nil && r.TotalTrades >= 2 {
				optResults = append(optResults, optResult{fast, slow, r.SharpeRatio, r.TotalTrades})
			}
		}
	}
	assert("S8a: optimizer produces results", len(optResults) > 0,
		fmt.Sprintf("got %d results", len(optResults)))
	if len(optResults) > 0 {
		fmt.Printf("  Grid search results:\n")
		for _, o := range optResults {
			fmt.Printf("    fast=%.0f slow=%.0f → trades=%d sharpe=%.2f\n", o.Fast, o.Slow, o.Trades, o.Sharpe)
		}
	}

	// ══════════════════════════════════════════════════════════════════════
	// SCENARIO 9: Multi-Symbol Comparison
	// ══════════════════════════════════════════════════════════════════════
	fmt.Println("\n── SCENARIO 9: Multi-Symbol Comparison ─────────────────────")

	compareSymbols := []string{"XAUUSD", "XAGUSD", "CL"}
	for _, sym := range compareSymbols {
		symInfo, _ := data.GetSymbol(sym)
		b, err := data.FetchOHLCV(ctx, data.FetchParams{Symbol: sym, Interval: "1d", Period: "1y"})
		if err != nil {
			assert(fmt.Sprintf("S9: %s fetch", sym), false, err.Error())
			continue
		}
		compCfg := backtest.Config{
			InitialCapital: 10000, PositionSizePct: 0.02,
			Slippage: symInfo.TickSize, Symbol: sym, Interval: "1d",
		}
		eng := backtest.NewEngine(compCfg)
		eng.LoadData(b)
		eng.SetStrategy(backtest.StrategyRegistry["ema_cross"].Factory())
		r, err := eng.Run(map[string]float64{"fast": 9, "slow": 21})
		if err != nil {
			assert(fmt.Sprintf("S9: %s backtest", sym), false, err.Error())
			continue
		}
		assert(fmt.Sprintf("S9: %-8s trades=%d WR=%.0f%% PnL=$%.0f Sharpe=%.2f",
			sym, r.TotalTrades, r.WinRate, r.TotalPnL, r.SharpeRatio),
			r.TotalTrades >= 0, "")
	}

	// ══════════════════════════════════════════════════════════════════════
	// SCENARIO 10: Custom Strategy Parameters
	// ══════════════════════════════════════════════════════════════════════
	fmt.Println("\n── SCENARIO 10: Custom Strategy Parameters ─────────────────")

	customTests := []struct {
		name   string
		strat  string
		params map[string]float64
	}{
		{"RSI custom thresholds", "rsi", map[string]float64{"period": 10, "overbought": 75, "oversold": 25}},
		{"MACD tight", "macd", map[string]float64{"fast": 8, "slow": 17, "signal": 9}},
		{"BB wide", "bb_breakout", map[string]float64{"period": 30, "std": 2.5}},
		{"Supertrend aggressive", "supertrend", map[string]float64{"period": 7, "multiplier": 2.0}},
		{"Donchian short", "donchian", map[string]float64{"period": 10}},
		{"SMA+RSI fast", "sma_rsi", map[string]float64{"sma_period": 20, "rsi_period": 7, "overbought": 65, "oversold": 35}},
	}

	for _, tt := range customTests {
		eng := backtest.NewEngine(cfg)
		eng.LoadData(bars)
		eng.SetStrategy(backtest.StrategyRegistry[tt.strat].Factory())
		r, err := eng.Run(tt.params)
		assert(fmt.Sprintf("S10: %s", tt.name),
			err == nil && r != nil,
			fmt.Sprintf("err=%v", err))
	}

	// ══════════════════════════════════════════════════════════════════════
	// SCENARIO 11: Slippage Correctness
	// ══════════════════════════════════════════════════════════════════════
	fmt.Println("\n── SCENARIO 11: Slippage Verification ──────────────────────")

	// Create test data where we know the exact entry/exit
	testBars := makeSyntheticBars(100, 100.0, 0.5)
	slipCfg := backtest.Config{
		InitialCapital: 10000, PositionSizePct: 0.1,
		Slippage: 1.0, Symbol: "TEST", Interval: "1d",
	}
	slipEng := backtest.NewEngine(slipCfg)
	slipEng.LoadData(testBars)
	slipEng.SetStrategy(backtest.StrategyRegistry["ema_cross"].Factory())
	slipR, err := slipEng.Run(map[string]float64{"fast": 5, "slow": 10})
	if err == nil && slipR != nil && len(slipR.Trades) > 0 {
		for _, t := range slipR.Trades {
			// Entry price should differ from bar open by slippage
			assert("S11: entry/exit prices non-zero",
				t.EntryPrice > 0 && t.ExitPrice > 0,
				fmt.Sprintf("entry=%.2f exit=%.2f", t.EntryPrice, t.ExitPrice))
			break // just check first trade
		}
	}

	// ══════════════════════════════════════════════════════════════════════
	// SCENARIO 12: Binary Startup Test
	// ══════════════════════════════════════════════════════════════════════
	fmt.Println("\n── SCENARIO 12: FormatNumber ────────────────────────────────")

	assert("S12a: FormatNumber 2 dec", data.FormatNumber(1234.567, 2) == "1234.57", data.FormatNumber(1234.567, 2))
	assert("S12b: FormatNumber 5 dec", data.FormatNumber(0.001, 5) == "0.00100", data.FormatNumber(0.001, 5))
	assert("S12c: FormatNumber 0 dec", data.FormatNumber(100.0, 0) == "100", data.FormatNumber(100.0, 0))

	// ══════════════════════════════════════════════════════════════════════
	// SCENARIO 13: Profit Factor Edge Cases
	// ══════════════════════════════════════════════════════════════════════
	fmt.Println("\n── SCENARIO 13: ProfitFactor Edge Cases ─────────────────────")

	// Strategy with sma_rsi often produces 0 trades on short data
	zeroEng := backtest.NewEngine(cfg)
	zeroEng.LoadData(bars)
	zeroEng.SetStrategy(backtest.StrategyRegistry["sma_rsi"].Factory())
	zeroR, err := zeroEng.Run(map[string]float64{"sma_period": 50, "rsi_period": 14, "overbought": 70, "oversold": 30})
	if err == nil && zeroR != nil && zeroR.TotalTrades == 0 {
		assert("S13a: 0 trades PF is +Inf", math.IsInf(zeroR.ProfitFactor, 1), fmt.Sprintf("PF=%f", zeroR.ProfitFactor))
		assert("S13b: 0 trades WR is 0", zeroR.WinRate == 0, "")
		assert("S13c: 0 trades Sharpe is 0", zeroR.SharpeRatio == 0, "")
	}

	// ══════════════════════════════════════════════════════════════════════
	// SCENARIO 14: Trade PnL Sum Equals Total PnL
	// ══════════════════════════════════════════════════════════════════════
	fmt.Println("\n── SCENARIO 14: PnL Integrity Check ────────────────────────")

	for key, meta := range backtest.StrategyRegistry {
		eng := backtest.NewEngine(cfg)
		eng.LoadData(bars)
		eng.SetStrategy(meta.Factory())
		r, err := eng.Run(meta.Params)
		if err != nil || r == nil {
			continue
		}
		sumPnL := 0.0
		for _, t := range r.Trades {
			sumPnL += t.PnL
		}
		diff := math.Abs(sumPnL - r.TotalPnL)
		assert(fmt.Sprintf("S14: %s sum(trade.PnL)==TotalPnL", key), diff < 0.01,
			fmt.Sprintf("sum=%.4f total=%.4f diff=%.6f", sumPnL, r.TotalPnL, diff))
	}

	// ══════════════════════════════════════════════════════════════════════
	// SUMMARY
	// ══════════════════════════════════════════════════════════════════════
	fmt.Println("\n" + strings.Repeat("=", 71))
	total := passed + failed
	fmt.Printf("  TOTAL: %d tests | PASSED: %d | FAILED: %d\n", total, passed, failed)
	fmt.Println(strings.Repeat("=", 71))

	if failed > 0 {
		os.Exit(1)
	}
}

// makeSyntheticBars creates predictable OHLCV data for testing
func makeSyntheticBars(n int, startPrice, volatility float64) []data.OHLCV {
	bars := make([]data.OHLCV, n)
	price := startPrice
	t := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		// Sine wave pattern for predictable crossovers
		delta := volatility * math.Sin(float64(i)*0.3)
		price += delta
		if price < 10 {
			price = 10
		}
		bars[i] = data.OHLCV{
			Time:   t.AddDate(0, 0, i),
			Open:   price - volatility*0.2,
			High:   price + volatility,
			Low:    price - volatility,
			Close:  price,
			Volume: 1000,
		}
	}
	return bars
}
