package backtest

import (
	"math"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/indicators"
)

// ICTAdvancedStrategy extends the base ICT 2022 model with multi-timeframe bias,
// session awareness (Kill Zones, CBDR/STD), NDOG/NWOG gap detection, and SMT divergence.
// It implements MultiTimeframeStrategy, MultiSymbolStrategy, and SessionAwareStrategy.
type ICTAdvancedStrategy struct {
	// ── Base ICT fields (same as ICTMentorshipStrategy) ──
	bars        []data.OHLCV
	atr         []float64
	swingHighs  []float64
	swingLows   []float64
	swingPeriod int
	dispMult    float64
	bodyRatio   float64
	fibValid    bool
	lookback    int
	lastSigBar  int

	// ── MTF fields ──
	htfBars  map[string][]data.OHLCV
	htfIndex []int // maps LTF bar index → HTF bar index
	htfBias  []int // per-LTF-bar: +1 bullish, -1 bearish, 0 neutral

	// ── Session fields ──
	sessions    []SessionLabel
	cbdrResults []CBDRResult
	ndogGaps    []OpeningGap
	nwogGaps    []OpeningGap

	// ── SMT fields ──
	smtSignals []SMTSignal
	smtByIndex map[int]SMTSignal // fast lookup by bar index

	// ── PD Array fields ──
	pdZones     []indicators.PDZone
	pdResult    indicators.PremiumDiscountResult // cached per-bar premium/discount
	usePDArrays bool

	// ── Config toggles (from params) ──
	useHTFFilter  bool
	useKillZone   bool
	useCBDR       bool
	useSMT        bool
	useGaps       bool
	correlatedSym string
}

func (s *ICTAdvancedStrategy) Name() string { return "ICT Advanced" }
func (s *ICTAdvancedStrategy) Description() string {
	return "ICT 2022 + MTF bias, Kill Zones, CBDR/STD, NDOG/NWOG, SMT divergence"
}

// ── Strategy interface ──

func (s *ICTAdvancedStrategy) Init(bars []data.OHLCV, params map[string]float64) {
	s.bars = bars
	s.swingPeriod = int(getParam(params, "swing_period", 5))
	atrPeriod := int(getParam(params, "atr_period", 14))
	s.dispMult = getParam(params, "disp_mult", 1.5)
	s.bodyRatio = getParam(params, "body_ratio", 0.6)
	s.fibValid = getParam(params, "fvg_fib_valid", 1) == 1
	s.lookback = int(getParam(params, "lookback", float64(s.swingPeriod*6)))
	if s.lookback < s.swingPeriod*3 {
		s.lookback = s.swingPeriod * 3
	}
	s.lastSigBar = -s.lookback

	// Config toggles
	s.useHTFFilter = getParam(params, "htf_filter", 1) == 1
	s.useKillZone = getParam(params, "killzone_only", 0) == 1
	s.useCBDR = getParam(params, "cbdr_filter", 0) == 1
	s.useSMT = getParam(params, "smt_confluence", 0) == 1
	s.useGaps = getParam(params, "gap_awareness", 0) == 1
	s.usePDArrays = getParam(params, "pd_arrays", 0) == 1

	s.atr = indicators.ATR(bars, atrPeriod)
	s.swingHighs = indicators.SwingHighs(bars, s.swingPeriod)
	s.swingLows = indicators.SwingLows(bars, s.swingPeriod)

	// Detect PD Array zones and cache premium/discount per bar
	s.pdZones = indicators.DetectAllPDArrays(bars, indicators.DefaultPDParams())
	s.pdResult = indicators.ComputePremiumDiscount(bars, s.lookback)
}

// ── MultiTimeframeStrategy interface ──

func (s *ICTAdvancedStrategy) Timeframes() []string {
	return []string{"1d"}
}

func (s *ICTAdvancedStrategy) InitMTF(barsByTF map[string][]data.OHLCV, params map[string]float64) {
	s.htfBars = barsByTF
	daily, ok := barsByTF["1d"]
	if !ok || len(daily) < 3 {
		return
	}
	s.htfIndex = data.AlignHTFToLTF(s.bars, daily)
	s.htfBias = computeHTFBias(daily, s.bars, s.htfIndex)
}

// ── MultiSymbolStrategy interface ──

func (s *ICTAdvancedStrategy) Symbols() []string {
	if s.correlatedSym != "" {
		return []string{s.correlatedSym}
	}
	return nil
}

