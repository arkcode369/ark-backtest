package backtest

import (
	"fmt"
	"math"
	"sort"
	"time"
	"trading-backtest-bot/internal/data"
)

// ── Trade represents a single completed trade ─────────────────────────────

type TradeDirection string

const (
	Long  TradeDirection = "LONG"
	Short TradeDirection = "SHORT"
)

type Trade struct {
	EntryTime  time.Time
	ExitTime   time.Time
	Direction  TradeDirection
	EntryPrice float64
	ExitPrice  float64
	Quantity   float64
	PnL        float64 // in USD (or base currency)
	PnLPct     float64 // percentage
	MAE        float64 // Max Adverse Excursion
	MFE        float64 // Max Favorable Excursion
	Reason     string  // exit reason
}

// ── Signal types ──────────────────────────────────────────────────────────

type SignalType int

const (
	NoSignal   SignalType = 0
	BuySignal  SignalType = 1
	SellSignal SignalType = -1
)

// ── Strategy interface ────────────────────────────────────────────────────

// Strategy is implemented by all trading strategies
type Strategy interface {
	Name() string
	Init(bars []data.OHLCV, params map[string]float64)
	Signal(index int) SignalType // called bar by bar
	Description() string
}

// ── Engine configuration ──────────────────────────────────────────────────

type Config struct {
	InitialCapital  float64
	PositionSizePct float64 // % of capital per trade (e.g. 0.02 = 2%)
	Commission      float64 // round-trip commission in USD (charged once at exit)
	Slippage        float64 // in price units
	StopLossPct     float64 // 0 = disabled
	TakeProfitPct   float64 // 0 = disabled
	MaxOpenTrades   int     // reserved for future multi-position support; currently unused
	Symbol          string
	Interval        string
}

// ── Result ────────────────────────────────────────────────────────────────

type Result struct {
	Symbol         string
	Interval       string
	Strategy       string
	Period         string
	TotalTrades    int
	WinningTrades  int
	LosingTrades   int
	WinRate        float64
	TotalPnL       float64
	TotalPnLPct    float64
	MaxDrawdown    float64
	MaxDrawdownPct float64
	SharpeRatio    float64
	ProfitFactor   float64
	AvgWin         float64
	AvgLoss        float64
	LargestWin     float64
	LargestLoss    float64
	MaxConsecWins  int
	MaxConsecLoss  int
	Trades         []Trade
	EquityCurve    []float64
	FinalCapital   float64
	StartDate      time.Time
	EndDate        time.Time
}

// ── Engine ────────────────────────────────────────────────────────────────

type Engine struct {
	cfg      Config
	bars     []data.OHLCV
	strategy Strategy
}

func NewEngine(cfg Config) *Engine {
	return &Engine{cfg: cfg}
}

func (e *Engine) LoadData(bars []data.OHLCV) {
	e.bars = bars
}

func (e *Engine) SetStrategy(s Strategy) {
	e.strategy = s
}

