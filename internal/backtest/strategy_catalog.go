package backtest

// ── Strategy Catalog ─────────────────────────────────────────────────────
// Rich metadata for strategy overview, parameter descriptions, presets,
// and recommended usage. Used by the bot to render /strategies and /si menus.

// ParamInfo describes a single strategy parameter for display purposes.
type ParamInfo struct {
	Key         string
	Label       string  // human-readable label
	Description string  // what it does
	Default     float64 // default value
}

// Preset is a named parameter set (e.g., "conservative", "aggressive").
type Preset struct {
	Name   string
	Label  string             // e.g., "Conservative"
	Params map[string]float64 // override values
}

// CatalogEntry contains rich display metadata for a strategy.
type CatalogEntry struct {
	Key            string // registry key (e.g., "ema_cross")
	Category       string // "Classic" or "ICT"
	Emoji          string
	ShortDesc      string // one-liner for /strategies list
	HowItWorks     string // multi-line explanation for /si
	WhenToUse      string // market conditions / instruments
	RecommendedTF  string // recommended timeframes
	ParamDetails   []ParamInfo
	Presets        []Preset
	ExampleCommand string // copy-paste ready example
}

// StrategyCatalog holds display metadata for all strategies, keyed by registry key.
var StrategyCatalog = map[string]CatalogEntry{

	// ── Classic Strategies ──────────────────────────────────────────────

	"ema_cross": {
		Key:       "ema_cross",
		Category:  "Classic",
		Emoji:     "📊",
		ShortDesc: "Buy/sell when fast EMA crosses slow EMA",
		HowItWorks: "Computes two Exponential Moving Averages (fast & slow). " +
			"Generates a BUY when the fast EMA crosses above the slow EMA (golden cross), " +
			"and a SELL when it crosses below (death cross). Works best in trending markets.",
		WhenToUse:     "Trending markets (forex, indices, metals). Avoid during tight ranges.",
		RecommendedTF: "1h, 1d",
		ParamDetails: []ParamInfo{
			{Key: "fast", Label: "Fast EMA", Description: "Period for the fast (responsive) EMA", Default: 9},
			{Key: "slow", Label: "Slow EMA", Description: "Period for the slow (smoothed) EMA", Default: 21},
		},
		Presets: []Preset{
			{Name: "scalp", Label: "Scalp (5/13)", Params: map[string]float64{"fast": 5, "slow": 13}},
			{Name: "default", Label: "Default (9/21)", Params: map[string]float64{"fast": 9, "slow": 21}},
			{Name: "swing", Label: "Swing (21/55)", Params: map[string]float64{"fast": 21, "slow": 55}},
		},
		ExampleCommand: "/backtest XAUUSD 1d ema_cross fast=9 slow=21",
	},

	"rsi": {
		Key:       "rsi",
		Category:  "Classic",
		Emoji:     "📉",
		ShortDesc: "Mean reversion on RSI overbought/oversold levels",
		HowItWorks: "Computes the Relative Strength Index (RSI). " +
			"Generates a BUY when RSI crosses above the oversold level (e.g., 30), " +
			"and a SELL when RSI crosses below the overbought level (e.g., 70). " +
			"Captures reversals at extreme momentum readings.",
		WhenToUse:     "Range-bound or mean-reverting markets. Less effective in strong trends.",
		RecommendedTF: "1h, 1d",
		ParamDetails: []ParamInfo{
			{Key: "period", Label: "RSI Period", Description: "Lookback period for RSI calculation", Default: 14},
			{Key: "overbought", Label: "Overbought", Description: "Level above which market is overbought", Default: 70},
			{Key: "oversold", Label: "Oversold", Description: "Level below which market is oversold", Default: 30},
		},
		Presets: []Preset{
			{Name: "tight", Label: "Tight (80/20)", Params: map[string]float64{"period": 14, "overbought": 80, "oversold": 20}},
			{Name: "default", Label: "Default (70/30)", Params: map[string]float64{"period": 14, "overbought": 70, "oversold": 30}},
			{Name: "wide", Label: "Wide (60/40)", Params: map[string]float64{"period": 14, "overbought": 60, "oversold": 40}},
		},
		ExampleCommand: "/backtest XAUUSD 1h rsi period=14",
	},

	"macd": {
		Key:       "macd",
		Category:  "Classic",
		Emoji:     "📈",
		ShortDesc: "MACD line crosses signal line for trend entries",
		HowItWorks: "Computes MACD (fast EMA - slow EMA) and a signal line (EMA of MACD). " +
			"BUY when MACD crosses above signal, SELL when it crosses below. " +
			"Combines trend-following with momentum confirmation.",
		WhenToUse:     "Trending markets. Good for indices and forex majors.",
		RecommendedTF: "1h, 1d",
		ParamDetails: []ParamInfo{
			{Key: "fast", Label: "Fast EMA", Description: "Fast EMA period for MACD", Default: 12},
			{Key: "slow", Label: "Slow EMA", Description: "Slow EMA period for MACD", Default: 26},
			{Key: "signal", Label: "Signal", Description: "Signal line EMA period", Default: 9},
		},
		Presets: []Preset{
			{Name: "fast", Label: "Fast (8/17/9)", Params: map[string]float64{"fast": 8, "slow": 17, "signal": 9}},
			{Name: "default", Label: "Default (12/26/9)", Params: map[string]float64{"fast": 12, "slow": 26, "signal": 9}},
			{Name: "slow", Label: "Slow (19/39/9)", Params: map[string]float64{"fast": 19, "slow": 39, "signal": 9}},
		},
		ExampleCommand: "/backtest EURUSD 1d macd",
	},

	"bb_breakout": {
		Key:       "bb_breakout",
		Category:  "Classic",
		Emoji:     "🎯",
		ShortDesc: "Breakout when price exits Bollinger Bands",
		HowItWorks: "Computes Bollinger Bands (SMA +/- N standard deviations). " +
			"BUY when price breaks above the upper band, SELL when it breaks below the lower band. " +
			"Captures volatility expansion breakouts.",
		WhenToUse:     "Volatility breakout setups. Works on all instruments.",
		RecommendedTF: "1h, 1d",
		ParamDetails: []ParamInfo{
			{Key: "period", Label: "BB Period", Description: "SMA lookback period", Default: 20},
			{Key: "std", Label: "Std Dev", Description: "Number of standard deviations for bands", Default: 2.0},
		},
		Presets: []Preset{
			{Name: "tight", Label: "Tight bands (20/1.5)", Params: map[string]float64{"period": 20, "std": 1.5}},
			{Name: "default", Label: "Default (20/2.0)", Params: map[string]float64{"period": 20, "std": 2.0}},
			{Name: "wide", Label: "Wide bands (20/2.5)", Params: map[string]float64{"period": 20, "std": 2.5}},
		},
		ExampleCommand: "/backtest NQ 1d bb_breakout",
	},

	"supertrend": {
		Key:       "supertrend",
		Category:  "Classic",
		Emoji:     "🔄",
		ShortDesc: "Trend direction change via ATR-based Supertrend",
		HowItWorks: "Computes Supertrend indicator using ATR-based trailing stop. " +
			"BUY when Supertrend flips from bearish to bullish (price breaks above trailing stop), " +
			"SELL on the opposite flip. Clean trend-following with built-in volatility adaptation.",
		WhenToUse:     "Trending instruments. Effective on metals, indices, and energy.",
		RecommendedTF: "1h, 1d",
		ParamDetails: []ParamInfo{
			{Key: "period", Label: "ATR Period", Description: "ATR lookback period", Default: 10},
			{Key: "multiplier", Label: "Multiplier", Description: "ATR multiplier for band distance", Default: 3.0},
		},
		Presets: []Preset{
			{Name: "sensitive", Label: "Sensitive (7/2.0)", Params: map[string]float64{"period": 7, "multiplier": 2.0}},
			{Name: "default", Label: "Default (10/3.0)", Params: map[string]float64{"period": 10, "multiplier": 3.0}},
			{Name: "smooth", Label: "Smooth (14/4.0)", Params: map[string]float64{"period": 14, "multiplier": 4.0}},
		},
		ExampleCommand: "/backtest XAUUSD 1d supertrend",
	},

	"donchian": {
		Key:       "donchian",
		Category:  "Classic",
		Emoji:     "🐢",
		ShortDesc: "Turtle Trading N-period high/low breakout",
		HowItWorks: "Tracks the highest high and lowest low over N periods (Donchian Channel). " +
			"BUY when price breaks above the N-period high, SELL when it breaks below the N-period low. " +
			"The original Turtle Trading system — pure trend-following breakout.",
		WhenToUse:     "Strong trending markets. Classic for commodities and futures.",
		RecommendedTF: "1d",
		ParamDetails: []ParamInfo{
			{Key: "period", Label: "Channel Period", Description: "Lookback period for high/low channel", Default: 20},
		},
		Presets: []Preset{
			{Name: "short", Label: "Short-term (10)", Params: map[string]float64{"period": 10}},
			{Name: "default", Label: "Default (20)", Params: map[string]float64{"period": 20}},
			{Name: "long", Label: "Long-term (55)", Params: map[string]float64{"period": 55}},
		},
		ExampleCommand: "/backtest CL 1d donchian period=20",
	},

	"sma_rsi": {
		Key:       "sma_rsi",
		Category:  "Classic",
		Emoji:     "🔗",
		ShortDesc: "SMA trend filter + RSI momentum confirmation",
		HowItWorks: "Combines a Simple Moving Average (trend filter) with RSI (momentum). " +
			"BUY when price is above SMA AND RSI exits oversold. " +
			"SELL when price is below SMA AND RSI exits overbought. " +
			"Reduces false signals by requiring dual confirmation.",
		WhenToUse:     "All markets. Filters out counter-trend RSI signals.",
		RecommendedTF: "1h, 1d",
		ParamDetails: []ParamInfo{
			{Key: "sma_period", Label: "SMA Period", Description: "Trend filter SMA lookback", Default: 50},
			{Key: "rsi_period", Label: "RSI Period", Description: "RSI calculation period", Default: 14},
			{Key: "overbought", Label: "Overbought", Description: "RSI overbought threshold", Default: 70},
			{Key: "oversold", Label: "Oversold", Description: "RSI oversold threshold", Default: 30},
		},
		Presets: []Preset{
			{Name: "fast", Label: "Fast (20/7)", Params: map[string]float64{"sma_period": 20, "rsi_period": 7, "overbought": 70, "oversold": 30}},
			{Name: "default", Label: "Default (50/14)", Params: map[string]float64{"sma_period": 50, "rsi_period": 14, "overbought": 70, "oversold": 30}},
			{Name: "conservative", Label: "Conservative (100/14)", Params: map[string]float64{"sma_period": 100, "rsi_period": 14, "overbought": 75, "oversold": 25}},
		},
		ExampleCommand: "/backtest GBPUSD 1d sma_rsi",
	},

	// ── ICT Strategies ─────────────────────────────────────────────────

	"ict2022": {
		Key:       "ict2022",
		Category:  "ICT",
		Emoji:     "🏛️",
		ShortDesc: "Liquidity sweep → FVG → MSS → OTE entry (full ICT model)",
		HowItWorks: "The core ICT 2022 Mentorship model in 6 steps:\n" +
			"1. Detect swing high/low (liquidity pool)\n" +
			"2. Find liquidity grab (price sweeps past swing, then reverses)\n" +
			"3. Identify displacement candle (strong body > ATR) + Fair Value Gap\n" +
			"4. Confirm Market Structure Shift (break prior swing)\n" +
			"5. Validate FVG sits at/below 50% fib retracement\n" +
			"6. Entry when price retraces into the FVG zone",
		WhenToUse:     "All markets. Best on liquid instruments (XAUUSD, NQ, ES, EURUSD). Avoid low-volume periods.",
		RecommendedTF: "5m, 15m, 1h",
		ParamDetails: []ParamInfo{
			{Key: "swing_period", Label: "Swing Period", Description: "Bars needed to confirm a swing high/low", Default: 5},
			{Key: "atr_period", Label: "ATR Period", Description: "ATR period for displacement measurement", Default: 14},
			{Key: "disp_mult", Label: "Displacement Mult", Description: "Min displacement = disp_mult x ATR", Default: 1.5},
			{Key: "body_ratio", Label: "Body Ratio", Description: "Min candle body / range ratio (0-1)", Default: 0.6},
			{Key: "fvg_fib_valid", Label: "Fib Validation", Description: "1=require FVG below 50% fib, 0=skip", Default: 1},
			{Key: "lookback", Label: "Lookback", Description: "Max bars to look back for setups", Default: 30},
		},
		Presets: []Preset{
			{Name: "sensitive", Label: "Sensitive (more trades)", Params: map[string]float64{"swing_period": 3, "atr_period": 10, "disp_mult": 1.0, "body_ratio": 0.5, "fvg_fib_valid": 0, "lookback": 40}},
			{Name: "default", Label: "Default (balanced)", Params: map[string]float64{"swing_period": 5, "atr_period": 14, "disp_mult": 1.5, "body_ratio": 0.6, "fvg_fib_valid": 1, "lookback": 30}},
			{Name: "strict", Label: "Strict (fewer, higher quality)", Params: map[string]float64{"swing_period": 7, "atr_period": 14, "disp_mult": 2.0, "body_ratio": 0.7, "fvg_fib_valid": 1, "lookback": 20}},
		},
		ExampleCommand: "/backtest XAUUSD 15m ict2022 sl=0.005 tp=0.01",
	},

	"silver_bullet": {
		Key:       "silver_bullet",
		Category:  "ICT",
		Emoji:     "🔫",
		ShortDesc: "Scalp entries in 3 daily ICT kill-zone windows",
		HowItWorks: "Identifies ICT Silver Bullet setups in three 1-hour windows (EST):\n" +
			"- London: 03:00-04:00\n" +
			"- NY AM: 10:00-11:00\n" +
			"- NY PM: 14:00-15:00\n\n" +
			"Within each window, looks for:\n" +
			"1. Liquidity sweep (stop run past recent swing)\n" +
			"2. Fair Value Gap formation from displacement\n" +
			"3. Entry on retrace into FVG within the same window",
		WhenToUse:     "Intraday scalping on liquid instruments. Requires 5m or 15m data with enough history.",
		RecommendedTF: "5m, 15m",
		ParamDetails: []ParamInfo{
			{Key: "swing_period", Label: "Swing Period", Description: "Bars to confirm swing high/low", Default: 5},
			{Key: "atr_period", Label: "ATR Period", Description: "ATR period for displacement", Default: 14},
			{Key: "disp_mult", Label: "Displacement Mult", Description: "Min displacement = mult x ATR", Default: 1.0},
			{Key: "body_ratio", Label: "Body Ratio", Description: "Min body/range ratio for displacement", Default: 0.5},
			{Key: "sb_lookback", Label: "SB Lookback", Description: "Bars to scan for sweep within window", Default: 20},
		},
		Presets: []Preset{
			{Name: "aggressive", Label: "Aggressive (more setups)", Params: map[string]float64{"swing_period": 3, "disp_mult": 0.7, "body_ratio": 0.4, "sb_lookback": 25}},
			{Name: "default", Label: "Default", Params: map[string]float64{"swing_period": 5, "disp_mult": 1.0, "body_ratio": 0.5, "sb_lookback": 20}},
			{Name: "conservative", Label: "Conservative", Params: map[string]float64{"swing_period": 5, "disp_mult": 1.5, "body_ratio": 0.6, "sb_lookback": 15}},
		},
		ExampleCommand: "/backtest XAUUSD 5m silver_bullet sl=0.003 tp=0.006",
	},

	"turtle_soup": {
		Key:       "turtle_soup",
		Category:  "ICT",
		Emoji:     "🐢",
		ShortDesc: "False breakout (stop hunt) → displacement reversal",
		HowItWorks: "Detects stop runs where price sweeps past a swing high/low to grab liquidity, " +
			"then reverses with displacement. Optionally targets 'equal highs/lows' (clustered stops).\n\n" +
			"Setup:\n" +
			"1. Identify swing high/low (or equal highs/lows cluster)\n" +
			"2. Price sweeps past the level (stop hunt)\n" +
			"3. Strong displacement candle in opposite direction\n" +
			"4. Entry on displacement or next bar",
		WhenToUse:     "Session opens, news events, anywhere stop clusters form. All markets.",
		RecommendedTF: "5m, 15m, 1h",
		ParamDetails: []ParamInfo{
			{Key: "swing_period", Label: "Swing Period", Description: "Bars to confirm swing point", Default: 5},
			{Key: "atr_period", Label: "ATR Period", Description: "ATR period for measuring displacement", Default: 14},
			{Key: "disp_mult", Label: "Displacement Mult", Description: "Min displacement = mult x ATR", Default: 1.5},
			{Key: "body_ratio", Label: "Body Ratio", Description: "Min body/range ratio", Default: 0.6},
			{Key: "ts_lookback", Label: "Lookback", Description: "Bars to scan for sweep setups", Default: 30},
			{Key: "equal_tol", Label: "Equal Tolerance", Description: "Max distance (x ATR) for equal H/L", Default: 0.1},
			{Key: "require_equal", Label: "Require Equal", Description: "1=only trade equal H/L setups, 0=all", Default: 0},
		},
		Presets: []Preset{
			{Name: "all_sweeps", Label: "All sweeps", Params: map[string]float64{"require_equal": 0, "disp_mult": 1.0, "body_ratio": 0.5}},
			{Name: "default", Label: "Default", Params: map[string]float64{"require_equal": 0, "disp_mult": 1.5, "body_ratio": 0.6}},
			{Name: "equal_only", Label: "Equal H/L only (high confluence)", Params: map[string]float64{"require_equal": 1, "disp_mult": 1.5, "body_ratio": 0.6, "equal_tol": 0.1}},
		},
		ExampleCommand: "/backtest XAUUSD 15m turtle_soup sl=0.005 tp=0.01",
	},

	"amd_session": {
		Key:       "amd_session",
		Category:  "ICT",
		Emoji:     "🔄",
		ShortDesc: "Accumulation (Asia) → Manipulation (London) → Distribution (NY)",
		HowItWorks: "Implements the ICT AMD daily cycle:\n\n" +
			"1. ACCUMULATION: Measures Asian session range (19:00-00:00 EST)\n" +
			"2. MANIPULATION: Detects London false breakout of Asian range\n" +
			"   - London breaks below Asian low = bullish manipulation\n" +
			"   - London breaks above Asian high = bearish manipulation\n" +
			"3. DISTRIBUTION: NY session displacement in opposite direction of manipulation\n\n" +
			"Only generates signals during NY session (07:00-16:00 EST).",
		WhenToUse:     "Forex pairs, XAUUSD, indices. Requires intraday data spanning multiple sessions.",
		RecommendedTF: "5m, 15m, 30m",
		ParamDetails: []ParamInfo{
			{Key: "swing_period", Label: "Swing Period", Description: "Swing detection period", Default: 5},
			{Key: "atr_period", Label: "ATR Period", Description: "ATR for displacement measurement", Default: 14},
			{Key: "disp_mult", Label: "Displacement Mult", Description: "Min displacement strength", Default: 1.0},
			{Key: "body_ratio", Label: "Body Ratio", Description: "Min body/range ratio", Default: 0.5},
		},
		Presets: []Preset{
			{Name: "sensitive", Label: "Sensitive", Params: map[string]float64{"disp_mult": 0.7, "body_ratio": 0.4}},
			{Name: "default", Label: "Default", Params: map[string]float64{"disp_mult": 1.0, "body_ratio": 0.5}},
			{Name: "strict", Label: "Strict", Params: map[string]float64{"disp_mult": 1.5, "body_ratio": 0.6}},
		},
		ExampleCommand: "/backtest EURUSD 15m amd_session sl=0.003 tp=0.006",
	},

	"ob_retest": {
		Key:       "ob_retest",
		Category:  "ICT",
		Emoji:     "🧱",
		ShortDesc: "Enter on retrace into unmitigated Order Block zone",
		HowItWorks: "Detects Order Blocks (the last opposing candle before a displacement move) " +
			"using the PD Array detection engine. Enters when price retraces back to an unmitigated OB zone.\n\n" +
			"Filters:\n" +
			"- OB must be unmitigated (price hasn't fully traded through it)\n" +
			"- min_ob_age: OB must be at least N bars old\n" +
			"- Premium/Discount filter: only buy in discount, sell in premium",
		WhenToUse:     "All markets. Higher timeframes for swing trades, lower for scalps.",
		RecommendedTF: "15m, 1h, 1d",
		ParamDetails: []ParamInfo{
			{Key: "swing_period", Label: "Swing Period", Description: "Swing detection period", Default: 5},
			{Key: "atr_period", Label: "ATR Period", Description: "ATR period", Default: 14},
			{Key: "ob_impulse", Label: "OB Impulse", Description: "Min impulse strength (x ATR) for OB", Default: 1.5},
			{Key: "min_ob_age", Label: "Min OB Age", Description: "Min bars since OB formed", Default: 5},
			{Key: "pd_lookback", Label: "PD Lookback", Description: "Bars for premium/discount calc", Default: 50},
			{Key: "use_pd_filter", Label: "PD Filter", Description: "1=require premium/discount alignment, 0=off", Default: 1},
		},
		Presets: []Preset{
			{Name: "aggressive", Label: "Aggressive (fresh OBs)", Params: map[string]float64{"min_ob_age": 2, "ob_impulse": 1.0, "use_pd_filter": 0}},
			{Name: "default", Label: "Default", Params: map[string]float64{"min_ob_age": 5, "ob_impulse": 1.5, "use_pd_filter": 1}},
			{Name: "strict", Label: "Strict (aged OBs + PD filter)", Params: map[string]float64{"min_ob_age": 10, "ob_impulse": 2.0, "use_pd_filter": 1}},
		},
		ExampleCommand: "/backtest XAUUSD 1h ob_retest sl=0.005 tp=0.01",
	},

	"weekly_profile": {
		Key:       "weekly_profile",
		Category:  "ICT",
		Emoji:     "📅",
		ShortDesc: "Trade bounces off weekly open using PWH/PWL bias",
		HowItWorks: "Implements the ICT Weekly Profile concept:\n\n" +
			"1. Compute weekly levels: Weekly Open, Previous Week High (PWH), Previous Week Low (PWL)\n" +
			"2. Determine directional bias from PWH/PWL\n" +
			"   - Price > weekly open = bullish bias\n" +
			"   - Price < weekly open = bearish bias\n" +
			"3. Enter when price retraces to touch weekly open with displacement in bias direction\n\n" +
			"Ideal for catching the Wednesday/Thursday weekly continuation.",
		WhenToUse:     "Forex, metals, indices. Needs data spanning multiple weeks.",
		RecommendedTF: "1h, 1d",
		ParamDetails: []ParamInfo{
			{Key: "swing_period", Label: "Swing Period", Description: "Swing detection period", Default: 5},
			{Key: "atr_period", Label: "ATR Period", Description: "ATR for displacement", Default: 14},
			{Key: "disp_mult", Label: "Displacement Mult", Description: "Min displacement strength", Default: 1.0},
			{Key: "body_ratio", Label: "Body Ratio", Description: "Min body/range ratio", Default: 0.5},
			{Key: "touch_atr", Label: "Touch ATR", Description: "Max distance to weekly open (x ATR)", Default: 0.5},
		},
		Presets: []Preset{
			{Name: "wide", Label: "Wide touch zone", Params: map[string]float64{"touch_atr": 1.0, "disp_mult": 0.8}},
			{Name: "default", Label: "Default", Params: map[string]float64{"touch_atr": 0.5, "disp_mult": 1.0}},
			{Name: "tight", Label: "Tight touch zone", Params: map[string]float64{"touch_atr": 0.3, "disp_mult": 1.5}},
		},
		ExampleCommand: "/backtest XAUUSD 1h weekly_profile sl=0.005 tp=0.01",
	},

	"breaker_entry": {
		Key:       "breaker_entry",
		Category:  "ICT",
		Emoji:     "💥",
		ShortDesc: "Enter on retest of failed (flipped) Order Block",
		HowItWorks: "Detects Breaker Blocks — Order Blocks that failed and flipped direction:\n\n" +
			"1. A bearish OB forms (last bullish candle before selloff)\n" +
			"2. Price later breaks ABOVE the OB top (invalidating bearish OB)\n" +
			"3. The failed bearish OB becomes a BULLISH Breaker zone\n" +
			"4. Entry when price retraces back to the breaker zone\n\n" +
			"Breakers are considered high-probability because the OB failure confirms a structural shift.",
		WhenToUse:     "All markets. Breakers form after trend changes.",
		RecommendedTF: "15m, 1h, 1d",
		ParamDetails: []ParamInfo{
			{Key: "swing_period", Label: "Swing Period", Description: "Swing detection period", Default: 5},
			{Key: "atr_period", Label: "ATR Period", Description: "ATR period", Default: 14},
			{Key: "ob_impulse", Label: "OB Impulse", Description: "Min impulse for original OB", Default: 1.5},
			{Key: "min_age", Label: "Min Breaker Age", Description: "Min bars since breaker formed", Default: 3},
			{Key: "pd_lookback", Label: "PD Lookback", Description: "Premium/discount calculation lookback", Default: 50},
			{Key: "use_pd_filter", Label: "PD Filter", Description: "1=require PD alignment, 0=off", Default: 0},
		},
		Presets: []Preset{
			{Name: "aggressive", Label: "Aggressive (young breakers)", Params: map[string]float64{"min_age": 1, "ob_impulse": 1.0}},
			{Name: "default", Label: "Default", Params: map[string]float64{"min_age": 3, "ob_impulse": 1.5}},
			{Name: "strict", Label: "Strict (aged + PD filter)", Params: map[string]float64{"min_age": 5, "ob_impulse": 2.0, "use_pd_filter": 1}},
		},
		ExampleCommand: "/backtest NQ 1h breaker_entry sl=0.005 tp=0.01",
	},

	"cbdr_std": {
		Key:       "cbdr_std",
		Category:  "ICT",
		Emoji:     "📐",
		ShortDesc: "Mean reversion at Asian range standard deviation levels",
		HowItWorks: "Uses the ICT CBDR (Central Bank Dealers Range) concept:\n\n" +
			"1. Measure the Asian session range (CBDR high/low)\n" +
			"2. Project standard deviation levels from CBDR\n" +
			"   - STD 1 = 1x range from CBDR edge\n" +
			"   - STD 2 = 2x range from CBDR edge\n" +
			"3. Enter when price reaches STD level with displacement reversal\n\n" +
			"Based on the concept that 80% of daily range is defined by CBDR projections. " +
			"Only trades during London/NY sessions (02:00-16:00 EST).",
		WhenToUse:     "Forex majors, XAUUSD. Requires intraday data covering Asian + London/NY sessions.",
		RecommendedTF: "5m, 15m, 30m",
		ParamDetails: []ParamInfo{
			{Key: "swing_period", Label: "Swing Period", Description: "Swing detection period", Default: 5},
			{Key: "atr_period", Label: "ATR Period", Description: "ATR for displacement", Default: 14},
			{Key: "disp_mult", Label: "Displacement Mult", Description: "Min displacement strength", Default: 0.5},
			{Key: "body_ratio", Label: "Body Ratio", Description: "Min body/range ratio", Default: 0.4},
			{Key: "min_std", Label: "Min STD", Description: "Minimum STD level to trade (e.g., 1.0)", Default: 1.0},
			{Key: "max_std", Label: "Max STD", Description: "Maximum STD level (avoid extremes)", Default: 2.5},
		},
		Presets: []Preset{
			{Name: "wide", Label: "Wide range (0.5-3.0 STD)", Params: map[string]float64{"min_std": 0.5, "max_std": 3.0, "disp_mult": 0.3}},
			{Name: "default", Label: "Default (1.0-2.5 STD)", Params: map[string]float64{"min_std": 1.0, "max_std": 2.5, "disp_mult": 0.5}},
			{Name: "precision", Label: "Precision (1.5-2.0 STD)", Params: map[string]float64{"min_std": 1.5, "max_std": 2.0, "disp_mult": 0.8}},
		},
		ExampleCommand: "/backtest XAUUSD 5m cbdr_std sl=0.003 tp=0.006",
	},

	"ict_advanced": {
		Key:       "ict_advanced",
		Category:  "ICT",
		Emoji:     "🧠",
		ShortDesc: "Full ICT model + 10 advanced filters (MTF, Judas, IPDA, etc.)",
		HowItWorks: "The most comprehensive ICT strategy. Starts with the ICT 2022 base model, " +
			"then layers up to 10 optional filters:\n\n" +
			"- htf_filter: Daily timeframe bias alignment\n" +
			"- killzone_only: Only trade during Kill Zone sessions\n" +
			"- cbdr_filter: Require CBDR STD level confluence\n" +
			"- smt_confluence: Smart Money divergence confirmation\n" +
			"- gap_awareness: NDOG/NWOG gap direction alignment\n" +
			"- pd_arrays: Premium/Discount zone confirmation\n" +
			"- judas_swing: Require nearby Judas Swing alignment\n" +
			"- liq_void: Require unfilled liquidity void as target\n" +
			"- ipda_filter: Skip entries during consolidation\n" +
			"- bias9_step: Block signals against 9-step daily bias\n\n" +
			"Each filter = 0 (off) or 1 (on). Start with defaults and enable filters one by one.",
		WhenToUse:     "Advanced ICT traders who want maximum confluence. All markets.",
		RecommendedTF: "5m, 15m, 1h",
		ParamDetails: []ParamInfo{
			{Key: "swing_period", Label: "Swing Period", Description: "Swing detection period", Default: 5},
			{Key: "atr_period", Label: "ATR Period", Description: "ATR period", Default: 14},
			{Key: "disp_mult", Label: "Displacement Mult", Description: "Min displacement strength", Default: 1.5},
			{Key: "body_ratio", Label: "Body Ratio", Description: "Min body/range ratio", Default: 0.6},
			{Key: "fvg_fib_valid", Label: "Fib Validation", Description: "1=require fib validation, 0=skip", Default: 1},
			{Key: "lookback", Label: "Lookback", Description: "Setup scan lookback", Default: 30},
			{Key: "htf_filter", Label: "HTF Filter", Description: "1=require daily bias alignment", Default: 1},
			{Key: "killzone_only", Label: "Kill Zone Only", Description: "1=only trade in Kill Zones", Default: 0},
			{Key: "cbdr_filter", Label: "CBDR Filter", Description: "1=require CBDR STD confluence", Default: 0},
			{Key: "smt_confluence", Label: "SMT Confluence", Description: "1=require SMT divergence", Default: 0},
			{Key: "gap_awareness", Label: "Gap Awareness", Description: "1=check NDOG/NWOG alignment", Default: 0},
			{Key: "pd_arrays", Label: "PD Arrays", Description: "1=require PD zone confirmation", Default: 0},
			{Key: "judas_swing", Label: "Judas Swing", Description: "1=require Judas Swing alignment", Default: 0},
			{Key: "liq_void", Label: "Liquidity Void", Description: "1=require void target", Default: 0},
			{Key: "ipda_filter", Label: "IPDA Filter", Description: "1=skip consolidation entries", Default: 0},
			{Key: "bias9_step", Label: "9-Step Bias", Description: "1=block signals against daily bias", Default: 0},
		},
		Presets: []Preset{
			{Name: "base", Label: "Base (HTF only)", Params: map[string]float64{"htf_filter": 1, "killzone_only": 0, "cbdr_filter": 0, "smt_confluence": 0, "gap_awareness": 0, "pd_arrays": 0, "judas_swing": 0, "liq_void": 0, "ipda_filter": 0, "bias9_step": 0}},
			{Name: "killzone", Label: "Kill Zone + HTF", Params: map[string]float64{"htf_filter": 1, "killzone_only": 1, "cbdr_filter": 0, "smt_confluence": 0, "gap_awareness": 0, "pd_arrays": 0, "judas_swing": 0, "liq_void": 0, "ipda_filter": 0, "bias9_step": 0}},
			{Name: "full", Label: "Full confluence (all filters)", Params: map[string]float64{"htf_filter": 1, "killzone_only": 1, "cbdr_filter": 1, "smt_confluence": 0, "gap_awareness": 1, "pd_arrays": 1, "judas_swing": 1, "liq_void": 0, "ipda_filter": 1, "bias9_step": 1}},
		},
		ExampleCommand: "/backtest XAUUSD 15m ict_advanced htf_filter=1 killzone_only=1 sl=0.005 tp=0.01",
	},
}

// CatalogOrder defines the display order of strategies in the /strategies menu.
var CatalogOrder = []string{
	// Classic
	"ema_cross", "rsi", "macd", "bb_breakout", "supertrend", "donchian", "sma_rsi",
	// ICT
	"ict2022", "silver_bullet", "turtle_soup", "amd_session", "ob_retest",
	"weekly_profile", "breaker_entry", "cbdr_std", "ict_advanced",
}
