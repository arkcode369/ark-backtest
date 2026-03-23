package bot

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"trading-backtest-bot/internal/backtest"
	"trading-backtest-bot/internal/data"
	"trading-backtest-bot/internal/strategy"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot wraps the Telegram bot and all services
type Bot struct {
	api           *tgbotapi.BotAPI
	stratCreator  *strategy.Creator
	mu            sync.Mutex
	stratSessions map[int64]bool // chats in strategy creation mode (mutex-protected)
	sem           chan struct{}   // semaphore to limit concurrent goroutines
	wg            sync.WaitGroup // tracks in-flight handler goroutines
	ctx           context.Context
	cancel        context.CancelFunc
	userLimiter   *UserRateLimiter // per-user rate limiting
}

func New(token string) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to init bot: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Bot{
		api:           api,
		stratCreator:  strategy.NewCreator(),
		stratSessions: make(map[int64]bool),
		sem:           make(chan struct{}, 20),
		ctx:           ctx,
		cancel:        cancel,
		userLimiter:   NewUserRateLimiter(),
	}, nil
}

func (b *Bot) isStratSession(chatID int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.stratSessions[chatID]
}

func (b *Bot) setStratSession(chatID int64, active bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if active {
		b.stratSessions[chatID] = true
	} else {
		delete(b.stratSessions, chatID)
	}
}

func (b *Bot) Run() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.api.GetUpdatesChan(u)

	slog.Info("bot started", "username", b.api.Self.UserName)

	for {
		select {
		case <-b.ctx.Done():
			slog.Info("bot context cancelled, stopping update loop")
			return
		case update, ok := <-updates:
			if !ok {
				slog.Info("update channel closed")
				return
			}
			if update.Message == nil {
				continue
			}
			b.wg.Add(1)
			go func() {
				defer b.wg.Done()
				b.sem <- struct{}{}
				defer func() { <-b.sem }()
				b.handleMessage(update.Message)
			}()
		}
	}
}

// Stop gracefully stops the bot: stops receiving updates and waits for in-flight handlers
func (b *Bot) Stop() {
	slog.Info("stopping bot...")
	b.cancel()
	b.api.StopReceivingUpdates()

	// Wait for in-flight handlers with timeout
	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		slog.Info("all handlers finished")
	case <-time.After(65 * time.Second):
		slog.Warn("shutdown timeout: some handlers may still be running")
	}

	// Stop the strategy creator's cleanup goroutine
	b.stratCreator.Stop()
}

func (b *Bot) handleMessage(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	text := strings.TrimSpace(msg.Text)

	// ── Strategy creation mode ────────────────────────────────
	if b.isStratSession(chatID) && !strings.HasPrefix(text, "/") {
		b.handleStrategyChat(chatID, text)
		return
	}

	// ── Commands ──────────────────────────────────────────────
	cmd := msg.Command()
	args := strings.TrimSpace(msg.CommandArguments())

	switch cmd {
	case "start":
		b.sendStart(chatID)
	case "help":
		b.sendHelp(chatID)
	case "symbols":
		b.handleSymbols(chatID, args)
	case "price":
		if !b.userLimiter.Allow(chatID, "price") {
			b.send(chatID, "\u23f3 Please wait a moment before requesting another price.")
			return
		}
		b.handlePrice(chatID, args)
	case "backtest":
		if !b.userLimiter.Allow(chatID, "backtest") {
			b.send(chatID, "\u23f3 Please wait — your previous backtest may still be running.")
			return
		}
		b.handleBacktest(chatID, args)
	case "compare":
		if !b.userLimiter.Allow(chatID, "backtest") {
			b.send(chatID, "\u23f3 Please wait — your previous comparison may still be running.")
			return
		}
		b.handleCompare(chatID, args)
	case "optimize":
		if !b.userLimiter.Allow(chatID, "backtest") {
			b.send(chatID, "\u23f3 Please wait — your previous optimization may still be running.")
			return
		}
		b.handleOptimize(chatID, args)
	case "strategy":
		if !b.userLimiter.Allow(chatID, "ai") {
			b.send(chatID, "\u23f3 Please wait before starting a new strategy session.")
			return
		}
		b.handleStrategyMode(chatID, args)
	case "endstrategy":
		b.handleEndStrategy(chatID)
	case "genmd":
		if !b.userLimiter.Allow(chatID, "ai") {
			b.send(chatID, "\u23f3 Please wait — document generation is in progress.")
			return
		}
		b.handleGenMD(chatID, args)
	case "analyze":
		if !b.userLimiter.Allow(chatID, "ai") {
			b.send(chatID, "\u23f3 Please wait before sending another analysis request.")
			return
		}
		b.handleAnalyze(chatID, args)
	case "intervals":
		b.handleIntervals(chatID)
	case "strategies":
		b.handleListStrategies(chatID)
	case "si":
		b.handleStrategyInfo(chatID, args)
	case "list":
		b.handleListFiles(chatID)
	default:
		if cmd != "" {
			b.send(chatID, "\u2753 Unknown command: /"+cmd+"\nUse /help for all available commands.")
		}
	}
}

// ── /start ────────────────────────────────────────────────────────────────

func (b *Bot) sendStart(chatID int64) {
	msg := "🚀 *Trading Backtest Bot*\n\n" +
		"Backtest trading strategies on futures, CFDs, forex, metals, indices, and energy using free Yahoo Finance data — powered by AI analytics.\n\n" +
		"*Quick Start:*\n" +
		"• /symbols — browse all supported instruments\n" +
		"• /price XAUUSD — get latest price\n" +
		"• /strategies — browse all strategies\n" +
		"• /si ict2022 — detailed strategy info & presets\n" +
		"• /backtest XAUUSD 1d ema\\_cross — run a backtest\n" +
		"• /strategy — start AI strategy builder\n" +
		"• /help — full command reference"
	b.sendMD(chatID, msg)
}

// ── /help ─────────────────────────────────────────────────────────────────