// Run executes the backtest
func (e *Engine) Run(params map[string]float64) (*Result, error) {
	if len(e.bars) < 2 {
		return nil, fmt.Errorf("not enough data (got %d bars)", len(e.bars))
	}
	if e.strategy == nil {
		return nil, fmt.Errorf("no strategy set")
	}

	e.strategy.Init(e.bars, params)

	capital := e.cfg.InitialCapital
	equityCurve := []float64{capital}
	var trades []Trade

	type Position struct {
		Direction  TradeDirection
		EntryPrice float64
		EntryIndex int
		Quantity   float64
		StopLoss   float64
		TakeProfit float64
		MAE        float64
		MFE        float64
	}

	var openPos *Position

	for i := 1; i < len(e.bars); i++ {
		bar := e.bars[i]
		sig := e.strategy.Signal(i) // compute once per bar

		// ── Manage open position ─────────────────────────────
		if openPos != nil {
			// Update MAE/MFE
			if openPos.Direction == Long {
				unreal := bar.High - openPos.EntryPrice
				if unreal > openPos.MFE {
					openPos.MFE = unreal
				}
				adverse := openPos.EntryPrice - bar.Low
				if adverse > openPos.MAE {
					openPos.MAE = adverse
				}
			} else {
				unreal := openPos.EntryPrice - bar.Low
				if unreal > openPos.MFE {
					openPos.MFE = unreal
				}
				adverse := bar.High - openPos.EntryPrice
				if adverse > openPos.MAE {
					openPos.MAE = adverse
				}
			}

			// Check stop loss
			exitReason := ""
			exitPrice := 0.0
			if openPos.StopLoss > 0 {
				if openPos.Direction == Long && bar.Low <= openPos.StopLoss {
					exitReason = "SL"
					exitPrice = openPos.StopLoss
				} else if openPos.Direction == Short && bar.High >= openPos.StopLoss {
					exitReason = "SL"
					exitPrice = openPos.StopLoss
				}
			}
			// Check take profit
			if exitReason == "" && openPos.TakeProfit > 0 {
				if openPos.Direction == Long && bar.High >= openPos.TakeProfit {
					exitReason = "TP"
					exitPrice = openPos.TakeProfit
				} else if openPos.Direction == Short && bar.Low <= openPos.TakeProfit {
					exitReason = "TP"
					exitPrice = openPos.TakeProfit
				}
			}

			// Check strategy signal for exit
			if exitReason == "" {
				// BUG-01 fix: slippage works AGAINST trader on exit
				if openPos.Direction == Long && sig == SellSignal {
					exitReason = "Signal"
					exitPrice = bar.Open - e.cfg.Slippage
				} else if openPos.Direction == Short && sig == BuySignal {
					exitReason = "Signal"
					exitPrice = bar.Open + e.cfg.Slippage
				}
			}

			if exitReason != "" {
				// Close position
				var pnl float64
				if openPos.Direction == Long {
					pnl = (exitPrice - openPos.EntryPrice) * openPos.Quantity
				} else {
					pnl = (openPos.EntryPrice - exitPrice) * openPos.Quantity
				}
				// BUG-15 fix: commission represents round-trip cost, charged once at exit
				pnl -= e.cfg.Commission
				pnlPct := pnl / (openPos.EntryPrice * openPos.Quantity) * 100

				t := Trade{
					EntryTime:  e.bars[openPos.EntryIndex].Time,
					ExitTime:   bar.Time,
					Direction:  openPos.Direction,
					EntryPrice: openPos.EntryPrice,
					ExitPrice:  exitPrice,
					Quantity:   openPos.Quantity,
					PnL:        pnl,
					PnLPct:     pnlPct,
					MAE:        openPos.MAE,
					MFE:        openPos.MFE,
					Reason:     exitReason,
				}
				trades = append(trades, t)
				capital += pnl
				openPos = nil
			}
		}

		// ── Check for entry signal ───────────────────────────
		// BUG-04 fix: only check openPos == nil, don't check completed trade count
		if openPos == nil {
			if sig != NoSignal {
				// BUG-10 fix: use bar.Open for entry (consistent with exit pricing)
				entryPrice := bar.Open + float64(sig)*e.cfg.Slippage
				if entryPrice > 0 {
					qty := (capital * e.cfg.PositionSizePct) / entryPrice

					var sl, tp float64
					if e.cfg.StopLossPct > 0 {
						if sig == BuySignal {
							sl = entryPrice * (1 - e.cfg.StopLossPct)
						} else {
							sl = entryPrice * (1 + e.cfg.StopLossPct)
						}
					}
					if e.cfg.TakeProfitPct > 0 {
						if sig == BuySignal {
							tp = entryPrice * (1 + e.cfg.TakeProfitPct)
						} else {
							tp = entryPrice * (1 - e.cfg.TakeProfitPct)
						}
					}

					dir := Long
					if sig == SellSignal {
						dir = Short
					}

					openPos = &Position{
						Direction:  dir,
						EntryPrice: entryPrice,
						EntryIndex: i,
						Quantity:   qty,
						StopLoss:   sl,
						TakeProfit: tp,
					}
					// BUG-15 fix: removed entry-side commission deduction;
					// commission is charged as round-trip cost at exit only
				}
			}
		}

		// BUG-07 fix: per-bar mark-to-market equity curve
		if openPos != nil {
			var unrealizedPnL float64
			if openPos.Direction == Long {
				unrealizedPnL = (bar.Close - openPos.EntryPrice) * openPos.Quantity
			} else {
				unrealizedPnL = (openPos.EntryPrice - bar.Close) * openPos.Quantity
			}
			equityCurve = append(equityCurve, capital+unrealizedPnL)
		} else {
			equityCurve = append(equityCurve, capital)
		}
	}

	// Close any open position at end
	if openPos != nil {
		lastBar := e.bars[len(e.bars)-1]
		exitPrice := lastBar.Close
		var pnl float64
		if openPos.Direction == Long {
			pnl = (exitPrice - openPos.EntryPrice) * openPos.Quantity
		} else {
			pnl = (openPos.EntryPrice - exitPrice) * openPos.Quantity
		}
		// Round-trip commission charged at exit
		pnl -= e.cfg.Commission
		pnlPct := pnl / (openPos.EntryPrice * openPos.Quantity) * 100

		trades = append(trades, Trade{
			EntryTime:  e.bars[openPos.EntryIndex].Time,
			ExitTime:   lastBar.Time,
			Direction:  openPos.Direction,
			EntryPrice: openPos.EntryPrice,
			ExitPrice:  exitPrice,
			Quantity:   openPos.Quantity,
			PnL:        pnl,
			PnLPct:     pnlPct,
			MAE:        openPos.MAE,
			MFE:        openPos.MFE,
			Reason:     "EOD",
		})
		capital += pnl
		// Update the last equity curve entry to reflect the closed position
		equityCurve[len(equityCurve)-1] = capital
	}

	return computeStats(e, trades, equityCurve, capital), nil
}

