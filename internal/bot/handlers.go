package bot

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
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
}

func New(token string) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to init bot: %w", err)
	}
	return &Bot{
		api:           api,
		stratCreator:  strategy.NewCreator(),
		stratSessions: make(map[int64]bool),
		sem:           make(chan struct{}, 20),
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

	fmt.Printf("🤖 Bot @%s is running...\n", b.api.Self.UserName)

	for update := range updates {
		if update.Message == nil {
			continue
		}
		go func() {
			b.sem <- struct{}{}
			defer func() { <-b.sem }()
			b.handleMessage(update.Message)
		}()
	}
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
		b.handlePrice(chatID, args)
	case "backtest":
		b.handleBacktest(chatID, args)
	case "strategy":
		b.handleStrategyMode(chatID, args)
	case "endstrategy":
		b.handleEndStrategy(chatID)
	case "genmd":
		b.handleGenMD(chatID, args)
	case "analyze":
		b.handleAnalyze(chatID, args)
	case "intervals":
		b.handleIntervals(chatID)
	case "strategies":
		b.handleListStrategies(chatID)
	default:
		if cmd != "" {
			b.send(chatID, "❓ Unknown command: /"+cmd+"\nUse /help for all available commands.")
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
		"  _Example:_ `/backtest EURUSD 1h rsi period=14`\n\n" +
		"*🧠 Strategy Builder (AI)*\n" +
		"`/strategy [topic]` — start AI strategy conversation\n" +
		"`/endstrategy` — end session\n" +
		"`/genmd [name]` — generate strategy document (.md file)\n" +
		"`/analyze [question]` — quick AI market analysis\n\n" +
		"*📋 Info*\n" +
		"`/strategies` — list built-in strategies\n\n" +
		"*⚙️ Backtest Options (add to command)*\n" +
		"  `capital=10000` — starting capital (default: 10000)\n" +
		"  `pos=0.02` — position size % (default: 2%)\n" +
		"  `sl=0.02` — stop loss % (default: disabled)\n" +
		"  `tp=0.04` — take profit % (default: disabled)\n" +
		"  `commission=5` — round-trip commission USD (default: 0)\n" +
		"  `period=1y` — data period (default: 1y)\n\n" +
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

	ctx := context.Background()
	price, currency, err := data.FetchLatestPrice(ctx, sym)
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
	sb.WriteString("📈 *Built-in Strategies*\n\n")
	for key, meta := range backtest.StrategyRegistry {
		sb.WriteString(fmt.Sprintf("• `%s` — *%s*\n  _%s_\n  Params: ", key, meta.Name, meta.Description))
		var params []string
		for k, v := range meta.Params {
			params = append(params, fmt.Sprintf("%s=%.0f", k, v))
		}
		sb.WriteString(strings.Join(params, ", "))
		sb.WriteString("\n\n")
	}
	sb.WriteString("Usage: `/backtest SYMBOL INTERVAL STRATEGY [param=value ...]`")
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
	stratParams := extractStratParams(opts, stratKey)

	// Engine config
	capital := getOptFloat(opts, "capital", 10000)
	posSizePct := getOptFloat(opts, "pos", 0.02)
	slPct := getOptFloat(opts, "sl", 0)
	tpPct := getOptFloat(opts, "tp", 0)
	commission := getOptFloat(opts, "commission", 0)
	period := getOptStr(opts, "period", defaultPeriod(interval))

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
	ctx := context.Background()
	bars, err := data.FetchOHLCV(ctx, data.FetchParams{
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
	engine.SetStrategy(meta.Factory())

	result, err := engine.Run(params)
	if err != nil {
		b.send(chatID, fmt.Sprintf("❌ Backtest error: %v", err))
		return
	}

	// Send result
	b.sendMD(chatID, backtest.FormatResult(result, capital))

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

	ctx := context.Background()
	response, err := b.stratCreator.Chat(ctx, chatID, text)
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

	ctx := context.Background()
	md, err := b.stratCreator.GenerateStrategyMD(ctx, chatID, stratName)
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

	ctx := context.Background()
	response, err := b.stratCreator.QuickAnalysis(ctx, args)
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
			if cut < 0 {
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
		for strings.Contains(line, "**") {
			start := strings.Index(line, "**")
			end := strings.Index(line[start+2:], "**")
			if end < 0 {
				break
			}
			inner := line[start+2 : start+2+end]
			line = line[:start] + inner + line[start+2+end+2:]
		}

		// Italic *text* or _text_ → keep text
		for strings.Contains(line, "*") {
			start := strings.Index(line, "*")
			end := strings.Index(line[start+1:], "*")
			if end < 0 {
				break
			}
			inner := line[start+1 : start+1+end]
			line = line[:start] + inner + line[start+1+end+1:]
		}

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