func (b *Bot) sendHelp(chatID int64) {
	msg := "📖 *Command Reference*\n\n" +
		"*📊 Data & Prices*\n" +
		"`/price SYMBOL` — latest price\n" +
		"`/symbols [category]` — list instruments\n" +
		"`/intervals` — data interval limits\n\n" +
		"*🔬 Backtesting*\n" +
		"`/backtest SYMBOL INTERVAL STRATEGY [params]`\n" +
		"  _Example:_ `/backtest XAUUSD 1d ema_cross fast=9 slow=21`\n" +
		"  _Example:_ `/backtest NQ 1h macd fast=12 slow=26 signal=9`\n" +
		"  _Example:_ `/backtest EURUSD 1h rsi period=14`\n" +
		"`/compare SYM1,SYM2,SYM3 INTERVAL STRATEGY [params]`\n" +
		"  _Compare strategy across multiple symbols_\n" +
		"`/optimize SYMBOL INTERVAL STRATEGY param=min:max:step`\n" +
		"  _Example:_ `/optimize XAUUSD 1d ema_cross fast=5:20:1 slow=15:50:5`\n\n" +
		"*🧠 Strategy Builder (AI)*\n" +
		"`/strategy [topic]` — start AI strategy conversation\n" +
		"`/endstrategy` — end session\n" +
		"`/genmd [name]` — generate strategy document (.md file)\n" +
		"`/analyze [question]` — quick AI market analysis\n\n" +
		"*📋 Info*\n" +
		"`/strategies` — list all strategies with descriptions\n" +
		"`/si <strategy>` — detailed strategy info, params & presets\n" +
		"`/list` — list saved strategy documents\n\n" +
		"*⚙️ Backtest Options (add to command)*\n" +
		"  `capital=10000` — starting capital (default: 10000)\n" +
		"  `pos=0.02` — position size % (default: 2%)\n" +
		"  `sl=0.02` — stop loss % (default: disabled)\n" +
		"  `tp=0.04` — take profit % (default: disabled)\n" +
		"  `commission=5` — round-trip commission USD (default: 0)\n" +
		"  `period=1y` — data period (default: varies by interval)\n" +
		"  `oos=0.3` — walk-forward out-of-sample fraction (default: disabled)\n\n" +
		"*Data Limits:*\n" +
		"  1m: 7d | 5m-30m: 60d | 1h: 2yr | 1d: 10yr+"

	b.sendMD(chatID, msg)
}

// ── /symbols ──────────────────────────────────────────────────────────────

