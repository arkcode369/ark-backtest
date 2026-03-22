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
- **Data**: Yahoo Finance v8 API (direct HTTP)
- **AI**: marketriskmonitor.com/api/analyze (Claude claude-opus-4-6 + extended thinking)

## Project Structure

```
trading-backtest-bot/
├── cmd/
│   ├── main.go          # entry point
│   └── test/main.go     # integration tests
├── internal/
│   ├── data/
│   │   ├── symbols.go   # symbol definitions + lookup
│   │   └── fetcher.go   # Yahoo Finance HTTP client
│   ├── indicators/
│   │   └── indicators.go # SMA/EMA/RSI/MACD/BB/ATR/Stoch/VWAP/Donchian/Supertrend
│   ├── backtest/
│   │   ├── engine.go    # backtesting engine + result computation
│   │   └── strategies.go # 7 built-in strategies + registry
│   ├── ai/
│   │   └── client.go    # Claude API client (regular + thinking mode)
│   ├── strategy/
│   │   └── creator.go   # AI strategy builder + MD generator
│   └── bot/
│       └── handlers.go  # all Telegram command handlers
├── bin/
│   └── trading-backtest-bot  # compiled binary
├── .env.example
├── run.sh
└── go.mod
```

## How to Run

1. Get bot token from @BotFather on Telegram
2. `cp .env.example .env && nano .env` (set TELEGRAM_TOKEN)
3. `./run.sh` or `TELEGRAM_TOKEN=xxx ./bin/trading-backtest-bot`

## Integrates With

- **External**: Yahoo Finance (free, no API key), marketriskmonitor.com (Claude AI proxy)
- **Telegram**: via polling (no webhook needed)
