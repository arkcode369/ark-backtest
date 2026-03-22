package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
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

	// TODO: Replace log with structured logging (e.g., slog or zerolog) for production use.

	// Graceful shutdown: listen for SIGINT and SIGTERM
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Run the bot in a goroutine since b.Run() blocks on the Telegram update channel
	go func() {
		b.Run()
	}()

	// Block until a shutdown signal is received
	sig := <-sigCh
	log.Printf("Received signal %v, shutting down gracefully...", sig)
	os.Exit(0)
}
