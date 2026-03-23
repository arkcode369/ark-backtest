package data

import "sort"

// AlignHTFToLTF maps each lower-timeframe bar to the index of the
// most recent completed higher-timeframe bar. Returns a slice where
// htfIndex[i] is the HTF bar index active at LTF bar i, or -1 if
// no HTF bar exists before that time.
func AlignHTFToLTF(ltfBars, htfBars []OHLCV) []int {
	result := make([]int, len(ltfBars))

	if len(htfBars) == 0 {
		for i := range result {
			result[i] = -1
		}
		return result
	}

	for i, ltf := range ltfBars {
		// Binary search: find the last HTF bar with Time <= ltf.Time
		idx := sort.Search(len(htfBars), func(j int) bool {
			return htfBars[j].Time.After(ltf.Time)
		})
		// idx is the first HTF bar AFTER ltf.Time, so idx-1 is the one we want
		if idx > 0 {
			result[i] = idx - 1
		} else {
			result[i] = -1
		}
	}
	return result
}