func (s *ICTAdvancedStrategy) InitMultiSymbol(barsBySymbol map[string][]data.OHLCV, params map[string]float64) {
	s.correlatedSym = "" // reset
	for sym := range barsBySymbol {
		_ = sym
		symBars := barsBySymbol[sym]
		s.smtSignals = DetectSMT(s.bars, symBars, s.swingPeriod, s.lookback)
		s.smtByIndex = make(map[int]SMTSignal)
		for _, sig := range s.smtSignals {
			s.smtByIndex[sig.Index] = sig
		}
		break // only use first correlated symbol
	}
}

// ── SessionAwareStrategy interface ──

func (s *ICTAdvancedStrategy) InitSessions(sessions []SessionLabel) {
	s.sessions = sessions
	s.cbdrResults = ComputeCBDR(s.bars)
	s.ndogGaps = DetectNDOG(s.bars)
	// NWOG needs daily bars; use HTF if available, otherwise use execution bars
	if daily, ok := s.htfBars["1d"]; ok {
		s.nwogGaps = DetectNWOG(daily)
	} else {
		s.nwogGaps = DetectNWOG(s.bars)
	}
}

// ── Signal generation ──

func (s *ICTAdvancedStrategy) Signal(i int) SignalType {
	if i < s.swingPeriod*2+3 || math.IsNaN(s.atr[i]) {
		return NoSignal
	}
	if i-s.lastSigBar < s.swingPeriod {
		return NoSignal
	}

	// Kill Zone filter: if enabled, only generate signals during kill zones
	if s.useKillZone && s.sessions != nil && i < len(s.sessions) {
		if !s.sessions[i].IsKillZone {
			return NoSignal
		}
	}

	// Run base ICT checks (same logic as ICTMentorshipStrategy)
	buy := s.checkBuy(i)
	sell := s.checkSell(i)

	if !buy && !sell {
		return NoSignal
	}

	// Apply HTF bias filter
	if s.useHTFFilter && s.htfBias != nil && i < len(s.htfBias) {
		bias := s.htfBias[i]
		if buy && bias < 0 {
			buy = false // don't buy against bearish HTF bias
		}
		if sell && bias > 0 {
			sell = false // don't sell against bullish HTF bias
		}
	}

	// SMT confluence filter: if enabled, require SMT divergence alignment
	if s.useSMT && s.smtByIndex != nil {
		if buy {
			// Look for bullish SMT near this bar
			found := false
			for j := max(0, i-s.swingPeriod); j <= i; j++ {
				if sig, ok := s.smtByIndex[j]; ok && sig.Type == BullishSMT {
					found = true
					break
				}
			}
			if !found {
				buy = false
			}
		}
		if sell {
			found := false
			for j := max(0, i-s.swingPeriod); j <= i; j++ {
				if sig, ok := s.smtByIndex[j]; ok && sig.Type == BearishSMT {
					found = true
					break
				}
			}
			if !found {
				sell = false
			}
		}
	}

	// CBDR filter: if enabled, check that current price is within CBDR STD projection range
	// This validates that the market has room to move in the expected direction
	if s.useCBDR && len(s.cbdrResults) > 0 && i < len(s.bars) {
		cbdr := FindCBDRForBar(s.cbdrResults, s.bars[i])
		if cbdr != nil && cbdr.Range > 0 {
			price := s.bars[i].Close
			if buy {
				// For buys, price should be below STD2 up projection (room to go up)
				if price > cbdr.StdDev2Up {
					buy = false // already extended too far up
				}
			}
			if sell {
				// For sells, price should be above STD2 down projection (room to go down)
				if price < cbdr.StdDev2Down {
					sell = false // already extended too far down
				}
			}
		}
	}

	// PD Array confluence filter: if enabled, require alignment with PD zones
	if s.usePDArrays && len(s.pdZones) > 0 {
		if buy && !s.hasPDConfluence(i, indicators.Bullish) {
			buy = false
		}
		if sell && !s.hasPDConfluence(i, indicators.Bearish) {
			sell = false
		}
	}

	if buy {
		s.lastSigBar = i
		return BuySignal
	}
	if sell {
		s.lastSigBar = i
		return SellSignal
	}
	return NoSignal
}

