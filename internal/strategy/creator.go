package strategy

import (
	"fmt"
	"strings"
	"time"
	"trading-backtest-bot/internal/ai"
	"trading-backtest-bot/internal/backtest"
	"trading-backtest-bot/internal/data"
)

const systemPrompt = `You are an expert quantitative trading strategist and financial analyst specializing in futures, CFD, forex, metals, indices, and energy markets.

Your role is to help traders design, analyze, and optimize trading strategies with rigorous mathematical and statistical thinking.

When discussing strategies:
1. Always consider market microstructure, liquidity, and session timing
2. Provide specific, actionable parameters (not vague ranges)
3. Include risk management rules (position sizing, stop-loss, take-profit)
4. Mention common pitfalls (overfitting, look-ahead bias, survivorship bias)
5. Reference relevant market-specific behavior (e.g., gold's correlation with USD, oil's seasonality)

When outputting strategy documents:
- Use proper Markdown formatting
- Include entry rules, exit rules, filters, and risk parameters
- Add pseudocode or indicator formulas where helpful
- Rate each component: Entry Quality, Exit Quality, Risk Management (1-5 stars)

You have access to these instruments:
- Metals: XAUUSD (Gold), XAGUSD (Silver), COPPER, PALLADIUM, PLATINUM
- Indices: NQ (Nasdaq), ES (S&P500), YM (Dow Jones), RTY (Russell)  
- Forex: EURUSD, GBPUSD, USDJPY, USDCHF, AUDUSD, NZDUSD, USDCAD, DXY
- Energy: CL (Crude Oil), RB (RBOB Gas), HO (Heating Oil), NG (Natural Gas)

Data intervals available: 1m (7d max), 5m/15m/30m (60d max), 1h (2yr), 1d (10yr+)

Available built-in strategies for backtesting:
- ema_cross: EMA Crossover (params: fast, slow)
- rsi: RSI Mean Reversion (params: period, overbought, oversold)
- macd: MACD Crossover (params: fast, slow, signal)
- bb_breakout: Bollinger Band Breakout (params: period, std)
- supertrend: Supertrend (params: period, multiplier)
- donchian: Donchian Breakout (params: period)
- sma_rsi: SMA + RSI Confluence (params: sma_period, rsi_period, overbought, oversold)`

// Creator handles AI-powered strategy creation and analysis
type Creator struct {
	client   *ai.Client
	sessions map[int64][]ai.Message // telegram chat_id -> conversation history
}

func NewCreator() *Creator {
	return &Creator{
		client:   ai.NewClient(),
		sessions: make(map[int64][]ai.Message),
	}
}

// Chat sends a message in the strategy creation conversation
func (c *Creator) Chat(chatID int64, userMessage string) (string, error) {
	if _, ok := c.sessions[chatID]; !ok {
		c.sessions[chatID] = []ai.Message{}
	}

	c.sessions[chatID] = append(c.sessions[chatID], ai.Message{
		Role:    "user",
		Content: userMessage,
	})

	response, err := c.client.Chat(systemPrompt, c.sessions[chatID], 4096)
	if err != nil {
		return "", err
	}

	c.sessions[chatID] = append(c.sessions[chatID], ai.Message{
		Role:    "assistant",
		Content: response,
	})

	return response, nil
}

// ResetSession clears the conversation for a chat
func (c *Creator) ResetSession(chatID int64) {
	delete(c.sessions, chatID)
}

// GenerateStrategyMD generates a full strategy document in Markdown
func (c *Creator) GenerateStrategyMD(chatID int64, strategyName string) (string, error) {
	prompt := fmt.Sprintf(`Based on our conversation, generate a complete trading strategy document in Markdown for: "%s"

Include these sections (be concise but complete):

# %s

## Overview
Type, best instruments, timeframes, market conditions.

## Entry Rules
Exact conditions, confirmation filters, session filters.

## Exit Rules
Take profit, stop loss, time-based exit.

## Position Sizing & Risk Management
Risk per trade, leverage, daily loss limit.

## Parameters
| Parameter | Default | Range | Description |
|---|---|---|---|

## Entry/Exit Pseudocode

## Backtest Command
/backtest SYMBOL INTERVAL STRATEGY [params]

## Performance Expectations & Limitations

---
Generated: %s`, strategyName, strategyName, time.Now().Format("2006-01-02 15:04"))

	// Build messages: existing session context + genmd prompt
	msgs := make([]ai.Message, len(c.sessions[chatID]))
	copy(msgs, c.sessions[chatID])
	msgs = append(msgs, ai.Message{Role: "user", Content: prompt})

	// Use thinking with conservative budget (3000) and max_tokens (4096)
	// to stay well within API timeout limits
	response, err := c.client.ChatWithThinking(systemPrompt, msgs, 4096, 3000)
	if err != nil {
		// Fallback: try without thinking if it fails
		response, err = c.client.Chat(systemPrompt, msgs, 4096)
		if err != nil {
			return "", fmt.Errorf("strategy generation failed: %w", err)
		}
	}

	return response, nil
}

// AnalyzeBacktestResult uses AI to analyze backtest results
func (c *Creator) AnalyzeBacktestResult(chatID int64, result *backtest.Result, symbol data.Symbol) (string, error) {
	summary := fmt.Sprintf(`Analyze this backtest result and provide insights:

**Instrument:** %s (%s) - %s
**Strategy:** %s
**Period:** %s
**Timeframe:** %s

**Performance Metrics:**
- Total Trades: %d
- Win Rate: %.1f%% (%d wins / %d losses)
- Total P&L: $%.2f (%.2f%%)
- Profit Factor: %.2f
- Sharpe Ratio: %.2f
- Max Drawdown: $%.2f (%.2f%%)
- Avg Win: $%.2f
- Avg Loss: $%.2f
- Max Consecutive Wins: %d
- Max Consecutive Losses: %d

Please provide:
1. Overall assessment (is this strategy viable?)
2. Statistical quality analysis
3. Risk-adjusted performance evaluation
4. Market regime suitability
5. Specific improvement suggestions
6. Parameter optimization recommendations
7. Warning signs or red flags`,
		result.Symbol, symbol.Ticker, symbol.Description,
		result.Strategy,
		result.Period,
		result.Interval,
		result.TotalTrades,
		result.WinRate, result.WinningTrades, result.LosingTrades,
		result.TotalPnL, result.TotalPnLPct,
		result.ProfitFactor,
		result.SharpeRatio,
		result.MaxDrawdown, result.MaxDrawdownPct,
		result.AvgWin, result.AvgLoss,
		result.MaxConsecWins, result.MaxConsecLoss,
	)

	return c.Chat(chatID, summary)
}

// QuickAnalysis does a quick AI analysis without session context
func (c *Creator) QuickAnalysis(prompt string) (string, error) {
	messages := []ai.Message{{Role: "user", Content: prompt}}
	return c.client.Chat(systemPrompt, messages, 2048)
}

// FormatMDForTelegram converts markdown to Telegram-compatible format
// Telegram MarkdownV2 has different rules
func FormatMDForTelegram(md string) string {
	// Keep it simple - Telegram supports basic markdown
	// Just trim and return
	return strings.TrimSpace(md)
}
