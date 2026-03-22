# Trading Backtest Bot

**Purpose**: Telegram bot untuk backtesting strategi trading di futures/CFD (metals, indices, forex, energy) menggunakan data Yahoo Finance gratis, dengan AI analytics via marketriskmonitor.com

**Type**: agent (Golang Telegram Bot)

**Status**: active

## What It Does

- Fetch OHLCV data gratis dari Yahoo Finance untuk 25+ instruments
- Backtest 7 built-in strategies dengan metrics lengkap (Sharpe, MaxDD, PF, dll)
- AI-powered strategy creation via Claude claude-opus-4-6 (extended thinking)
- Generate strategy document dalam format Markdown
- Quick AI analysis untuk pertanyaan market

## Supported Instruments

| Kategori | Symbols |
|---|---|
| Metals | XAUUSD, XAGUSD, COPPER, PALLADIUM, PLATINUM |
| Indices | NQ, ES, YM, RTY |
| Forex | EURUSD, GBPUSD, USDJPY, USDCHF, AUDUSD, NZDUSD, USDCAD, DXY |
| Energy | CL, RB, HO, NG |

## Data Limits (Yahoo Finance)

| Interval | Max History |
|---|---|
| 1m | 7 days |
| 5m/15m/30m | 60 days |
| 1h | ~2 years |
| 1d | 10+ years |

## Built-in Strategies

- `ema_cross` — EMA Crossover (fast=9, slow=21)
- `rsi` — RSI Mean Reversion (period=14, ob=70, os=30)
- `macd` — MACD Crossover (fast=12, slow=26, signal=9)
- `bb_breakout` — Bollinger Band Breakout (period=20, std=2.0)
- `supertrend` — Supertrend (period=10, multiplier=3.0)
- `donchian` — Donchian Breakout/Turtle (period=20)
- `sma_rsi` — SMA + RSI Confluence (sma=50, rsi=14)

## Indicators Library

- SMA, EMA, RSI, MACD, Bollinger Bands
- ATR, Stochastic, VWAP, Donchian Channel, Supertrend

## Telegram Commands

```
/start          — welcome message
/help           — full command reference
/symbols        — list all instruments
/price SYMBOL   — latest price
/intervals      — data interval info
/strategies     — list strategies
/backtest SYMBOL INTERVAL STRATEGY [options]
/strategy       — AI strategy builder mode
/endstrategy    — end AI session
/genmd [name]   — generate strategy MD document
/analyze [q]    — quick AI analysis
```

## Tech Stack

- **Language**: Go 1.22
- **Telegram**: go-telegram-bot-api/v5
- **Data**: Yahoo Finance v8 API (direct HTTP, rate-limited to ~5 req/s)
- **AI**: Configurable endpoint (default: marketriskmonitor.com/api/analyze, Claude claude-opus-4-6 + extended thinking)
- **Logging**: `log/slog` structured logging (handlers), standard `log` (main entry point)

## Architecture

- **Context propagation**: All HTTP and AI calls accept `context.Context` for cancellation and timeouts
- **Rate limiting**: Mutex-based rate limiter (200ms min delay) for Yahoo Finance API
- **Connection pooling**: Shared `http.Client` instances for data fetcher and AI client
- **Concurrency control**: Semaphore pattern (capacity 20) for Telegram message handlers
- **Session management**: TTL-based cleanup (30min interval, 1hr idle) for AI chat sessions
- **Graceful shutdown**: SIGINT/SIGTERM signal handling in main

## Project Structure

```
trading-backtest-bot/
├── cmd/
│   ├── main.go              # entry point with graceful shutdown
│   └── test/main.go         # integration tests
├── internal/
│   ├── data/
│   │   ├── symbols.go       # symbol definitions + case-insensitive lookup
│   │   └── fetcher.go       # Yahoo Finance HTTP client with rate limiting
│   ├── indicators/
│   │   ├── indicators.go    # SMA/EMA/RSI/MACD/BB/ATR/Stoch/VWAP/Donchian/Supertrend
│   │   └── indicators_test.go # 30 unit tests
│   ├── backtest/
│   │   ├── engine.go        # backtesting engine + result computation
│   │   ├── strategies.go    # 7 built-in strategies + registry
│   │   └── engine_test.go   # 13 unit tests
│   ├── ai/
│   │   └── client.go        # AI API client (configurable endpoint/model)
│   ├── strategy/
│   │   └── creator.go       # AI strategy builder + MD generator with session TTL
│   └── bot/
│       └── handlers.go      # Telegram command handlers with semaphore concurrency
├── .env.example
├── Dockerfile
├── docker-compose.yml
├── run.sh
└── go.mod
```

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `TELEGRAM_TOKEN` | Yes | — | Bot token from @BotFather |
| `AI_ENDPOINT` | No | `https://marketriskmonitor.com/api/analyze` | AI API endpoint |
| `AI_MODEL` | No | `claude-opus-4-6` | AI model identifier |

## How to Run

1. Get bot token from @BotFather on Telegram
2. `cp .env.example .env && nano .env` (set TELEGRAM_TOKEN)
3. `./run.sh` or `TELEGRAM_TOKEN=xxx go run cmd/main.go`
4. Or with Docker: `docker-compose up -d`

## Running Tests

```
go test ./internal/indicators/... -v
go test ./internal/backtest/... -v
```

## Integrates With

- **External**: Yahoo Finance (free, no API key), AI proxy (configurable via `AI_ENDPOINT`)
- **Telegram**: via polling (no webhook needed)