func (s *ICTAdvancedStrategy) checkBuy(i int) bool {
	bars := s.bars
	start := i - s.lookback
	if start < s.swingPeriod {
		start = s.swingPeriod
	}

	for j := i - s.swingPeriod - 1; j >= start; j-- {
		if math.IsNaN(s.swingLows[j]) {
			continue
		}
		swingLowPrice := s.swingLows[j]

		// Step 1: Liquidity grab — sweep below swing low
		grabIdx := -1
		for g := j + 1; g <= i; g++ {
			if bars[g].Low < swingLowPrice && bars[g].Close > swingLowPrice {
				grabIdx = g
				break
			}
		}
		if grabIdx < 0 {
			continue
		}

		// Step 2: Displacement + bullish FVG
		dispIdx := -1
		var fvgTop, fvgBot float64
		for d := grabIdx + 1; d < i; d++ {
			if math.IsNaN(s.atr[d]) || s.atr[d] == 0 {
				continue
			}
			body := bars[d].Close - bars[d].Open
			rng := bars[d].High - bars[d].Low
			if rng == 0 {
				continue
			}
			if body >= s.dispMult*s.atr[d] && body/rng >= s.bodyRatio {
				if d-1 >= 0 && d+1 < len(bars) {
					gapBot := bars[d-1].High
					gapTop := bars[d+1].Low
					if gapTop > gapBot {
						dispIdx = d
						fvgBot = gapBot
						fvgTop = gapTop
						break
					}
				}
			}
		}
		if dispIdx < 0 {
			continue
		}

		// Step 3: MSS — break above prior swing high
		swingHighPrice := 0.0
		for k := grabIdx - 1; k >= start; k-- {
			if !math.IsNaN(s.swingHighs[k]) {
				swingHighPrice = s.swingHighs[k]
				break
			}
		}
		if swingHighPrice == 0 {
			continue
		}
		foundMSS := false
		for k := dispIdx; k <= i; k++ {
			if bars[k].High > swingHighPrice {
				foundMSS = true
				break
			}
		}
		if !foundMSS {
			continue
		}

		// Step 4-5: Fib validation
		if s.fibValid {
			grabLow := bars[grabIdx].Low
			dispHigh := bars[dispIdx].High
			fib50 := grabLow + (dispHigh-grabLow)*0.5
			if fvgBot > fib50 {
				continue
			}
		}

		// Step 6: Entry — retrace into FVG
		if bars[i].Low <= fvgTop && bars[i].High >= fvgBot {
			// Gap awareness: check if any NDOG/NWOG aligns with FVG zone (bonus, not required)
			if s.useGaps {
				s.checkGapAlignment(i, fvgBot, fvgTop)
			}
			return true
		}
	}
	return false
}

func (s *ICTAdvancedStrategy) checkSell(i int) bool {
	bars := s.bars
	start := i - s.lookback
	if start < s.swingPeriod {
		start = s.swingPeriod
	}

	for j := i - s.swingPeriod - 1; j >= start; j-- {
		if math.IsNaN(s.swingHighs[j]) {
			continue
		}
		swingHighPrice := s.swingHighs[j]

		// Step 1: BSL grab — sweep above swing high
		grabIdx := -1
		for g := j + 1; g <= i; g++ {
			if bars[g].High > swingHighPrice && bars[g].Close < swingHighPrice {
				grabIdx = g
				break
			}
		}
		if grabIdx < 0 {
			continue
		}

		// Step 2: Bearish displacement + bearish FVG
		dispIdx := -1
		var fvgTop, fvgBot float64
		for d := grabIdx + 1; d < i; d++ {
			if math.IsNaN(s.atr[d]) || s.atr[d] == 0 {
				continue
			}
			body := bars[d].Open - bars[d].Close
			rng := bars[d].High - bars[d].Low
			if rng == 0 {
				continue
			}
			if body >= s.dispMult*s.atr[d] && body/rng >= s.bodyRatio {
				if d-1 >= 0 && d+1 < len(bars) {
					gapTop := bars[d-1].Low
					gapBot := bars[d+1].High
					if gapTop > gapBot {
						dispIdx = d
						fvgTop = gapTop
						fvgBot = gapBot
						break
					}
				}
			}
		}
		if dispIdx < 0 {
			continue
		}

		// Step 3: MSS — break below prior swing low
		swingLowPrice := 0.0
		for k := grabIdx - 1; k >= start; k-- {
			if !math.IsNaN(s.swingLows[k]) {
				swingLowPrice = s.swingLows[k]
				break
			}
		}
		if swingLowPrice == 0 {
			continue
		}
		foundMSS := false
		for k := dispIdx; k <= i; k++ {
			if bars[k].Low < swingLowPrice {
				foundMSS = true
				break
			}
		}
		if !foundMSS {
			continue
		}

		// Step 4-5: Fib validation
		if s.fibValid {
			grabHigh := bars[grabIdx].High
			dispLow := bars[dispIdx].Low
			fib50 := dispLow + (grabHigh-dispLow)*0.5
			if fvgTop < fib50 {
				continue
			}
		}

		// Step 6: Entry — retrace into FVG
		if bars[i].High >= fvgBot && bars[i].Low <= fvgTop {
			if s.useGaps {
				s.checkGapAlignment(i, fvgBot, fvgTop)
			}
			return true
		}
	}
	return false
}