func computeStats(e *Engine, trades []Trade, equity []float64, finalCapital float64) *Result {
	r := &Result{
		Symbol:       e.cfg.Symbol,
		Interval:     e.cfg.Interval,
		Strategy:     e.strategy.Name(),
		Trades:       trades,
		EquityCurve:  equity,
		FinalCapital: finalCapital,
	}

	if len(e.bars) > 0 {
		r.StartDate = e.bars[0].Time
		r.EndDate = e.bars[len(e.bars)-1].Time
		r.Period = fmt.Sprintf("%s -> %s", r.StartDate.Format("2006-01-02"), r.EndDate.Format("2006-01-02"))
	}

	r.TotalTrades = len(trades)
	totalPnL := 0.0
	grossProfit := 0.0
	grossLoss := 0.0
	consecWins, consecLoss := 0, 0
	maxCW, maxCL := 0, 0

	for _, t := range trades {
		totalPnL += t.PnL
		if t.PnL > 0 {
			r.WinningTrades++
			grossProfit += t.PnL
			if t.PnL > r.LargestWin {
				r.LargestWin = t.PnL
			}
			consecWins++
			consecLoss = 0
			if consecWins > maxCW {
				maxCW = consecWins
			}
		} else {
			r.LosingTrades++
			grossLoss += math.Abs(t.PnL)
			if math.Abs(t.PnL) > r.LargestLoss {
				r.LargestLoss = math.Abs(t.PnL)
			}
			consecLoss++
			consecWins = 0
			if consecLoss > maxCL {
				maxCL = consecLoss
			}
		}
	}

	r.TotalPnL = totalPnL
	r.TotalPnLPct = (finalCapital - e.cfg.InitialCapital) / e.cfg.InitialCapital * 100
	r.MaxConsecWins = maxCW
	r.MaxConsecLoss = maxCL

	if r.TotalTrades > 0 {
		r.WinRate = float64(r.WinningTrades) / float64(r.TotalTrades) * 100
	}
	if r.WinningTrades > 0 {
		r.AvgWin = grossProfit / float64(r.WinningTrades)
	}
	if r.LosingTrades > 0 {
		r.AvgLoss = grossLoss / float64(r.LosingTrades)
	}
	if grossLoss > 0 {
		r.ProfitFactor = grossProfit / grossLoss
	} else {
		r.ProfitFactor = math.Inf(1)
	}

	// BUG-06 fix: Max Drawdown percentage uses peak equity at point of max drawdown
	peak := equity[0]
	peakAtMaxDD := peak
	for _, eq := range equity {
		if eq > peak {
			peak = eq
		}
		dd := peak - eq
		if dd > r.MaxDrawdown {
			r.MaxDrawdown = dd
			peakAtMaxDD = peak
		}
	}
	if peakAtMaxDD > 0 {
		r.MaxDrawdownPct = r.MaxDrawdown / peakAtMaxDD * 100
	}

	// BUG-08 fix: Sharpe Ratio annualized using trades-per-year instead of sqrt(252)
	r.SharpeRatio = computeSharpe(trades, r.StartDate, r.EndDate)

	return r
}

