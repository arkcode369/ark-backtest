package backtest

import "trading-backtest-bot/internal/data"

// MultiTimeframeStrategy is optionally implemented by strategies
// that need higher-timeframe context for bias determination.
type MultiTimeframeStrategy interface {
	Strategy
	// Timeframes returns the additional timeframe intervals needed (e.g., ["1d"])
	Timeframes() []string
	// InitMTF is called after Init() with bars grouped by timeframe
	InitMTF(barsByTF map[string][]data.OHLCV, params map[string]float64)
}

// MultiSymbolStrategy is optionally implemented by strategies
// that need correlated-symbol data (e.g., for SMT divergence).
type MultiSymbolStrategy interface {
	Strategy
	// Symbols returns additional symbol keys needed (e.g., ["NQ"])
	Symbols() []string
	// InitMultiSymbol is called after Init() with bars grouped by symbol
	InitMultiSymbol(barsBySymbol map[string][]data.OHLCV, params map[string]float64)
}

// SessionAwareStrategy is optionally implemented by strategies
// that need session/timezone annotations on bars (Kill Zones, CBDR, etc).
type SessionAwareStrategy interface {
	Strategy
	// InitSessions is called after Init() with session labels for each bar
	InitSessions(sessions []SessionLabel)
}