func (b *Bot) handleSymbols(chatID int64, args string) {
	cat := ""
	if args != "" {
		cat = strings.ToUpper(args[:1]) + strings.ToLower(args[1:])
	}

	cats := map[string][]string{
		"Metals":  {"XAUUSD", "XAGUSD", "COPPER", "PALLADIUM", "PLATINUM"},
		"Indices": {"NQ", "ES", "YM", "RTY"},
		"Forex":   {"EURUSD", "GBPUSD", "USDJPY", "USDCHF", "AUDUSD", "NZDUSD", "USDCAD", "DXY"},
		"Energy":  {"CL", "RB", "HO", "NG"},
	}

	if cat != "" {
		syms, ok := cats[cat]
		if !ok {
			b.send(chatID, fmt.Sprintf("❌ Category '%s' not found. Try: Metals, Indices, Forex, Energy", args))
			return
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("📊 *%s Instruments*\n\n", cat))
		for _, key := range syms {
			s := data.SymbolMap[key]
			sb.WriteString(fmt.Sprintf("• `%s` — %s\n  _%s_\n", key, s.Name, s.Description))
		}
		b.sendMD(chatID, sb.String())
		return
	}

	// Show all categories summary
	var sb strings.Builder
	sb.WriteString("📊 *Supported Instruments*\n\n")
	for catName, syms := range cats {
		sb.WriteString(fmt.Sprintf("*%s:*\n", catName))
		for _, key := range syms {
			s := data.SymbolMap[key]
			sb.WriteString(fmt.Sprintf("  `%s` — %s\n", key, s.Name))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("Use `/symbols Metals` to filter by category\nUse `/price SYMBOL` for current price")
	b.sendMD(chatID, sb.String())
}

// ── /price ────────────────────────────────────────────────────────────────

func (b *Bot) handlePrice(chatID int64, args string) {
	if args == "" {
		b.send(chatID, "Usage: /price SYMBOL\nExample: /price XAUUSD")
		return
	}

	sym := strings.ToUpper(strings.TrimSpace(args))
	symInfo, ok := data.GetSymbol(sym)
	if !ok {
		b.send(chatID, fmt.Sprintf("❌ Unknown symbol: %s\nUse /symbols to see all supported instruments.", sym))
		return
	}

	b.send(chatID, "⏳ Fetching price...")

	price, currency, err := data.FetchLatestPrice(b.ctx, sym)
	if err != nil {
		b.send(chatID, fmt.Sprintf("❌ Error fetching price for %s: %v", sym, err))
		return
	}

	decimals := 2
	if symInfo.TickSize < 0.01 {
		decimals = 5
	}

	msg := fmt.Sprintf("💹 *%s — %s*\n\nPrice: `%s %s`\nTicker: `%s`\nCategory: %s",
		sym, symInfo.Name,
		currency, data.FormatNumber(price, decimals),
		symInfo.Ticker, symInfo.Category,
	)
	b.sendMD(chatID, msg)
}

// ── /intervals ────────────────────────────────────────────────────────────

func (b *Bot) handleIntervals(chatID int64) {
	msg := "📅 *Data Interval Limits (Yahoo Finance)*\n\n" +
		"Interval | Max History\n" +
		"---------|------------\n" +
		"`1m`  | 7 days\n" +
		"`2m`  | 60 days\n" +
		"`5m`  | 60 days\n" +
		"`15m` | 60 days\n" +
		"`30m` | 60 days\n" +
		"`1h`  | ~2 years\n" +
		"`1d`  | 10+ years\n" +
		"`1w`  | 10+ years\n\n" +
		"⚠️ *Note:* 1m is limited to 7 days per Yahoo Finance API.\n" +
		"Use `1d` or `1h` for meaningful backtests."
	b.sendMD(chatID, msg)
}

// ── /strategies ───────────────────────────────────────────────────────────

func (b *Bot) handleListStrategies(chatID int64) {
	var sb strings.Builder
	sb.WriteString("📈 *Strategy Menu*\n\n")

	// Group by category in display order
	currentCat := ""
	for _, key := range backtest.CatalogOrder {
		cat, ok := backtest.StrategyCatalog[key]
		if !ok {
			continue
		}
		if cat.Category != currentCat {
			currentCat = cat.Category
			if currentCat == "Classic" {
				sb.WriteString("━━━ *Classic Strategies* ━━━\n\n")
			} else {
				sb.WriteString("\n━━━ *ICT Strategies* ━━━\n\n")
			}
		}
		sb.WriteString(fmt.Sprintf("%s `%s` — *%s*\n", cat.Emoji, key, backtest.StrategyRegistry[key].Name))
		sb.WriteString(fmt.Sprintf("    %s\n", cat.ShortDesc))
		sb.WriteString(fmt.Sprintf("    TF: %s\n\n", cat.RecommendedTF))
	}

	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
	sb.WriteString("*Detail strategy:* `/si <nama>`\n")
	sb.WriteString("  Contoh: `/si ict2022`\n\n")
	sb.WriteString("*Quick backtest:* `/backtest SYMBOL TF STRATEGY`\n")
	sb.WriteString("  Contoh: `/backtest XAUUSD 1d ema_cross`\n\n")
	sb.WriteString("*Preset:* `/si <nama>` lalu copy preset yg mau dipakai")

	b.sendMD(chatID, sb.String())
}

func (b *Bot) handleStrategyInfo(chatID int64, args string) {
	if args == "" {
		b.send(chatID, "Usage: /si <strategy_key>\nExample: /si ict2022\n\nUse /strategies to see all available strategies.")
		return
	}

	key := strings.ToLower(strings.TrimSpace(args))
	cat, ok := backtest.StrategyCatalog[key]
	if !ok {
		// Try partial match
		var matches []string
		for k := range backtest.StrategyCatalog {
			if strings.Contains(k, key) {
				matches = append(matches, k)
			}
		}
		if len(matches) == 1 {
			key = matches[0]
			cat = backtest.StrategyCatalog[key]
		} else if len(matches) > 1 {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Multiple matches for '%s':\n", key))
			for _, m := range matches {
				sb.WriteString(fmt.Sprintf("  /si %s\n", m))
			}
			b.send(chatID, sb.String())
			return
		} else {
			b.send(chatID, fmt.Sprintf("Strategy '%s' not found.\nUse /strategies to see all available strategies.", key))
			return
		}
	}

	meta := backtest.StrategyRegistry[key]

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s *%s* (`%s`)\n", cat.Emoji, meta.Name, key))
	sb.WriteString(fmt.Sprintf("Category: %s | TF: %s\n\n", cat.Category, cat.RecommendedTF))

	// How It Works
	sb.WriteString("*Cara Kerja:*\n")
	sb.WriteString(cat.HowItWorks)
	sb.WriteString("\n\n")

	// When To Use
	sb.WriteString("*Kapan Digunakan:*\n")
	sb.WriteString(cat.WhenToUse)
	sb.WriteString("\n\n")

	// Parameters
	sb.WriteString("*Parameter:*\n")
	for _, p := range cat.ParamDetails {
		sb.WriteString(fmt.Sprintf("  `%s` = %s", p.Key, p.Label))
		if p.Default == float64(int(p.Default)) {
			sb.WriteString(fmt.Sprintf(" (default: %.0f)", p.Default))
		} else {
			sb.WriteString(fmt.Sprintf(" (default: %g)", p.Default))
		}
		sb.WriteString(fmt.Sprintf("\n    _%s_\n", p.Description))
	}
	sb.WriteString("\n")

	// Presets
	if len(cat.Presets) > 0 {
		sb.WriteString("*Preset (copy-paste ready):*\n\n")
		for _, pr := range cat.Presets {
			sb.WriteString(fmt.Sprintf("_%s_\n", pr.Label))
			sb.WriteString(fmt.Sprintf("`/backtest XAUUSD 1h %s", key))
			for _, pd := range cat.ParamDetails {
				if v, ok := pr.Params[pd.Key]; ok {
					if v == float64(int(v)) {
						sb.WriteString(fmt.Sprintf(" %s=%.0f", pd.Key, v))
					} else {
						sb.WriteString(fmt.Sprintf(" %s=%g", pd.Key, v))
					}
				}
			}
			sb.WriteString(" sl=0.005 tp=0.01`\n\n")
		}
	}

	// Example
	sb.WriteString("*Contoh:*\n")
	sb.WriteString(fmt.Sprintf("`%s`", cat.ExampleCommand))

	b.sendMD(chatID, sb.String())
}

// ── /backtest ─────────────────────────────────────────────────────────────

func (b *Bot) handleBacktest(chatID int64, args string) {
	if args == "" {
		b.send(chatID, "Usage: /backtest SYMBOL INTERVAL STRATEGY [options]\nExample: /backtest XAUUSD 1d ema_cross fast=9 slow=21 capital=10000")
		return
	}

	parts := strings.Fields(args)
	if len(parts) < 3 {
		b.send(chatID, "❌ Need at least: SYMBOL INTERVAL STRATEGY\nExample: /backtest XAUUSD 1d ema_cross")
		return
	}

	symbolKey := strings.ToUpper(parts[0])
	interval := strings.ToLower(parts[1])
	stratKey := strings.ToLower(parts[2])

	// Parse key=value options
	opts := parseOptions(parts[3:])

	// Apply preset if specified (e.g., preset=strict)
	if presetName, ok := opts["preset"]; ok {
		if cat, catOK := backtest.StrategyCatalog[stratKey]; catOK {
			found := false
			for _, pr := range cat.Presets {
				if strings.EqualFold(pr.Name, presetName) {
					for k, v := range pr.Params {
						// Only apply preset value if user didn't explicitly set it
						if _, userSet := opts[k]; !userSet {
							opts[k] = fmt.Sprintf("%g", v)
						}
					}
					found = true
					break
				}
			}
			if !found {
				var names []string
				for _, pr := range cat.Presets {
					names = append(names, pr.Name)
				}
				b.send(chatID, fmt.Sprintf("❌ Preset '%s' not found for %s.\nAvailable: %s\nUse /si %s to see preset details.", presetName, stratKey, strings.Join(names, ", "), stratKey))
				return
			}
		}
		delete(opts, "preset")
	}

	stratParams := extractStratParams(opts, stratKey)

	// Engine config
	capital := getOptFloat(opts, "capital", 10000)
	posSizePct := getOptFloat(opts, "pos", 0.02)
	slPct := getOptFloat(opts, "sl", 0)
	tpPct := getOptFloat(opts, "tp", 0)
	commission := getOptFloat(opts, "commission", 0)
	period := getOptStr(opts, "period", defaultPeriod(interval))
	oosFraction := getOptFloat(opts, "oos", 0) // walk-forward out-of-sample fraction

	// Validate engine config params
	if capital <= 0 {
		b.send(chatID, "❌ Invalid parameter: capital must be > 0")
		return
	}
	if posSizePct <= 0 || posSizePct > 1 {
		b.send(chatID, "❌ Invalid parameter: pos (position size) must be between 0 and 1 (e.g., 0.02 for 2%)")
		return
	}
	if slPct < 0 || slPct > 1 {
		b.send(chatID, "❌ Invalid parameter: sl (stop loss) must be between 0 and 1 (e.g., 0.02 for 2%)")
		return
	}
	if tpPct < 0 || tpPct > 1 {
		b.send(chatID, "❌ Invalid parameter: tp (take profit) must be between 0 and 1 (e.g., 0.04 for 4%)")
		return
	}
	if commission < 0 {
		b.send(chatID, "❌ Invalid parameter: commission must be >= 0")
		return
	}

	// Validate strategy-specific params
	if err := validateParams(stratParams); err != nil {
		b.send(chatID, fmt.Sprintf("❌ Invalid strategy parameter: %v", err))
		return
	}

	// Validate symbol
	symInfo, ok := data.GetSymbol(symbolKey)
	if !ok {
		b.send(chatID, fmt.Sprintf("❌ Unknown symbol: %s\nUse /symbols to see all supported instruments.", symbolKey))
		return
	}

	// Validate interval
	if _, ok := data.ValidIntervals[interval]; !ok {
		b.send(chatID, fmt.Sprintf("❌ Unsupported interval: %s\nSupported: 1m, 5m, 15m, 30m, 1h, 1d, 1w\n⚠️ Note: Yahoo Finance does not support 4h — use 1h instead.", interval))
		return
	}
	// Warn if 4h is mapped to 1h
	if interval == "4h" {
		b.send(chatID, "⚠️ Yahoo Finance doesn't support 4h. Using 1h data instead.")
		interval = "1h"
	}

	// Validate strategy
	meta, ok := backtest.StrategyRegistry[stratKey]
	if !ok {
		b.send(chatID, fmt.Sprintf("❌ Unknown strategy: %s\nUse /strategies to see all available strategies.", stratKey))
		return
	}

	b.send(chatID, fmt.Sprintf("⏳ Fetching %s data (%s, %s)...", symbolKey, interval, period))

	// Fetch data
	bars, err := data.FetchOHLCV(b.ctx, data.FetchParams{
		Symbol:   symbolKey,
		Interval: interval,
		Period:   period,
	})
	if err != nil {
		b.send(chatID, fmt.Sprintf("❌ Data fetch error: %v", err))
		return
	}
	if len(bars) < 30 {
		b.send(chatID, fmt.Sprintf("❌ Not enough data: %d bars. Try a longer period or wider interval.", len(bars)))
		return
	}

	b.send(chatID, fmt.Sprintf("📊 Running backtest: %d bars, %s strategy...", len(bars), meta.Name))

	// Merge default + user params
	params := copyParams(meta.Params)
	for k, v := range stratParams {
		params[k] = v
	}

	// Run backtest
	cfg := backtest.Config{
		InitialCapital:  capital,
		PositionSizePct: posSizePct,
		Commission:      commission,
		Slippage:        symInfo.TickSize,
		StopLossPct:     slPct,
		TakeProfitPct:   tpPct,
		Symbol:          symbolKey,
		Interval:        interval,
	}

	engine := backtest.NewEngine(cfg)
	engine.LoadData(bars)
	strat := meta.Factory()
	engine.SetStrategy(strat)

	// Auto-fetch MTF data if strategy needs higher-timeframe context
	if mtf, ok := strat.(backtest.MultiTimeframeStrategy); ok {
		tfs := mtf.Timeframes()
		if len(tfs) > 0 {
			b.send(chatID, fmt.Sprintf("📈 Fetching higher-timeframe data (%s)...", strings.Join(tfs, ", ")))
			mtfData, err := data.FetchMultiTF(b.ctx, symbolKey, tfs, period)
			if err != nil {
				b.send(chatID, fmt.Sprintf("⚠️ MTF data fetch warning: %v (continuing without HTF bias)", err))
			} else {
				engine.LoadMTFData(mtfData)
			}
		}
	}

	// Auto-fetch correlated symbol data if strategy needs it (SMT divergence)
	if ms, ok := strat.(backtest.MultiSymbolStrategy); ok {
		syms := ms.Symbols()
		if len(syms) > 0 {
			b.send(chatID, fmt.Sprintf("🔗 Fetching correlated symbol data (%s)...", strings.Join(syms, ", ")))
			symData, err := data.FetchMultiSymbol(b.ctx, syms, interval, period)
			if err != nil {
				b.send(chatID, fmt.Sprintf("⚠️ Multi-symbol data fetch warning: %v (continuing without SMT)", err))
			} else {
				engine.LoadSymbolData(symData)
			}
		}
	}

	result, err := engine.Run(params)
	if err != nil {
		b.send(chatID, fmt.Sprintf("❌ Backtest error: %v", err))
		return
	}

	// Send result
	b.sendMD(chatID, backtest.FormatResult(result, capital))

	// Send equity curve chart
	if len(result.EquityCurve) > 2 {
		chart := backtest.FormatEquityCurve(result.EquityCurve, 50, 12)
		if chart != "" {
			b.send(chatID, chart)
		}
	}

	// Send top trades if any
	if len(result.Trades) > 0 {
		n := 3
		if len(result.Trades) < 3 {
			n = len(result.Trades)
		}
		best, worst := backtest.TopTrades(result, n)
		var sb strings.Builder
		sb.WriteString("🏆 *Top Winning Trades:*\n")
		for _, t := range best {
			if t.PnL > 0 {
				sb.WriteString(fmt.Sprintf("  %s → %s | %s | +$%.2f (+%.2f%%)\n",
					t.EntryTime.Format("01/02 15:04"), t.ExitTime.Format("01/02 15:04"),
					t.Direction, t.PnL, t.PnLPct))
			}
		}
		sb.WriteString("\n💀 *Worst Losing Trades:*\n")
		for _, t := range worst {
			if t.PnL < 0 {
				sb.WriteString(fmt.Sprintf("  %s → %s | %s | -$%.2f (%.2f%%)\n",
					t.EntryTime.Format("01/02 15:04"), t.ExitTime.Format("01/02 15:04"),
					t.Direction, math.Abs(t.PnL), t.PnLPct))
			}
		}
		b.sendMD(chatID, sb.String())

		// Tip for next steps
		b.send(chatID, "💡 Tip: Use /analyze to ask AI about these results, or /strategy to build a custom strategy.")
	}

	// Walk-forward validation if requested
	if oosFraction > 0 {
		if oosFraction > 0.5 {
			oosFraction = 0.3
		}
		wfEngine := backtest.NewEngine(cfg)
		wfEngine.LoadData(bars)
		wfEngine.SetStrategy(meta.Factory())
		wfResult, err := wfEngine.RunWalkForward(params, oosFraction)
		if err != nil {
			b.send(chatID, fmt.Sprintf("Walk-forward error: %v", err))
		} else {
			b.sendMD(chatID, backtest.FormatWalkForwardResult(wfResult))
		}
	}
}

// ── /strategy ─────────────────────────────────────────────────────────────

func (b *Bot) handleStrategyMode(chatID int64, args string) {
	b.setStratSession(chatID, true)
	b.stratCreator.ResetSession(chatID)

	greeting := "🧠 *AI Strategy Builder Mode Active*\n\n" +
		"I'm your quantitative trading strategist. Tell me about:\n" +
		"• What market you want to trade (e.g., \"Gold futures on 1h\")\n" +
		"• Your trading style (trend-following, mean-reversion, breakout)\n" +
		"• Your risk tolerance and trading session\n\n" +
		"When you're done designing, use:\n" +
		"• `/genmd Strategy Name` — generate a full strategy document\n" +
		"• `/endstrategy` — exit strategy mode\n\n" +
		"_What would you like to trade?_"

	b.sendMD(chatID, greeting)

	if args != "" {
		b.handleStrategyChat(chatID, args)
	}
}

func (b *Bot) handleStrategyChat(chatID int64, text string) {
	b.send(chatID, "🤔 Thinking...")

	response, err := b.stratCreator.Chat(b.ctx, chatID, text)
	if err != nil {
		b.send(chatID, fmt.Sprintf("❌ AI error: %v", err))
		return
	}

	b.sendAI(chatID, response)
}

func (b *Bot) handleEndStrategy(chatID int64) {
	b.setStratSession(chatID, false)
	b.send(chatID, "✅ Strategy session ended. Use /strategy to start a new one.")
}

// ── /genmd ────────────────────────────────────────────────────────────────

func (b *Bot) handleGenMD(chatID int64, args string) {
	stratName := args
	if stratName == "" {
		stratName = "Custom Trading Strategy"
	}

	b.send(chatID, "📝 Generating strategy document with extended thinking... (this may take 30-60s)")

	md, err := b.stratCreator.GenerateStrategyMD(b.ctx, chatID, stratName)
	if err != nil {
		b.send(chatID, fmt.Sprintf("❌ Error generating document: %v", err))
		return
	}

	// Save to file
	filename := fmt.Sprintf("strategy_%d_%s.md",
		time.Now().Unix(),
		sanitizeFilename(stratName),
	)

	storageDir := os.Getenv("STORAGE_DIR")
	if storageDir == "" {
		storageDir = "/storage"
	}
	os.MkdirAll(storageDir, 0755)
	filepath_ := filepath.Join(storageDir, filename)

	if err := os.WriteFile(filepath_, []byte(md), 0644); err != nil {
		b.send(chatID, fmt.Sprintf("❌ Could not save file: %v", err))
		return
	}

	// Send as document file
	doc := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(filepath_))
	doc.Caption = fmt.Sprintf("📄 %s\nGenerated by ARK Backtest Bot", stratName)
	if _, err := b.api.Send(doc); err != nil {
		// If file send fails (e.g. /storage not mounted in Docker), send as text
		b.send(chatID, "⚠️ Could not send file, showing as text instead:")
	}

	// Send readable preview (converted from markdown)
	preview := md
	if len(preview) > 3500 {
		preview = preview[:3500] + "\n\n...(full document sent as file above)"
	}
	b.sendAI(chatID, preview)
}

// ── /analyze ──────────────────────────────────────────────────────────────

func (b *Bot) handleAnalyze(chatID int64, args string) {
	if args == "" {
		b.send(chatID, "Usage: /analyze [question]\nExample: /analyze What are the best settings for EMA cross on Gold 1h?")
		return
	}

	b.send(chatID, "🔍 Analyzing...")

	response, err := b.stratCreator.QuickAnalysis(b.ctx, args)
	if err != nil {
		b.send(chatID, fmt.Sprintf("❌ AI error: %v", err))
		return
	}

	b.sendAI(chatID, response)
}

// ── Helpers ───────────────────────────────────────────────────────────────

func (b *Bot) send(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.api.Send(msg); err != nil {
		slog.Error("failed to send message", "chatID", chatID, "error", err)
	}
}

func (b *Bot) sendMD(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	_, err := b.api.Send(msg)
	if err != nil {
		// Fallback to plain text if markdown fails
		b.send(chatID, text)
	}
}

// sendAI converts AI markdown response to Telegram-friendly format and sends it
func (b *Bot) sendAI(chatID int64, text string) {
	converted := convertAIResponse(text)
	b.sendLong(chatID, converted)
}

func (b *Bot) sendLong(chatID int64, text string) {
	const maxLen = 4000
	for len(text) > 0 {
		chunk := text
		if len(chunk) > maxLen {
			cut := strings.LastIndex(text[:maxLen], "\n")
			if cut <= 0 {
				cut = maxLen
			}
			chunk = text[:cut]
			text = text[cut:]
		} else {
			text = ""
		}
		b.send(chatID, chunk)
	}
}

// convertAIResponse converts standard Markdown from AI to plain readable text
// that looks clean in Telegram without parse mode
func convertAIResponse(text string) string {
	lines := strings.Split(text, "\n")
	var out []string

	inCodeBlock := false
	for _, line := range lines {
		// Code blocks — keep as-is with indent
		if strings.HasPrefix(line, "```") {
			inCodeBlock = !inCodeBlock
			if inCodeBlock {
				out = append(out, "▸ Code:")
			}
			continue
		}
		if inCodeBlock {
			out = append(out, "  "+line)
			continue
		}

		// Headers: ### → bold-like with emoji prefix
		if strings.HasPrefix(line, "#### ") {
			line = "◆ " + strings.TrimPrefix(line, "#### ")
		} else if strings.HasPrefix(line, "### ") {
			line = "\n▌ " + strings.ToUpper(strings.TrimPrefix(line, "### "))
		} else if strings.HasPrefix(line, "## ") {
			line = "\n━━━━━━━━━━━━━━━━\n" + strings.ToUpper(strings.TrimPrefix(line, "## ")) + "\n━━━━━━━━━━━━━━━━"
		} else if strings.HasPrefix(line, "# ") {
			line = "\n🔷 " + strings.ToUpper(strings.TrimPrefix(line, "# ")) + "\n"
		}

		// Bold **text** → just remove markers (keep text)
		line = stripMarkdownMarkers(line, "**")

		// Italic *text* → keep text
		line = stripMarkdownMarkers(line, "*")

		// Inline code `text` → keep as-is (readable)
		// Table rows | col | col | → indent them
		if strings.HasPrefix(strings.TrimSpace(line), "|") {
			// Skip separator rows like |---|---|
			if strings.Contains(line, "---") {
				continue
			}
			// Format table row
			cells := strings.Split(line, "|")
			var parts []string
			for _, c := range cells {
				c = strings.TrimSpace(c)
				if c != "" {
					parts = append(parts, c)
				}
			}
			if len(parts) > 0 {
				line = "  " + strings.Join(parts, " | ")
			}
		}

		// Horizontal rules
		if strings.TrimSpace(line) == "---" || strings.TrimSpace(line) == "---" {
			line = "────────────────"
		}

		out = append(out, line)
	}

	result := strings.Join(out, "\n")
	// Collapse 3+ blank lines to 2
	for strings.Contains(result, "\n\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n\n", "\n\n\n")
	}
	return strings.TrimSpace(result)
}