func computeSharpe(trades []Trade, startDate, endDate time.Time) float64 {
	if len(trades) < 2 {
		return 0
	}
	returns := make([]float64, len(trades))
	for i, t := range trades {
		returns[i] = t.PnLPct / 100
	}
	mean := 0.0
	for _, r := range returns {
		mean += r
	}
	mean /= float64(len(returns))

	variance := 0.0
	for _, r := range returns {
		d := r - mean
		variance += d * d
	}
	variance /= float64(len(returns))
	std := math.Sqrt(variance)

	if std == 0 {
		return 0
	}

	// Compute actual trading days and trades-per-year for proper annualization
	totalDays := endDate.Sub(startDate).Hours() / 24
	if totalDays <= 0 {
		return 0
	}
	years := totalDays / 365.25
	tradesPerYear := float64(len(trades)) / years

	return (mean / std) * math.Sqrt(tradesPerYear)
}

// FormatResult returns a human-readable summary of the backtest result
func FormatResult(r *Result, initialCapital float64) string {
	pf := "\u221e"
	if !math.IsInf(r.ProfitFactor, 1) {
		pf = fmt.Sprintf("%.2f", r.ProfitFactor)
	}

	return fmt.Sprintf("\U0001f4ca *Backtest Result*\n\n"+
		"*Symbol:* %s | *Interval:* %s\n"+
		"*Strategy:* %s\n"+
		"*Period:* %s\n\n"+
		"\U0001f4b0 *Capital*\n"+
		"  Initial: $%.2f\n"+
		"  Final:   $%.2f (%.2f%%)\n"+
		"  Net P&L: $%.2f\n\n"+
		"\U0001f4c8 *Performance*\n"+
		"  Total Trades: %d\n"+
		"  Win Rate: %.1f%% (%d W / %d L)\n"+
		"  Profit Factor: %s\n"+
		"  Sharpe Ratio: %.2f\n\n"+
		"\U0001f4c9 *Drawdown*\n"+
		"  Max DD: $%.2f (%.2f%%)\n\n"+
		"\u26a1 *Trade Stats*\n"+
		"  Avg Win:  $%.2f\n"+
		"  Avg Loss: $%.2f\n"+
		"  Largest Win:  $%.2f\n"+
		"  Largest Loss: $%.2f\n"+
		"  Max Consec. Wins: %d\n"+
		"  Max Consec. Loss: %d",
		r.Symbol, r.Interval,
		r.Strategy,
		r.Period,
		initialCapital, r.FinalCapital, r.TotalPnLPct, r.TotalPnL,
		r.TotalTrades,
		r.WinRate, r.WinningTrades, r.LosingTrades,
		pf, r.SharpeRatio,
		r.MaxDrawdown, r.MaxDrawdownPct,
		r.AvgWin, r.AvgLoss,
		r.LargestWin, r.LargestLoss,
		r.MaxConsecWins, r.MaxConsecLoss,
	)
}

