#!/bin/bash
# Trading Backtest Bot - Run Script

export PATH=$PATH:$HOME/.local/go/bin

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

if [ -z "$TELEGRAM_TOKEN" ]; then
  if [ -f .env ]; then
    set -a; source .env; set +a
  fi
fi

if [ -z "$TELEGRAM_TOKEN" ]; then
  echo "❌ TELEGRAM_TOKEN not set"
  echo "Create a .env file: echo 'TELEGRAM_TOKEN=your_token' > .env"
  exit 1
fi

echo "🔨 Building..."
go build -o ./bin/trading-backtest-bot ./cmd/main.go
if [ $? -ne 0 ]; then
  echo "❌ Build failed"
  exit 1
fi

echo "🚀 Starting Trading Backtest Bot..."
./bin/trading-backtest-bot