// stripMarkdownMarkers removes paired markdown markers (e.g., ** or *) in a single pass.
// It scans left-to-right, finds open+close pairs, and keeps only the inner text.
func stripMarkdownMarkers(line, marker string) string {
	mLen := len(marker)
	var sb strings.Builder
	sb.Grow(len(line))
	i := 0
	for i < len(line) {
		if i+mLen <= len(line) && line[i:i+mLen] == marker {
			// Found opening marker, look for closing
			end := strings.Index(line[i+mLen:], marker)
			if end >= 0 {
				// Write inner text (between markers)
				sb.WriteString(line[i+mLen : i+mLen+end])
				i = i + mLen + end + mLen
				continue
			}
		}
		sb.WriteByte(line[i])
		i++
	}
	return sb.String()
}

// validateParams checks that strategy-specific parameters are reasonable.
func validateParams(params map[string]float64) error {
	periodKeys := map[string]bool{
		"period": true, "fast": true, "slow": true, "signal": true,
		"sma_period": true, "rsi_period": true,
	}
	for k, v := range params {
		if v <= 0 {
			return fmt.Errorf("%s must be > 0 (got %.4g)", k, v)
		}
		if periodKeys[k] && v < 2 {
			return fmt.Errorf("%s must be >= 2 (got %.4g)", k, v)
		}
		if periodKeys[k] && v > 10000 {
			return fmt.Errorf("%s is unreasonably large (got %.4g), must be < 10000", k, v)
		}
	}
	return nil
}

