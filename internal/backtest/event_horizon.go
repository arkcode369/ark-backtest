package backtest

import (
	"math"
	"time"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ── Event Horizon PD Array Strategy ─────────────────────────────────────
//
// NWOG = New Week Opening Gap (Friday close to Monday open).
// Event Horizon = midpoint between the midpoints of two consecutive NWOGs.
// This level acts as a "gravitational" attraction point for price.
//
// Implementation:
//   1. Detect NWOGs from the input bars by finding week boundaries
//      (Monday open vs previous Friday close). Since Init only receives
//      LTF bars, week boundaries are identified directly from timestamps.
//   2. For two consecutive NWOGs, compute Event Horizon =
//      (NWOG_A.MidPoint + NWOG_B.MidPoint) / 2.
//   3. When price approaches the Event Horizon with displacement,
//      enter in the direction determined by the side of approach.
//
// Parameters:
//   swing_period – swing detection period (default 5)
//   atr_period   – ATR period (default 14)
//   disp_mult    – displacement ATR multiple (default 0.5)
//   body_ratio   – min body/range ratio (default 0.4)
//   touch_atr    – max distance to Event Horizon in ATR multiples (default 1.0)

type EventHorizonStrategy struct {
	bars        []data.OHLCV
	atr         []float64
	swingPeriod int
	dispMult    float64
	bodyRatio   float64
	touchATR    float64
	lastSigBar  int
	// eventHorizons stores (barIndex, level) pairs marking when each
	// Event Horizon becomes active and its price level.
	eventHorizons []ehLevel
}

type ehLevel struct {
	activeFrom int     // bar index from which this EH is valid
	level      float64 // the Event Horizon price
}

func (s *EventHorizonStrategy) Name() string { return "Event Horizon" }
func (s *EventHorizonStrategy) Description() string {
	return "Event Horizon: enter near the midpoint between two consecutive NWOG midpoints"
}

func (s *EventHorizonStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	s.swingPeriod = int(getParam(params, "swing_period", 5))
	atrPeriod := int(getParam(params, "atr_period", 14))
	s.dispMult = getParam(params, "disp_mult", 0.5)
	s.bodyRatio = getParam(params, "body_ratio", 0.4)
	s.touchATR = getParam(params, "touch_atr", 1.0)
	s.lastSigBar = -10

	s.atr = indicators.ATR(bars, atrPeriod)

	// Detect NWOGs from bars by finding week boundaries.
	nwogs := s.detectNWOGs(bars)

	// Build Event Horizon levels from consecutive NWOG pairs.
	s.eventHorizons = nil
	for k := 1; k < len(nwogs); k++ {
		ehPrice := (nwogs[k-1].mid + nwogs[k].mid) / 2.0
		s.eventHorizons = append(s.eventHorizons, ehLevel{
			activeFrom: nwogs[k].barIdx,
			level:      ehPrice,
		})
	}
}

// nwogInfo holds a detected NWOG with bar index and midpoint.
type nwogInfo struct {
	barIdx int
	mid    float64
}

// detectNWOGs scans bars for week boundaries and computes NWOGs.
func (s *EventHorizonStrategy) detectNWOGs(bars []data.OHLCV) []nwogInfo {
	if len(bars) < 2 {
		return nil
	}

	var nwogs []nwogInfo
	for i := 1; i < len(bars); i++ {
		prevDay := bars[i-1].Time.Weekday()
		currDay := bars[i].Time.Weekday()

		isWeekGap := false
		if prevDay == time.Friday && (currDay == time.Monday || currDay == time.Sunday) {
			isWeekGap = true
		}
		// Catch gaps spanning more than 2 calendar days (holidays, etc.)
		dayDiff := bars[i].Time.Sub(bars[i-1].Time).Hours() / 24.0
		if dayDiff > 2 {
			isWeekGap = true
		}
		if !isWeekGap {
			continue
		}

		prevClose := bars[i-1].Close
		currOpen := bars[i].Open
		gapSize := math.Abs(currOpen - prevClose)
		if gapSize < 1e-10 {
			continue
		}

		high := math.Max(prevClose, currOpen)
		low := math.Min(prevClose, currOpen)
		mid := (high + low) / 2.0

		nwogs = append(nwogs, nwogInfo{
			barIdx: i,
			mid:    mid,
		})
	}
	return nwogs
}

func (s *EventHorizonStrategy) Signal(i int) SignalType {
	if i < 2 || math.IsNaN(s.atr[i]) || s.atr[i] == 0 {
		return NoSignal
	}
	if i-s.lastSigBar < s.swingPeriod {
		return NoSignal
	}
	if len(s.eventHorizons) == 0 {
		return NoSignal
	}

	bar := s.bars[i]
	rng := bar.High - bar.Low
	if rng == 0 {
		return NoSignal
	}

	// Find the most recent active Event Horizon.
	var activeEH *ehLevel
	for k := len(s.eventHorizons) - 1; k >= 0; k-- {
		if s.eventHorizons[k].activeFrom <= i {
			activeEH = &s.eventHorizons[k]
			break
		}
	}
	if activeEH == nil {
		return NoSignal
	}

	level := activeEH.level
	tol := s.touchATR * s.atr[i]

	// Check if bar is near the Event Horizon level.
	if bar.Low > level+tol || bar.High < level-tol {
		return NoSignal
	}

	// Determine direction by displacement.
	bullBody := bar.Close - bar.Open
	bearBody := bar.Open - bar.Close

	if bullBody >= s.dispMult*s.atr[i] && bullBody/rng >= s.bodyRatio {
		s.lastSigBar = i
		return BuySignal
	}
	if bearBody >= s.dispMult*s.atr[i] && bearBody/rng >= s.bodyRatio {
		s.lastSigBar = i
		return SellSignal
	}

	return NoSignal
}
