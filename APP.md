# Trading Backtest Bot

**Purpose**: Telegram bot untuk backtesting strategi trading di futures/CFD (metals, indices, forex, energy) menggunakan data Yahoo Finance gratis, dengan AI analytics via marketriskmonitor.com

**Type**: agent (Golang Telegram Bot)

**Status**: active

## What It Does

- Fetch OHLCV data gratis dari Yahoo Finance untuk 25+ instruments
- Backtest 7 built-in strategies dengan metrics lengkap (Sharpe, MaxDD, PF, dll)
- Multi-symbol comparison across instruments
- Parameter optimization via grid search
- Walk-forward out-of-sample validation
- AI-powered strategy creation via Claude claude-opus-4-6 (extended thinking)
- Generate strategy document dalam format Markdown
- Quick AI analysis untuk pertanyaan market
- ASCII equity curve visualization
- Per-user rate limiting for abuse prevention

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
/compare SYM1,SYM2 INTERVAL STRATEGY [options]
/optimize SYMBOL INTERVAL STRATEGY param=min:max:step
/strategy       — AI strategy builder mode
/endstrategy    — end AI session
/genmd [name]   — generate strategy MD document
/analyze [q]    — quick AI analysis
/list           — list saved strategy documents
```

## Backtest Options

| Option | Default | Description |
|---|---|---|
| `capital=N` | 10000 | Starting capital |
| `pos=N` | 0.02 | Position size fraction (2%) |
| `sl=N` | 0 (disabled) | Stop loss fraction |
| `tp=N` | 0 (disabled) | Take profit fraction |
| `commission=N` | 0 | Round-trip commission USD |
| `period=Nd` | varies | Data period (7d, 30d, 1y, 2y) |
| `oos=N` | 0 (disabled) | Walk-forward out-of-sample fraction (0.1-0.5) |

## Tech Stack

- **Language**: Go 1.22
- **Telegram**: go-telegram-bot-api/v5
- **Data**: Yahoo Finance v8 API (direct HTTP, rate-limited to ~5 req/s)
- **AI**: Configurable endpoint (default: marketriskmonitor.com/api/analyze, Claude claude-opus-4-6 + extended thinking)
- **Logging**: `log/slog` structured logging throughout

## Architecture

- **Context propagation**: Cancellable context flows from main -> bot -> all handlers -> HTTP/AI calls
- **Graceful shutdown**: SIGINT/SIGTERM triggers `Bot.Stop()` which calls `StopReceivingUpdates()` and waits for in-flight handlers (65s timeout)
- **Rate limiting**: Mutex-based rate limiter (200ms min delay) for Yahoo Finance API
- **Per-user rate limiting**: Cooldown-based limiter per user per operation category (price: 2s, backtest: 5s, AI: 5s)
- **Connection pooling**: Shared `http.Client` instances for data fetcher and AI client
- **Concurrency control**: Semaphore pattern (capacity 20) + WaitGroup for Telegram message handlers
- **Session management**: TTL-based cleanup (30min interval, 1hr idle) for AI chat sessions
- **Health endpoint**: HTTP /health on port 8080 for Docker/monitoring

## Project Structure

```
trading-backtest-bot/
├── cmd/
│   ├── main.go              # entry point with graceful shutdown + health server
│   └── test/main.go         # integration tests
├── internal/
│   ├── data/
│   │   ├── symbols.go       # symbol definitions + case-insensitive lookup
│   │   ├── fetcher.go       # Yahoo Finance HTTP client with rate limiting
│   │   └── fetcher_test.go  # data utility tests
│   ├── indicators/
│   │   ├── indicators.go    # SMA/EMA/RSI/MACD/BB/ATR/Stoch/VWAP/Donchian/Supertrend
│   │   └── indicators_test.go # 37 unit tests
│   ├── backtest/
│   │   ├── engine.go        # backtesting engine + walk-forward validation
│   │   ├── chart.go         # ASCII equity curve renderer
│   │   ├── chart_test.go    # chart tests
│   │   ├── strategies.go    # 7 built-in strategies + registry
│   │   └── engine_test.go   # 13 unit tests
│   ├── ai/
│   │   └── client.go        # AI API client (configurable endpoint/model)
│   ├── strategy/
│   │   └── creator.go       # AI strategy builder + MD generator with session TTL
│   └── bot/
│       ├── handlers.go      # Telegram command handlers (backtest, compare, optimize, AI)
│       ├── ratelimit.go     # per-user rate limiting
│       └── handlers_test.go # handler + rate limiter tests
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
| `HEALTH_PORT` | No | `8080` | HTTP health check port |
| `STORAGE_DIR` | No | `/storage` | Directory for generated strategy files |

## How to Run

1. Get bot token from @BotFather on Telegram
2. `cp .env.example .env && nano .env` (set TELEGRAM_TOKEN)
3. `./run.sh` or `TELEGRAM_TOKEN=xxx go run cmd/main.go`
4. Or with Docker: `docker-compose up -d`

## Running Tests

```
go test ./internal/... -v          # all tests
go test -race ./internal/...       # with race detector
```

## Integrates With

- **External**: Yahoo Finance (free, no API key), AI proxy (configurable via `AI_ENDPOINT`)
- **Telegram**: via polling (no webhook needed)
