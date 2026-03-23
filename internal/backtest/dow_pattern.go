package backtest

import (
	"math"
	"time"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── Day-of-Week Pattern Strategy ────────────────────────────────────────
//
// Implements ICT day-of-week tendencies:
//   Monday:    Range setting / accumulation. Usually no trade.
//   Tuesday:   Reversal / manipulation. Look for fake moves.
//   Wednesday: Mid-week pivot. Trend change possible.
//   Thursday:  Expansion. Highest probability trend day.
//   Friday:    Consolidation / profit-taking.
//
// Signal generation:
//   - Only trade on allowed days (per trade_* params)
//   - Determine prior-day bias from the previous 1-3 trading days
//   - If prior days were bearish → prefer bullish reversal
//   - If prior days were bullish → prefer bearish reversal
//   - Enter on displacement confirmation on allowed days
//
// Parameters:
//   swing_period     – swing detection lookback (default 5)
//   atr_period       – ATR for displacement sizing (default 14)
//   disp_mult        – displacement threshold as ATR multiple (default 1.0)
//   body_ratio       – min body/range ratio for displacement (default 0.5)
//   trade_monday     – 0=skip, 1=trade (default 0)
//   trade_tuesday    – 0=skip, 1=trade (default 0)
//   trade_wednesday  – 0=skip, 1=trade (default 1)
//   trade_thursday   – 0=skip, 1=trade (default 1)
//   trade_friday     – 0=skip, 1=trade (default 0)

type DOWPatternStrategy struct {
	bars        []data.OHLCV
	atr         []float64
	swingHighs  []float64
	swingLows   []float64
	swingPeriod int
	dispMult    float64
	bodyRatio   float64
	tradeDays   [7]bool // indexed by time.Weekday (0=Sunday .. 6=Saturday)
	lastSigBar  int
	est         *time.Location

	// Per-day tracking: daily open/close for bias computation
	dailyBias map[string]int // dateKey → +1 bullish, -1 bearish
	dayOrder  []string
}

func (s *DOWPatternStrategy) Name() string { return "DOW Pattern" }
func (s *DOWPatternStrategy) Description() string {
	return "Day-of-Week Pattern: trade on high-probability days (Wed/Thu) with reversal bias from prior days"
}

func (s *DOWPatternStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	s.swingPeriod = int(getParam(params, "swing_period", 5))
	atrPeriod := int(getParam(params, "atr_period", 14))
	s.dispMult = getParam(params, "disp_mult", 1.0)
	s.bodyRatio = getParam(params, "body_ratio", 0.5)
	s.lastSigBar = -20
	s.est = time.FixedZone("EST", -5*3600)

	// Configure allowed trading days
	s.tradeDays = [7]bool{}
	if getParam(params, "trade_monday", 0) == 1 {
		s.tradeDays[time.Monday] = true
	}
	if getParam(params, "trade_tuesday", 0) == 1 {
		s.tradeDays[time.Tuesday] = true
	}
	if getParam(params, "trade_wednesday", 1) == 1 {
		s.tradeDays[time.Wednesday] = true
	}
	if getParam(params, "trade_thursday", 1) == 1 {
		s.tradeDays[time.Thursday] = true
	}
	if getParam(params, "trade_friday", 0) == 1 {
		s.tradeDays[time.Friday] = true
	}

	s.atr = indicators.ATR(bars, atrPeriod)
	s.swingHighs = indicators.SwingHighs(bars, s.swingPeriod)
	s.swingLows = indicators.SwingLows(bars, s.swingPeriod)

	s.precomputeDailyBias()
}

func (s *DOWPatternStrategy) tradingDateKey(t time.Time) string {
	estTime := t.In(s.est)
	if estTime.Hour() >= 18 {
		return estTime.Add(24 * time.Hour).Format("2006-01-02")
	}
	return estTime.Format("2006-01-02")
}

func (s *DOWPatternStrategy) precomputeDailyBias() {
	s.dailyBias = make(map[string]int)
	dayOpenClose := make(map[string][2]float64) // [0]=open, [1]=close
	seen := make(map[string]bool)

	for _, bar := range s.bars {
		dateKey := s.tradingDateKey(bar.Time.In(s.est))

		oc, exists := dayOpenClose[dateKey]
		if !exists {
			oc = [2]float64{bar.Open, bar.Close}
			s.dayOrder = append(s.dayOrder, dateKey)
			seen[dateKey] = true
		} else {
			oc[1] = bar.Close // update close to latest bar
		}
		dayOpenClose[dateKey] = oc
	}

	for _, dateKey := range s.dayOrder {
		oc := dayOpenClose[dateKey]
		if oc[1] > oc[0] {
			s.dailyBias[dateKey] = 1 // bullish day
		} else if oc[1] < oc[0] {
			s.dailyBias[dateKey] = -1 // bearish day
		} else {
			s.dailyBias[dateKey] = 0
		}
	}
}

// priorDaysBias returns the aggregate bias of the previous N trading days.
// +1 if net bullish, -1 if net bearish, 0 if neutral.
func (s *DOWPatternStrategy) priorDaysBias(dateKey string, n int) int {
	idx := -1
	for j, dk := range s.dayOrder {
		if dk == dateKey {
			idx = j
			break
		}
	}
	if idx < 1 {
		return 0
	}

	sum := 0
	count := 0
	for j := idx - 1; j >= 0 && count < n; j-- {
		sum += s.dailyBias[s.dayOrder[j]]
		count++
	}

	if sum > 0 {
		return 1
	} else if sum < 0 {
		return -1
	}
	return 0
}

func (s *DOWPatternStrategy) Signal(i int) SignalType {
	if i < s.swingPeriod*2+3 || math.IsNaN(s.atr[i]) || s.atr[i] == 0 {
		return NoSignal
	}
	if i-s.lastSigBar < s.swingPeriod*2 {
		return NoSignal
	}

	bar := s.bars[i]
	t := bar.Time.In(s.est)
	h := t.Hour()

	// Only generate during active session hours (07:00-16:00 EST)
	if h < 7 || h >= 16 {
		return NoSignal
	}

	// Check if today's day of week is allowed
	weekday := t.Weekday()
	if !s.tradeDays[weekday] {
		return NoSignal
	}

	dateKey := s.tradingDateKey(t)

	// Get prior days' bias (look back 3 trading days for context)
	priorBias := s.priorDaysBias(dateKey, 3)
	if priorBias == 0 {
		return NoSignal
	}

	rng := bar.High - bar.Low
	if rng == 0 {
		return NoSignal
	}

	// If prior days bearish → look for bullish reversal with displacement
	if priorBias == -1 {
		body := bar.Close - bar.Open
		if body > 0 && body >= s.dispMult*s.atr[i] && body/rng >= s.bodyRatio {
			s.lastSigBar = i
			return BuySignal
		}
	}

	// If prior days bullish → look for bearish reversal with displacement
	if priorBias == 1 {
		body := bar.Open - bar.Close
		if body > 0 && body >= s.dispMult*s.atr[i] && body/rng >= s.bodyRatio {
			s.lastSigBar = i
			return SellSignal
		}
	}

	return NoSignal
}
