package strategy

import (
	"context"
	"fmt"
	"strings"
	"sync"
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

Respond clearly and concisely. Use plain text formatting (no markdown tables or code blocks).

You have access to these instruments:
- Metals: XAUUSD (Gold), XAGUSD (Silver), COPPER, PALLADIUM, PLATINUM
- Indices: NQ (Nasdaq), ES (S&P500), YM (Dow Jones), RTY (Russell)
- Forex: EURUSD, GBPUSD, USDJPY, USDCHF, AUDUSD, NZDUSD, USDCAD, DXY
- Energy: CL (Crude Oil), RB (RBOB Gas), HO (Heating Oil), NG (Natural Gas)

Data intervals: 1m (7d max), 5m/15m/30m (60d), 1h (2yr), 1d (10yr+)

Built-in strategies: ema_cross, rsi, macd, bb_breakout, supertrend, donchian, sma_rsi`

const (
	sessionCleanupInterval = 30 * time.Minute
	sessionMaxIdleTime     = 1 * time.Hour
)

// sessionEntry wraps a session with a last-used timestamp for TTL cleanup
type sessionEntry struct {
	session  *ai.Session
	lastUsed time.Time
}

// Creator handles AI-powered strategy creation and analysis
type Creator struct {
	client   *ai.Client
	mu       sync.Mutex
	sessions map[int64]*sessionEntry // chat_id → session entry with TTL tracking
	cancel   context.CancelFunc
}

func NewCreator() *Creator {
	ctx, cancel := context.WithCancel(context.Background())
	c := &Creator{
		client:   ai.NewClient(),
		sessions: make(map[int64]*sessionEntry),
		cancel:   cancel,
	}
	go c.cleanupLoop(ctx)
	return c
}

// Stop cancels the cleanup goroutine
func (c *Creator) Stop() {
	c.cancel()
}

// cleanupLoop periodically removes sessions idle for more than sessionMaxIdleTime
func (c *Creator) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(sessionCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.mu.Lock()
			now := time.Now()
			for chatID, entry := range c.sessions {
				if now.Sub(entry.lastUsed) > sessionMaxIdleTime {
					delete(c.sessions, chatID)
				}
			}
			c.mu.Unlock()
		}
	}
}

// getOrCreate returns the session for a chat, creating one if needed (thread-safe)
func (c *Creator) getOrCreate(chatID int64) *ai.Session {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.sessions[chatID]
	if !ok {
		entry = &sessionEntry{
			session:  ai.NewSession(),
			lastUsed: time.Now(),
		}
		c.sessions[chatID] = entry
	} else {
		entry.lastUsed = time.Now()
	}
	return entry.session
}

// Chat sends a message in the strategy creation conversation
func (c *Creator) Chat(ctx context.Context, chatID int64, userMessage string) (string, error) {
	sess := c.getOrCreate(chatID)

	sess.Append(ai.Message{Role: "user", Content: userMessage})

	msgs := sess.Snapshot()
	response, err := c.client.Chat(ctx, systemPrompt, msgs, 4096)
	if err != nil {
		// Atomic rollback: create a new session with all messages except the last
		// (the failed user message), then swap it in under the lock.
		rollbackMsgs := msgs[:len(msgs)-1]
		newSess := ai.NewSession()
		for _, m := range rollbackMsgs {
			newSess.Append(m)
		}
		c.mu.Lock()
		if entry, ok := c.sessions[chatID]; ok {
			entry.session = newSess
			entry.lastUsed = time.Now()
		}
		c.mu.Unlock()
		return "", err
	}

	sess.Append(ai.Message{Role: "assistant", Content: response})
	return response, nil
}

// ResetSession clears the conversation for a chat
func (c *Creator) ResetSession(chatID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.sessions, chatID)
}

// GenerateStrategyMD generates a full strategy document in Markdown
func (c *Creator) GenerateStrategyMD(ctx context.Context, chatID int64, strategyName string) (string, error) {
	sess := c.getOrCreate(chatID)

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

## Entry/Exit Logic (Pseudocode)

## Backtest Command
/backtest SYMBOL INTERVAL STRATEGY [params]

## Performance Expectations & Limitations

---
Generated: %s`, strategyName, strategyName, time.Now().Format("2006-01-02 15:04"))

	// Build message list: session history + genmd prompt (don't add to session)
	history := sess.Snapshot()
	msgs := append(history, ai.Message{Role: "user", Content: prompt})

	// Try with thinking first (budget=2048, max_tokens=4096 — conservative)
	response, err := c.client.ChatWithThinking(ctx, systemPrompt, msgs, 4096, 2048)
	if err != nil {
		// Fallback: without thinking
		response, err = c.client.Chat(ctx, systemPrompt, msgs, 4096)
		if err != nil {
			return "", fmt.Errorf("strategy generation failed: %w", err)
		}
	}

	return response, nil
}

// AnalyzeBacktestResult uses AI to analyze backtest results
func (c *Creator) AnalyzeBacktestResult(ctx context.Context, chatID int64, result *backtest.Result, symbol data.Symbol) (string, error) {
	pf := fmt.Sprintf("%.2f", result.ProfitFactor)
	if result.ProfitFactor > 999 {
		pf = "∞"
	}

	summary := fmt.Sprintf(`Analyze this backtest result concisely:

Instrument: %s (%s) - %s
Strategy: %s | Period: %s | Timeframe: %s

Metrics:
- Trades: %d | WinRate: %.1f%% (%dW/%dL)
- P&L: $%.2f (%.2f%%) | PF: %s | Sharpe: %.2f
- MaxDD: $%.2f (%.2f%%)
- AvgWin: $%.2f | AvgLoss: $%.2f
- MaxConsecWins: %d | MaxConsecLoss: %d

Give: assessment, strengths, weaknesses, and 3 specific improvements.`,
		result.Symbol, symbol.Ticker, symbol.Description,
		result.Strategy, result.Period, result.Interval,
		result.TotalTrades, result.WinRate, result.WinningTrades, result.LosingTrades,
		result.TotalPnL, result.TotalPnLPct, pf, result.SharpeRatio,
		result.MaxDrawdown, result.MaxDrawdownPct,
		result.AvgWin, result.AvgLoss,
		result.MaxConsecWins, result.MaxConsecLoss,
	)

	return c.Chat(ctx, chatID, summary)
}

// QuickAnalysis does a one-shot AI analysis without session context
func (c *Creator) QuickAnalysis(ctx context.Context, prompt string) (string, error) {
	if strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("empty prompt")
	}
	messages := []ai.Message{{Role: "user", Content: prompt}}
	return c.client.Chat(ctx, systemPrompt, messages, 2048)
}
