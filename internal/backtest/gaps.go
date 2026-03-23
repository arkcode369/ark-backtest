package backtest

import (
	"math"
	"time"
	"trading-backtest-bot/internal/data"
)

// GapType identifies the type of opening gap
type GapType int

const (
	NDOG GapType = 1 // New Day Opening Gap
	NWOG GapType = 2 // New Week Opening Gap
)

// OpeningGap represents a gap between session close and open
type OpeningGap struct {
	Type      GapType
	Date      time.Time
	High      float64 // top of gap (max of close, open)
	Low       float64 // bottom of gap (min of close, open)
	MidPoint  float64 // (High + Low) / 2
	IsBullish bool    // true if open > previous close (gap up)
}

// DetectNDOG finds New Day Opening Gaps by comparing each day's
// first bar open against the previous day's last bar close.
func DetectNDOG(bars []data.OHLCV) []OpeningGap {
	if len(bars) < 2 {
		return nil
	}

	var gaps []OpeningGap

	// Group bars by calendar date (UTC)
	type dayData struct {
		firstOpen float64
		lastClose float64
		date      time.Time
	}

	var days []dayData
	var current *dayData

	for _, bar := range bars {
		dateKey := bar.Time.Truncate(24 * time.Hour)
		if current == nil || !current.date.Equal(dateKey) {
			if current != nil {
				days = append(days, *current)
			}
			current = &dayData{
				firstOpen: bar.Open,
				lastClose: bar.Close,
				date:      dateKey,
			}
		} else {
			current.lastClose = bar.Close
		}
	}
	if current != nil {
		days = append(days, *current)
	}

	// Find gaps between consecutive days
	for i := 1; i < len(days); i++ {
		prevClose := days[i-1].lastClose
		currOpen := days[i].firstOpen

		gapSize := math.Abs(currOpen - prevClose)
		if gapSize < 1e-10 {
			continue // no meaningful gap
		}

		high := math.Max(prevClose, currOpen)
		low := math.Min(prevClose, currOpen)

		gaps = append(gaps, OpeningGap{
			Type:      NDOG,
			Date:      days[i].date,
			High:      high,
			Low:       low,
			MidPoint:  (high + low) / 2,
			IsBullish: currOpen > prevClose,
		})
	}
	return gaps
}

// DetectNWOG finds New Week Opening Gaps by comparing Friday's close
// to Monday's (or Sunday's) open. Requires daily bars for reliable detection.
func DetectNWOG(bars []data.OHLCV) []OpeningGap {
	if len(bars) < 2 {
		return nil
	}

	var gaps []OpeningGap

	for i := 1; i < len(bars); i++ {
		prevDay := bars[i-1].Time.Weekday()
		currDay := bars[i].Time.Weekday()

		// Detect week boundary: previous bar is Friday (or later) and current is Monday (or Sunday evening)
		isWeekGap := false
		if prevDay == time.Friday && (currDay == time.Monday || currDay == time.Sunday) {
			isWeekGap = true
		}
		// Also catch Thursday->Monday gaps (holidays)
		if prevDay < currDay && currDay == time.Monday {
			// Check if more than 2 days between bars
			dayDiff := bars[i].Time.Sub(bars[i-1].Time).Hours() / 24
			if dayDiff > 2 {
				isWeekGap = true
			}
		}
		// Daily bars: check gap between consecutive bars that span a weekend
		dayDiff := bars[i].Time.Sub(bars[i-1].Time).Hours() / 24
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

		gaps = append(gaps, OpeningGap{
			Type:      NWOG,
			Date:      bars[i].Time,
			High:      high,
			Low:       low,
			MidPoint:  (high + low) / 2,
			IsBullish: currOpen > prevClose,
		})
	}
	return gaps
}

// FindGapForBar returns all gaps (NDOG/NWOG) that are relevant to the given bar's date.
// A gap is relevant if the bar's date matches the gap's date.
func FindGapForBar(gaps []OpeningGap, bar data.OHLCV) []OpeningGap {
	barDate := bar.Time.Truncate(24 * time.Hour)
	var relevant []OpeningGap
	for _, g := range gaps {
		gapDate := g.Date.Truncate(24 * time.Hour)
		if gapDate.Equal(barDate) {
			relevant = append(relevant, g)
		}
	}
	return relevant
}
