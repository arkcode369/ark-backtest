package data

// Symbol defines a tradeable instrument
type Symbol struct {
	Ticker      string // Yahoo Finance ticker
	Name        string
	Category    string
	TickSize    float64
	PointValue  float64 // USD per point/pip
	Description string
}

// SymbolMap is the master list of supported instruments
var SymbolMap = map[string]Symbol{
	// ── METALS ──────────────────────────────────────────────
	"XAUUSD": {Ticker: "GC=F", Name: "Gold Futures", Category: "Metals", TickSize: 0.10, PointValue: 100, Description: "COMEX Gold ($/oz)"},
	"XAGUSD": {Ticker: "SI=F", Name: "Silver Futures", Category: "Metals", TickSize: 0.005, PointValue: 5000, Description: "COMEX Silver ($/oz)"},
	"XAG":    {Ticker: "SI=F", Name: "Silver Futures", Category: "Metals", TickSize: 0.005, PointValue: 5000, Description: "COMEX Silver ($/oz)"},
	"COPPER": {Ticker: "HG=F", Name: "Copper Futures", Category: "Metals", TickSize: 0.0005, PointValue: 25000, Description: "COMEX Copper ($/lb)"},
	"PALLADIUM": {Ticker: "PA=F", Name: "Palladium Futures", Category: "Metals", TickSize: 0.05, PointValue: 100, Description: "NYMEX Palladium ($/oz)"},
	"PLATINUM": {Ticker: "PL=F", Name: "Platinum Futures", Category: "Metals", TickSize: 0.10, PointValue: 50, Description: "NYMEX Platinum ($/oz)"},

	// ── INDICES FUTURES ──────────────────────────────────────
	"NQ":  {Ticker: "NQ=F", Name: "Nasdaq-100 Futures", Category: "Indices", TickSize: 0.25, PointValue: 20, Description: "CME E-mini Nasdaq-100"},
	"ES":  {Ticker: "ES=F", Name: "S&P 500 Futures", Category: "Indices", TickSize: 0.25, PointValue: 50, Description: "CME E-mini S&P 500"},
	"YM":  {Ticker: "YM=F", Name: "Dow Jones Futures", Category: "Indices", TickSize: 1, PointValue: 5, Description: "CME E-mini Dow Jones"},
	"RTY": {Ticker: "RTY=F", Name: "Russell 2000 Futures", Category: "Indices", TickSize: 0.10, PointValue: 50, Description: "CME E-mini Russell 2000"},

	// ── FOREX ────────────────────────────────────────────────
	"EURUSD": {Ticker: "EURUSD=X", Name: "Euro/USD", Category: "Forex", TickSize: 0.00001, PointValue: 100000, Description: "EUR/USD Major"},
	"GBPUSD": {Ticker: "GBPUSD=X", Name: "GBP/USD", Category: "Forex", TickSize: 0.00001, PointValue: 100000, Description: "GBP/USD Major"},
	"USDJPY": {Ticker: "USDJPY=X", Name: "USD/JPY", Category: "Forex", TickSize: 0.001, PointValue: 1000, Description: "USD/JPY Major"},
	"USDCHF": {Ticker: "USDCHF=X", Name: "USD/CHF", Category: "Forex", TickSize: 0.00001, PointValue: 100000, Description: "USD/CHF Major"},
	"AUDUSD": {Ticker: "AUDUSD=X", Name: "AUD/USD", Category: "Forex", TickSize: 0.00001, PointValue: 100000, Description: "AUD/USD Major"},
	"NZDUSD": {Ticker: "NZDUSD=X", Name: "NZD/USD", Category: "Forex", TickSize: 0.00001, PointValue: 100000, Description: "NZD/USD Major"},
	"USDCAD": {Ticker: "USDCAD=X", Name: "USD/CAD", Category: "Forex", TickSize: 0.00001, PointValue: 100000, Description: "USD/CAD Major"},
	// DXY proxy - use UUP ETF as reference
	"DXY": {Ticker: "UUP", Name: "US Dollar Index (ETF proxy)", Category: "Forex", TickSize: 0.01, PointValue: 1, Description: "Invesco DB US Dollar Index (UUP ETF as DXY proxy)"},

	// ── ENERGY ──────────────────────────────────────────────
	"CL":  {Ticker: "CL=F", Name: "Crude Oil WTI", Category: "Energy", TickSize: 0.01, PointValue: 1000, Description: "NYMEX WTI Crude Oil ($/bbl)"},
	"RB":  {Ticker: "RB=F", Name: "RBOB Gasoline", Category: "Energy", TickSize: 0.0001, PointValue: 42000, Description: "NYMEX RBOB Gasoline ($/gal)"},
	"HO":  {Ticker: "HO=F", Name: "Heating Oil", Category: "Energy", TickSize: 0.0001, PointValue: 42000, Description: "NYMEX Heating Oil ($/gal)"},
	"NG":  {Ticker: "NG=F", Name: "Natural Gas", Category: "Energy", TickSize: 0.001, PointValue: 10000, Description: "NYMEX Natural Gas ($/MMBtu)"},
}

// ValidIntervals maps user-friendly names to Yahoo Finance interval strings
// Yahoo Finance does NOT support 4h — closest is 1h or 1d
var ValidIntervals = map[string]string{
	"1m":  "1m",
	"2m":  "2m",
	"5m":  "5m",
	"15m": "15m",
	"30m": "30m",
	"1h":  "60m",
	"60m": "60m",
	"4h":  "60m", // 4h not supported by Yahoo; fallback to 1h with a warning
	"1d":  "1d",
	"1w":  "1wk",
	"1wk": "1wk",
	"1mo": "1mo",
}

// IntervalMaxHistory defines maximum lookback period per interval (Yahoo Finance limits)
var IntervalMaxHistory = map[string]string{
	"1m":  "7d",
	"2m":  "60d",
	"5m":  "60d",
	"15m": "60d",
	"30m": "60d",
	"60m": "730d",
	"1d":  "3650d",
	"1wk": "3650d",
	"1mo": "3650d",
}

// GetSymbol looks up a symbol by user-provided key (case-insensitive)
func GetSymbol(key string) (Symbol, bool) {
	// try uppercase
	s, ok := SymbolMap[key]
	return s, ok
}

// ListByCategory returns all symbols for a given category
func ListByCategory(cat string) []Symbol {
	var out []Symbol
	for _, s := range SymbolMap {
		if s.Category == cat {
			out = append(out, s)
		}
	}
	return out
}

// AllCategories returns unique categories
func AllCategories() []string {
	seen := map[string]bool{}
	var cats []string
	for _, s := range SymbolMap {
		if !seen[s.Category] {
			seen[s.Category] = true
			cats = append(cats, s.Category)
		}
	}
	return cats
}
