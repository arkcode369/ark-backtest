package main

import (
	"fmt"
	"log"
	"os"
	"trading-backtest-bot/internal/bot"

	"github.com/joho/godotenv"
)

func main() {
	// Load .env file if exists
	godotenv.Load()

	token := os.Getenv("TELEGRAM_TOKEN")
	if token == "" {
		fmt.Println("❌ TELEGRAM_TOKEN environment variable not set")
		fmt.Println("Set it with: export TELEGRAM_TOKEN=your_bot_token")
		fmt.Println("Or create a .env file with: TELEGRAM_TOKEN=your_bot_token")
		os.Exit(1)
	}

	log.Println("🚀 Starting Trading Backtest Bot...")

	b, err := bot.New(token)
	if err != nil {
		log.Fatalf("❌ Failed to create bot: %v", err)
	}

	b.Run()
}