// TopTrades returns the N best/worst trades
func TopTrades(r *Result, n int) (best, worst []Trade) {
	sorted := make([]Trade, len(r.Trades))
	copy(sorted, r.Trades)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].PnL > sorted[j].PnL })
	if n > len(sorted) {
		n = len(sorted)
	}
	best = sorted[:n]
	worst = sorted[len(sorted)-n:]
	return
}

// WalkForwardResult holds both in-sample and out-of-sample backtest results
type WalkForwardResult struct {
	InSample    *Result
	OutOfSample *Result
	SplitIndex  int // bar index where the split occurs
}

// RunWalkForward splits data into in-sample and out-of-sample portions,
// runs the strategy on both, and returns both results.
// oosFraction is the fraction of data reserved for out-of-sample (0.0-0.5).
func (e *Engine) RunWalkForward(params map[string]float64, oosFraction float64) (*WalkForwardResult, error) {
	if oosFraction <= 0 || oosFraction > 0.5 {
		return nil, fmt.Errorf("oos fraction must be between 0 and 0.5 (got %.2f)", oosFraction)
	}
	if len(e.bars) < 60 {
		return nil, fmt.Errorf("need at least 60 bars for walk-forward (got %d)", len(e.bars))
	}
	if e.strategy == nil {
		return nil, fmt.Errorf("no strategy set")
	}

	splitIdx := int(float64(len(e.bars)) * (1 - oosFraction))
	if splitIdx < 30 {
		splitIdx = 30
	}

	// In-sample
	isEngine := NewEngine(e.cfg)
	isEngine.LoadData(e.bars[:splitIdx])
	isEngine.SetStrategy(e.strategy)
	isResult, err := isEngine.Run(params)
	if err != nil {
		return nil, fmt.Errorf("in-sample error: %w", err)
	}

	// Out-of-sample
	oosEngine := NewEngine(e.cfg)
	oosEngine.LoadData(e.bars[splitIdx:])
	oosEngine.SetStrategy(e.strategy)
	oosResult, err := oosEngine.Run(params)
	if err != nil {
		return nil, fmt.Errorf("out-of-sample error: %w", err)
	}

	return &WalkForwardResult{
		InSample:    isResult,
		OutOfSample: oosResult,
		SplitIndex:  splitIdx,
	}, nil
}

// FormatWalkForwardResult formats the walk-forward result for display
func FormatWalkForwardResult(wf *WalkForwardResult) string {
	is := wf.InSample
	oos := wf.OutOfSample

	return fmt.Sprintf("\U0001f50d *Walk-Forward Analysis*\n\n"+
		"*In-Sample (%s):*\n"+
		"  Trades: %d | WR: %.1f%% | P&L: $%.2f\n"+
		"  Sharpe: %.2f | PF: %.2f | DD: %.1f%%\n\n"+
		"*Out-of-Sample (%s):*\n"+
		"  Trades: %d | WR: %.1f%% | P&L: $%.2f\n"+
		"  Sharpe: %.2f | PF: %.2f | DD: %.1f%%\n\n"+
		"*Robustness Check:*\n"+
		"  Sharpe Decay: %.1f%%\n"+
		"  Win Rate Decay: %.1f pp\n"+
		"  PF Decay: %.1f%%",
		is.Period,
		is.TotalTrades, is.WinRate, is.TotalPnL,
		is.SharpeRatio, is.ProfitFactor, is.MaxDrawdownPct,
		oos.Period,
		oos.TotalTrades, oos.WinRate, oos.TotalPnL,
		oos.SharpeRatio, oos.ProfitFactor, oos.MaxDrawdownPct,
		sharpeDecay(is.SharpeRatio, oos.SharpeRatio),
		oos.WinRate-is.WinRate,
		pfDecay(is.ProfitFactor, oos.ProfitFactor),
	)
}

func sharpeDecay(is, oos float64) float64 {
	if is == 0 {
		return 0
	}
	return (oos - is) / math.Abs(is) * 100
}

func pfDecay(is, oos float64) float64 {
	if is == 0 || math.IsInf(is, 0) {
		return 0
	}
	if math.IsInf(oos, 0) {
		return 100
	}
	return (oos - is) / is * 100
}
