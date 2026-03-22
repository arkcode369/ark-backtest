package data

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

var (
	rateMu    sync.Mutex
	lastFetch time.Time
	minDelay  = 200 * time.Millisecond // max ~5 requests/second
)

func rateLimit() {
	rateMu.Lock()
	defer rateMu.Unlock()
	elapsed := time.Since(lastFetch)
	if elapsed < minDelay {
		time.Sleep(minDelay - elapsed)
	}
	lastFetch = time.Now()
}

// OHLCV is a single candlestick bar
type OHLCV struct {
	Time   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
}

// FetchParams holds parameters for data fetching
type FetchParams struct {
	Symbol   string
	Interval string // 1m, 5m, 15m, 1h, 1d ...
	Period   string // 7d, 30d, 1y ... (optional, overrides Start/End)
	Start    time.Time
	End      time.Time
}

// yahooResponse is the raw response from Yahoo Finance v8 chart API
type yahooResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				Symbol   string  `json:"symbol"`
				Currency string  `json:"currency"`
				RegPrice float64 `json:"regularMarketPrice"`
			} `json:"meta"`
			Timestamps []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Open   []interface{} `json:"open"`
					High   []interface{} `json:"high"`
					Low    []interface{} `json:"low"`
					Close  []interface{} `json:"close"`
					Volume []interface{} `json:"volume"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
		Error interface{} `json:"error"`
	} `json:"chart"`
}

func toFloat(v interface{}) (float64, bool) {
	if v == nil {
		return 0, false
	}
	switch val := v.(type) {
	case float64:
		return val, !math.IsNaN(val)
	case json.Number:
		f, err := val.Float64()
		return f, err == nil && !math.IsNaN(f)
	}
	return 0, false
}

// FetchOHLCV downloads OHLCV data from Yahoo Finance
func FetchOHLCV(ctx context.Context, params FetchParams) ([]OHLCV, error) {
	rateLimit()
	sym, ok := GetSymbol(params.Symbol)
	if !ok {
		return nil, fmt.Errorf("unknown symbol: %s", params.Symbol)
	}

	interval, ok := ValidIntervals[params.Interval]
	if !ok {
		return nil, fmt.Errorf("unsupported interval: %s (use: 1m,5m,15m,30m,1h,1d,1w)", params.Interval)
	}

	var urlStr string
	if params.Period != "" {
		urlStr = fmt.Sprintf(
			"https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=%s&range=%s&includePrePost=false",
			sym.Ticker, interval, params.Period,
		)
	} else {
		startUnix := params.Start.Unix()
		endUnix := params.End.Unix()
		urlStr = fmt.Sprintf(
			"https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=%s&period1=%d&period2=%d&includePrePost=false",
			sym.Ticker, interval, startUnix, endUnix,
		)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		preview := string(body)
		if len(preview) > 200 {
			preview = preview[:200]
		}
		return nil, fmt.Errorf("HTTP %d for %s: %s", resp.StatusCode, sym.Ticker, preview)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var yResp yahooResponse
	if err := json.Unmarshal(body, &yResp); err != nil {
		return nil, fmt.Errorf("JSON parse error: %w", err)
	}

	if yResp.Chart.Error != nil {
		return nil, fmt.Errorf("Yahoo Finance error for %s: %v", params.Symbol, yResp.Chart.Error)
	}

	if len(yResp.Chart.Result) == 0 {
		return nil, fmt.Errorf("no data returned for %s (ticker: %s)", params.Symbol, sym.Ticker)
	}

	result := yResp.Chart.Result[0]
	if len(result.Timestamps) == 0 {
		return nil, fmt.Errorf("empty data for %s", params.Symbol)
	}
	if len(result.Indicators.Quote) == 0 {
		return nil, fmt.Errorf("no quote data for %s", params.Symbol)
	}

	q := result.Indicators.Quote[0]
	var bars []OHLCV
	for i, ts := range result.Timestamps {
		o, okO := toFloat(safeGet(q.Open, i))
		h, okH := toFloat(safeGet(q.High, i))
		l, okL := toFloat(safeGet(q.Low, i))
		c, okC := toFloat(safeGet(q.Close, i))
		v, _ := toFloat(safeGet(q.Volume, i))

		if !okO || !okH || !okL || !okC {
			continue // skip bars with nil/NaN values
		}

		bars = append(bars, OHLCV{
			Time:   time.Unix(ts, 0).UTC(),
			Open:   o,
			High:   h,
			Low:    l,
			Close:  c,
			Volume: v,
		})
	}

	// Sort ascending
	sort.Slice(bars, func(i, j int) bool {
		return bars[i].Time.Before(bars[j].Time)
	})

	return bars, nil
}

func safeGet(s []interface{}, i int) interface{} {
	if i < len(s) {
		return s[i]
	}
	return nil
}

// FetchLatestPrice returns the latest close price for a symbol
func FetchLatestPrice(ctx context.Context, symbolKey string) (float64, string, error) {
	rateLimit()
	sym, ok := GetSymbol(symbolKey)
	if !ok {
		return 0, "", fmt.Errorf("unknown symbol: %s", symbolKey)
	}
	urlStr := fmt.Sprintf(
		"https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=1d&range=2d",
		sym.Ticker,
	)
	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return 0, "", fmt.Errorf("build request error: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		preview := string(body)
		if len(preview) > 200 {
			preview = preview[:200]
		}
		return 0, "", fmt.Errorf("HTTP %d for %s: %s", resp.StatusCode, sym.Ticker, preview)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, "", fmt.Errorf("read body error: %w", err)
	}
	var yResp yahooResponse
	if err := json.Unmarshal(body, &yResp); err != nil {
		return 0, "", fmt.Errorf("JSON parse error: %w", err)
	}

	if yResp.Chart.Error != nil {
		return 0, "", fmt.Errorf("Yahoo Finance error for %s: %v", symbolKey, yResp.Chart.Error)
	}

	if len(yResp.Chart.Result) == 0 {
		return 0, "", fmt.Errorf("no data for %s (ticker: %s)", symbolKey, sym.Ticker)
	}
	price := yResp.Chart.Result[0].Meta.RegPrice
	currency := yResp.Chart.Result[0].Meta.Currency
	return price, currency, nil
}

// FormatNumber formats a float with given decimals
func FormatNumber(f float64, decimals int) string {
	return strconv.FormatFloat(f, 'f', decimals, 64)
}
