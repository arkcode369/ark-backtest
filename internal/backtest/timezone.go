package backtest

import "time"

// estLoc is the shared Eastern Time location used across all session-related
// code. It correctly handles both EST and EDT transitions, unlike a fixed
// UTC-5 offset which ignores daylight saving time for ~8 months of the year.
var estLoc *time.Location

func init() {
	var err error
	estLoc, err = time.LoadLocation("America/New_York")
	if err != nil {
		// Fallback for environments without timezone database
		estLoc = time.FixedZone("EST", -5*3600)
	}
}