// checkGapAlignment logs/tracks when a gap zone overlaps with the FVG zone.
// Currently informational — could be used to adjust confidence scoring in future.
func (s *ICTAdvancedStrategy) checkGapAlignment(i int, fvgBot, fvgTop float64) bool {
	bar := s.bars[i]
	relevantGaps := FindGapForBar(append(s.ndogGaps, s.nwogGaps...), bar)
	for _, gap := range relevantGaps {
		// Check if gap zone overlaps with FVG zone
		if gap.Low <= fvgTop && gap.High >= fvgBot {
			return true // gap aligns with FVG — high-probability zone
		}
	}
	return false
}

// hasPDConfluence checks if there is an active (not mitigated) PD zone near
// the current bar's price that aligns with the given direction.
// For Bullish: checks for discount zone or bullish PD zones (FVG, OB, Breaker, Rejection Block).
// For Bearish: checks for premium zone or bearish PD zones.
func (s *ICTAdvancedStrategy) hasPDConfluence(i int, dir indicators.PDDirection) bool {
	if i >= len(s.bars) {
		return false
	}
	price := s.bars[i].Close
	barHigh := s.bars[i].High
	barLow := s.bars[i].Low

	// Check rolling premium/discount from cached result
	if i < len(s.pdResult.InDiscount) {
		if dir == indicators.Bullish && s.pdResult.InDiscount[i] {
			return true
		}
		if dir == indicators.Bearish && s.pdResult.InPremium[i] {
			return true
		}
	}

	// Check for PD zone overlap with current bar's price range
	for _, z := range s.pdZones {
		if z.Mitigated {
			continue
		}
		// Zone must have been detected before or at the current bar
		if z.Index > i {
			continue
		}
		if z.Direction != dir {
			continue
		}
		// Only consider relevant zone types
		switch z.Type {
		case indicators.PDFairValueGap, indicators.PDOrderBlock,
			indicators.PDBreakerBlock, indicators.PDRejectionBlock,
			indicators.PDPropulsionBlock, indicators.PDVolumeImbalance:
			// Check if bar price overlaps with zone
			if barLow <= z.Top && barHigh >= z.Bottom {
				return true
			}
			// Also check if close is within the zone
			if price >= z.Bottom && price <= z.Top {
				return true
			}
		}
	}
	return false
}

// ── HTF Bias Computation ──

// computeHTFBias determines daily bias for each LTF bar based on HTF (daily) candle structure.
// Bullish bias: previous daily close > previous daily open (bullish candle) AND
//
//	close above previous day's high (PDH)
//
// Bearish bias: previous daily close < previous daily open AND close below previous day's low (PDL)
// Neutral: otherwise
func computeHTFBias(dailyBars []data.OHLCV, ltfBars []data.OHLCV, htfIndex []int) []int {
	bias := make([]int, len(ltfBars))

	// Compute daily bias array
	dailyBias := make([]int, len(dailyBars))
	for i := 1; i < len(dailyBars); i++ {
		prev := dailyBars[i-1]
		curr := dailyBars[i]

		// Method: compare current day's close relative to previous day's range
		// Bullish: current close above previous high (strong) or previous close with bullish candle
		// Bearish: current close below previous low (strong) or previous close with bearish candle
		if curr.Close > prev.High {
			dailyBias[i] = 1 // strong bullish — closed above PDH
		} else if curr.Close < prev.Low {
			dailyBias[i] = -1 // strong bearish — closed below PDL
		} else if curr.Close > curr.Open && curr.Close > prev.Close {
			dailyBias[i] = 1 // moderate bullish
		} else if curr.Close < curr.Open && curr.Close < prev.Close {
			dailyBias[i] = -1 // moderate bearish
		} else {
			dailyBias[i] = 0 // neutral / consolidation
		}
	}

	// Map daily bias to each LTF bar
	for i := range ltfBars {
		if i < len(htfIndex) && htfIndex[i] >= 0 && htfIndex[i] < len(dailyBias) {
			bias[i] = dailyBias[htfIndex[i]]
		}
	}
	return bias
}