func parseOptions(parts []string) map[string]string {
	opts := make(map[string]string)
	for _, p := range parts {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) == 2 {
			opts[strings.ToLower(kv[0])] = kv[1]
		}
	}
	return opts
}

func getOptFloat(opts map[string]string, key string, def float64) float64 {
	if v, ok := opts[key]; ok {
		f, err := strconv.ParseFloat(v, 64)
		if err == nil {
			return f
		}
	}
	return def
}

func getOptStr(opts map[string]string, key string, def string) string {
	if v, ok := opts[key]; ok {
		return v
	}
	return def
}

// extractStratParams extracts strategy-specific params from options map
func extractStratParams(opts map[string]string, stratKey string) map[string]float64 {
	params := make(map[string]float64)
	// Get known params for this strategy
	meta, ok := backtest.StrategyRegistry[stratKey]
	if !ok {
		return params
	}
	for k := range meta.Params {
		if v, ok := opts[k]; ok {
			f, err := strconv.ParseFloat(v, 64)
			if err == nil {
				params[k] = f
			}
		}
	}
	return params
}

func copyParams(src map[string]float64) map[string]float64 {
	dst := make(map[string]float64, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func defaultPeriod(interval string) string {
	switch interval {
	case "1m":
		return "7d"
	case "2m", "5m", "15m", "30m":
		return "60d"
	case "1h", "60m":
		return "1y"
	default:
		return "2y"
	}
}

func sanitizeFilename(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "_")
	var out strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' {
			out.WriteRune(c)
		}
	}
	return out.String()
}

