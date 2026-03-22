package backtest

import (
	"fmt"
	"math"
	"strings"
)

// FormatEquityCurve renders an ASCII chart of the equity curve.
// width is the number of columns, height is the number of rows.
func FormatEquityCurve(curve []float64, width, height int) string {
	if len(curve) < 2 || width < 10 || height < 3 {
		return ""
	}

	// Downsample to fit width
	sampled := downsample(curve, width)

	// Find min/max
	minVal, maxVal := sampled[0], sampled[0]
	for _, v := range sampled {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	// Handle flat line
	if maxVal == minVal {
		maxVal = minVal + 1
	}

	// Build grid
	grid := make([][]rune, height)
	for i := range grid {
		grid[i] = make([]rune, len(sampled))
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}

	// Plot values
	for col, val := range sampled {
		// Map value to row (0 = top = maxVal, height-1 = bottom = minVal)
		row := int(math.Round(float64(height-1) * (maxVal - val) / (maxVal - minVal)))
		if row < 0 {
			row = 0
		}
		if row >= height {
			row = height - 1
		}
		grid[row][col] = '\u2588' // full block character
	}

	// Connect vertical gaps between consecutive columns
	for col := 1; col < len(sampled); col++ {
		prevRow := int(math.Round(float64(height-1) * (maxVal - sampled[col-1]) / (maxVal - minVal)))
		currRow := int(math.Round(float64(height-1) * (maxVal - sampled[col]) / (maxVal - minVal)))
		if prevRow < 0 {
			prevRow = 0
		}
		if currRow < 0 {
			currRow = 0
		}
		if prevRow >= height {
			prevRow = height - 1
		}
		if currRow >= height {
			currRow = height - 1
		}
		lo, hi := prevRow, currRow
		if lo > hi {
			lo, hi = hi, lo
		}
		for r := lo; r <= hi; r++ {
			if grid[r][col] == ' ' {
				grid[r][col] = '\u2502' // vertical bar for fill
			}
		}
	}

	// Render with axis labels
	var sb strings.Builder
	sb.WriteString("Equity Curve:\n")
	for row := 0; row < height; row++ {
		// Y-axis labels at top, middle, bottom
		val := maxVal - float64(row)*(maxVal-minVal)/float64(height-1)
		if row == 0 || row == height-1 || row == height/2 {
			sb.WriteString(fmt.Sprintf("%8.0f |", val))
		} else {
			sb.WriteString("         |")
		}
		sb.WriteString(string(grid[row]))
		sb.WriteString("\n")
	}
	// X-axis
	sb.WriteString("         +")
	sb.WriteString(strings.Repeat("\u2500", len(sampled)))
	sb.WriteString("\n")

	// Summary line
	startVal := curve[0]
	endVal := curve[len(curve)-1]
	changePct := (endVal - startVal) / startVal * 100
	arrow := "\u25b2" // up arrow
	if changePct < 0 {
		arrow = "\u25bc" // down arrow
	}
	sb.WriteString(fmt.Sprintf("         $%.0f -> $%.0f (%s%.1f%%)", startVal, endVal, arrow, changePct))

	return sb.String()
}

// downsample reduces data points to fit the target width using averaging
func downsample(data []float64, targetWidth int) []float64 {
	n := len(data)
	if n <= targetWidth {
		return data
	}

	result := make([]float64, targetWidth)
	bucketSize := float64(n) / float64(targetWidth)
	for i := 0; i < targetWidth; i++ {
		start := int(float64(i) * bucketSize)
		end := int(float64(i+1) * bucketSize)
		if end > n {
			end = n
		}
		if start >= end {
			start = end - 1
		}
		sum := 0.0
		for j := start; j < end; j++ {
			sum += data[j]
		}
		result[i] = sum / float64(end-start)
	}
	return result
}
