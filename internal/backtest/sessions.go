package backtest

import (
	"time"
	"trading-backtest-bot/internal/data"
)

// SessionType identifies trading session windows
type SessionType int

const (
	SessionNone   SessionType = 0
	SessionAsian  SessionType = 1 // 18:00-00:00 EST / 23:00-05:00 UTC
	SessionLondon SessionType = 2 // 02:00-05:00 EST / 07:00-10:00 UTC
	SessionNYAM   SessionType = 3 // 07:00-10:00 EST / 12:00-15:00 UTC (NY AM Kill Zone)
	SessionNYPM   SessionType = 4 // 13:30-16:00 EST / 18:30-21:00 UTC
)

// SessionLabel annotates a bar with its session context
type SessionLabel struct {
	Session    SessionType
	IsKillZone bool
}

// LabelSessions classifies each bar into its ICT session based on UTC hour.
// Kill zones are the specific windows ICT identifies as high-probability entry times.
func LabelSessions(bars []data.OHLCV) []SessionLabel {
	labels := make([]SessionLabel, len(bars))

	est := time.FixedZone("EST", -5*3600)

	for i, bar := range bars {
		t := bar.Time.In(est)
		h := t.Hour()
		m := t.Minute()
		totalMin := h*60 + m

		switch {
		// Asian session: 18:00-00:00 EST (CBDR: 18:00-midnight)
		case totalMin >= 18*60: // 18:00-23:59
			labels[i] = SessionLabel{Session: SessionAsian, IsKillZone: false}
		// Asian kill zone: 00:00-00:00 is midnight, but Asian extends to ~00:00

		// London Kill Zone: 02:00-05:00 EST
		case totalMin >= 2*60 && totalMin < 5*60:
			labels[i] = SessionLabel{Session: SessionLondon, IsKillZone: true}

		// NY AM Kill Zone: 07:00-10:00 EST (highest probability)
		case totalMin >= 7*60 && totalMin < 10*60:
			labels[i] = SessionLabel{Session: SessionNYAM, IsKillZone: true}

		// NY PM session: 13:30-16:00 EST
		case totalMin >= 13*60+30 && totalMin < 16*60:
			labels[i] = SessionLabel{Session: SessionNYPM, IsKillZone: false}

		default:
			labels[i] = SessionLabel{Session: SessionNone, IsKillZone: false}
		}
	}
	return labels
}

// CBDRResult holds the Asian session range (CBDR) and standard deviation projections for a given day
type CBDRResult struct {
	Date    time.Time
	High    float64 // Asian session high
	Low     float64 // Asian session low
	Range   float64 // High - Low
	StdDev1Up   float64 // High + 1*Range
	StdDev1Down float64 // Low - 1*Range
	StdDev2Up   float64 // High + 2*Range
	StdDev2Down float64 // Low - 2*Range
	StdDev3Up   float64 // High + 3*Range
	StdDev3Down float64 // Low - 3*Range
}

// ComputeCBDR calculates the CBDR (Central Bank Dealers Range) for each day in the bars.
// CBDR is defined as the Asian session range (18:00-00:00 EST).
// ICT uses the CBDR range to project standard deviations for the next day's potential range.
func ComputeCBDR(bars []data.OHLCV) []CBDRResult {
	est := time.FixedZone("EST", -5*3600)

	// Group bars by trading date and collect Asian session bars
	type dayBars struct {
		date       time.Time
		asianHigh  float64
		asianLow   float64
		hasAsian   bool
	}

	days := make(map[string]*dayBars) // keyed by date string
	var dayOrder []string

	for _, bar := range bars {
		t := bar.Time.In(est)
		h := t.Hour()

		// Asian session: 18:00-23:59 EST
		// The trading date for the Asian session is the NEXT calendar day
		// (18:00 Monday EST = Tuesday's Asian session)
		if h >= 18 {
			nextDay := t.Add(24 * time.Hour)
			dateKey := nextDay.Format("2006-01-02")
			d, exists := days[dateKey]
			if !exists {
				d = &dayBars{
					date:      nextDay.Truncate(24 * time.Hour),
					asianHigh: bar.High,
					asianLow:  bar.Low,
					hasAsian:  true,
				}
				days[dateKey] = d
				dayOrder = append(dayOrder, dateKey)
			} else {
				if bar.High > d.asianHigh {
					d.asianHigh = bar.High
				}
				if bar.Low < d.asianLow {
					d.asianLow = bar.Low
				}
				d.hasAsian = true
			}
		}
	}

	var results []CBDRResult
	for _, key := range dayOrder {
		d := days[key]
		if !d.hasAsian {
			continue
		}
		r := d.asianHigh - d.asianLow
		results = append(results, CBDRResult{
			Date:        d.date,
			High:        d.asianHigh,
			Low:         d.asianLow,
			Range:       r,
			StdDev1Up:   d.asianHigh + r,
			StdDev1Down: d.asianLow - r,
			StdDev2Up:   d.asianHigh + 2*r,
			StdDev2Down: d.asianLow - 2*r,
			StdDev3Up:   d.asianHigh + 3*r,
			StdDev3Down: d.asianLow - 3*r,
		})
	}
	return results
}

// FindCBDRForBar returns the CBDR result applicable to the given bar's trading day.
// Returns nil if no CBDR data is available for that day.
func FindCBDRForBar(cbdrs []CBDRResult, bar data.OHLCV) *CBDRResult {
	est := time.FixedZone("EST", -5*3600)
	t := bar.Time.In(est)
	// Determine trading date: if before midnight, it's the current date;
	// if 18:00+ it's the next date's session
	var dateKey string
	if t.Hour() >= 18 {
		dateKey = t.Add(24 * time.Hour).Format("2006-01-02")
	} else {
		dateKey = t.Format("2006-01-02")
	}

	for i := range cbdrs {
		if cbdrs[i].Date.Format("2006-01-02") == dateKey {
			return &cbdrs[i]
		}
	}
	return nil
}