// ── /compare ─────────────────────────────────────────────────────────────

func (b *Bot) handleCompare(chatID int64, args string) {
	if args == "" {
		b.send(chatID, "Usage: /compare SYM1,SYM2,SYM3 INTERVAL STRATEGY [params]\nExample: /compare XAUUSD,XAGUSD,CL 1d ema_cross fast=9 slow=21")
		return
	}

	parts := strings.Fields(args)
	if len(parts) < 3 {
		b.send(chatID, "Need at least: SYMBOLS INTERVAL STRATEGY\nExample: /compare XAUUSD,XAGUSD 1d ema_cross")
		return
	}

	symbols := strings.Split(strings.ToUpper(parts[0]), ",")
	if len(symbols) < 2 || len(symbols) > 10 {
		b.send(chatID, "Provide 2-10 symbols separated by commas.")
		return
	}

	interval := strings.ToLower(parts[1])
	stratKey := strings.ToLower(parts[2])

	opts := parseOptions(parts[3:])
	period := getOptStr(opts, "period", defaultPeriod(interval))
	capital := getOptFloat(opts, "capital", 10000)
	posSizePct := getOptFloat(opts, "pos", 0.02)

	meta, ok := backtest.StrategyRegistry[stratKey]
	if !ok {
		b.send(chatID, fmt.Sprintf("Unknown strategy: %s\nUse /strategies to see all.", stratKey))
		return
	}

	if _, ok := data.ValidIntervals[interval]; !ok {
		b.send(chatID, fmt.Sprintf("Unsupported interval: %s", interval))
		return
	}

	// Validate all symbols first
	for _, sym := range symbols {
		if _, ok := data.GetSymbol(sym); !ok {
			b.send(chatID, fmt.Sprintf("Unknown symbol: %s", sym))
			return
		}
	}

	b.send(chatID, fmt.Sprintf("Comparing %s with %s strategy across %d symbols...", stratKey, interval, len(symbols)))

	// Merge params
	stratParams := extractStratParams(opts, stratKey)
	params := copyParams(meta.Params)
	for k, v := range stratParams {
		params[k] = v
	}

	// Run backtests concurrently
	type compareResult struct {
		Symbol string
		Result *backtest.Result
		Err    error
	}
	results := make([]compareResult, len(symbols))
	var wg sync.WaitGroup

	for idx, sym := range symbols {
		wg.Add(1)
		go func(i int, s string) {
			defer wg.Done()
			symInfo, ok := data.GetSymbol(s)
			if !ok {
				results[i] = compareResult{Symbol: s, Err: fmt.Errorf("unknown symbol: %s", s)}
				return
			}
			bars, err := data.FetchOHLCV(b.ctx, data.FetchParams{
				Symbol: s, Interval: interval, Period: period,
			})
			if err != nil {
				results[i] = compareResult{Symbol: s, Err: err}
				return
			}
			if len(bars) < 30 {
				results[i] = compareResult{Symbol: s, Err: fmt.Errorf("only %d bars", len(bars))}
				return
			}
			cfg := backtest.Config{
				InitialCapital:  capital,
				PositionSizePct: posSizePct,
				Slippage:        symInfo.TickSize,
				Symbol:          s,
				Interval:        interval,
			}
			engine := backtest.NewEngine(cfg)
			engine.LoadData(bars)
			engine.SetStrategy(meta.Factory())
			r, err := engine.Run(params)
			results[i] = compareResult{Symbol: s, Result: r, Err: err}
		}(idx, sym)
	}
	wg.Wait()

	// Format comparison table
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\U0001f4ca *%s Comparison — %s %s*\n\n", meta.Name, interval, period))
	sb.WriteString(fmt.Sprintf("%-10s | %5s | %6s | %9s | %6s | %5s\n", "Symbol", "Trades", "WinRate", "P&L", "MaxDD%", "Sharpe"))
	sb.WriteString(strings.Repeat("\u2500", 58) + "\n")

	for _, r := range results {
		if r.Err != nil {
			sb.WriteString(fmt.Sprintf("%-10s | ERROR: %v\n", r.Symbol, r.Err))
			continue
		}
		res := r.Result
		sb.WriteString(fmt.Sprintf("%-10s | %5d | %5.1f%% | $%8.2f | %5.1f%% | %5.2f\n",
			r.Symbol, res.TotalTrades, res.WinRate, res.TotalPnL, res.MaxDrawdownPct, res.SharpeRatio))
	}

	b.send(chatID, sb.String())
}

// ── /optimize ────────────────────────────────────────────────────────────

func (b *Bot) handleOptimize(chatID int64, args string) {
	if args == "" {
		b.send(chatID, "Usage: /optimize SYMBOL INTERVAL STRATEGY param=min:max:step ...\n"+
			"Example: /optimize XAUUSD 1d ema_cross fast=5:20:1 slow=15:50:5\n\n"+
			"Parameter ranges: name=min:max:step")
		return
	}

	parts := strings.Fields(args)
	if len(parts) < 4 {
		b.send(chatID, "Need: SYMBOL INTERVAL STRATEGY param=min:max:step\nAt least one parameter range is required.")
		return
	}

	symbolKey := strings.ToUpper(parts[0])
	interval := strings.ToLower(parts[1])
	stratKey := strings.ToLower(parts[2])

	symInfo, ok := data.GetSymbol(symbolKey)
	if !ok {
		b.send(chatID, fmt.Sprintf("Unknown symbol: %s", symbolKey))
		return
	}
	if _, ok := data.ValidIntervals[interval]; !ok {
		b.send(chatID, fmt.Sprintf("Unsupported interval: %s", interval))
		return
	}
	meta, ok := backtest.StrategyRegistry[stratKey]
	if !ok {
		b.send(chatID, fmt.Sprintf("Unknown strategy: %s", stratKey))
		return
	}

	// Parse parameter ranges (param=min:max:step)
	ranges := make(map[string][3]float64) // name -> [min, max, step]
	opts := parseOptions(parts[3:])
	period := getOptStr(opts, "period", defaultPeriod(interval))
	capital := getOptFloat(opts, "capital", 10000)

	for _, p := range parts[3:] {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.ToLower(kv[0])
		// Skip non-range options
		if key == "period" || key == "capital" || key == "pos" || key == "sl" || key == "tp" || key == "commission" {
			continue
		}
		rangeStr := strings.Split(kv[1], ":")
		if len(rangeStr) != 3 {
			continue
		}
		min, err1 := strconv.ParseFloat(rangeStr[0], 64)
		max, err2 := strconv.ParseFloat(rangeStr[1], 64)
		step, err3 := strconv.ParseFloat(rangeStr[2], 64)
		if err1 != nil || err2 != nil || err3 != nil || step <= 0 || min > max {
			b.send(chatID, fmt.Sprintf("Invalid range for %s: %s (use min:max:step)", key, kv[1]))
			return
		}
		ranges[key] = [3]float64{min, max, step}
	}

	if len(ranges) == 0 {
		b.send(chatID, "No parameter ranges found. Use format: param=min:max:step")
		return
	}

	// Generate parameter combinations
	combos := generateCombinations(ranges, meta.Params)
	if len(combos) > 500 {
		b.send(chatID, fmt.Sprintf("Too many combinations (%d). Reduce ranges or increase step. Max: 500.", len(combos)))
		return
	}

	b.send(chatID, fmt.Sprintf("Optimizing %s on %s %s: %d combinations...", stratKey, symbolKey, interval, len(combos)))

	// Fetch data once
	bars, err := data.FetchOHLCV(b.ctx, data.FetchParams{
		Symbol: symbolKey, Interval: interval, Period: period,
	})
	if err != nil {
		b.send(chatID, fmt.Sprintf("Data fetch error: %v", err))
		return
	}
	if len(bars) < 30 {
		b.send(chatID, fmt.Sprintf("Not enough data: %d bars.", len(bars)))
		return
	}

	// Run all combinations
	type optResult struct {
		Params map[string]float64
		Result *backtest.Result
	}
	var results []optResult

	for _, params := range combos {
		cfg := backtest.Config{
			InitialCapital:  capital,
			PositionSizePct: 0.02,
			Slippage:        symInfo.TickSize,
			Symbol:          symbolKey,
			Interval:        interval,
		}
		engine := backtest.NewEngine(cfg)
		engine.LoadData(bars)
		engine.SetStrategy(meta.Factory())
		r, err := engine.Run(params)
		if err != nil {
			continue
		}
		if r.TotalTrades < 5 {
			continue // skip params that produce too few trades
		}
		results = append(results, optResult{Params: params, Result: r})
	}

	if len(results) == 0 {
		b.send(chatID, "No valid results. All parameter combinations produced < 5 trades.")
		return
	}

	// Sort by Sharpe ratio (descending)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Result.SharpeRatio > results[j].Result.SharpeRatio
	})

	// Show top 5
	n := 5
	if len(results) < n {
		n = len(results)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\U0001f3af *Optimization Results — %s %s %s*\n", symbolKey, interval, meta.Name))
	sb.WriteString(fmt.Sprintf("Tested: %d combinations | Bars: %d\n\n", len(combos), len(bars)))

	for i := 0; i < n; i++ {
		r := results[i]
		sb.WriteString(fmt.Sprintf("#%d ", i+1))
		var pStrs []string
		for k, v := range r.Params {
			pStrs = append(pStrs, fmt.Sprintf("%s=%.0f", k, v))
		}
		sb.WriteString(strings.Join(pStrs, " "))
		sb.WriteString(fmt.Sprintf("\n   Trades=%d WR=%.0f%% P&L=$%.0f Sharpe=%.2f PF=%.2f DD=%.1f%%\n\n",
			r.Result.TotalTrades, r.Result.WinRate, r.Result.TotalPnL,
			r.Result.SharpeRatio, r.Result.ProfitFactor, r.Result.MaxDrawdownPct))
	}

	sb.WriteString("Use the best params with /backtest to see full results.")
	b.send(chatID, sb.String())
}

// generateCombinations creates all parameter combinations from ranges
func generateCombinations(ranges map[string][3]float64, defaults map[string]float64) []map[string]float64 {
	// Build list of param names and their values
	type paramValues struct {
		name   string
		values []float64
	}
	var pv []paramValues
	for name, r := range ranges {
		var vals []float64
		for v := r[0]; v <= r[1]; v += r[2] {
			vals = append(vals, v)
		}
		pv = append(pv, paramValues{name: name, values: vals})
	}

	// Cartesian product
	var result []map[string]float64
	var generate func(idx int, current map[string]float64)
	generate = func(idx int, current map[string]float64) {
		if idx == len(pv) {
			combo := copyParams(defaults)
			for k, v := range current {
				combo[k] = v
			}
			result = append(result, combo)
			return
		}
		for _, v := range pv[idx].values {
			current[pv[idx].name] = v
			generate(idx+1, current)
		}
	}
	generate(0, make(map[string]float64))
	return result
}

// ── /list ─────────────────────────────────────────────────────────────────

func (b *Bot) handleListFiles(chatID int64) {
	storageDir := os.Getenv("STORAGE_DIR")
	if storageDir == "" {
		storageDir = "/storage"
	}

	entries, err := os.ReadDir(storageDir)
	if err != nil {
		b.send(chatID, "No strategy documents found. Use /genmd to generate one.")
		return
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			info, err := e.Info()
			if err != nil {
				continue
			}
			files = append(files, fmt.Sprintf("  %s (%s)",
				e.Name(), info.ModTime().Format("2006-01-02 15:04")))
		}
	}

	if len(files) == 0 {
		b.send(chatID, "No strategy documents found. Use /genmd to generate one.")
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Saved Strategy Documents (%d):\n\n", len(files)))
	for _, f := range files {
		sb.WriteString(f + "\n")
	}
	b.send(chatID, sb.String())
}
