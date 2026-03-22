package main

import (
	"fmt"
	"log/slog"
	"net/http"
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
		fmt.Println("TELEGRAM_TOKEN environment variable not set")
		fmt.Println("Set it with: export TELEGRAM_TOKEN=your_bot_token")
		fmt.Println("Or create a .env file with: TELEGRAM_TOKEN=your_bot_token")
		os.Exit(1)
	}

	slog.Info("starting Trading Backtest Bot")

	b, err := bot.New(token)
	if err != nil {
		slog.Error("failed to create bot", "error", err)
		os.Exit(1)
	}

	// Health check HTTP endpoint for Docker/monitoring
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		})
		port := os.Getenv("HEALTH_PORT")
		if port == "" {
			port = "8080"
		}
		slog.Info("health endpoint listening", "port", port)
		if err := http.ListenAndServe(":"+port, mux); err != nil {
			slog.Warn("health server error", "error", err)
		}
	}()

	// Graceful shutdown: listen for SIGINT and SIGTERM
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Run the bot in a goroutine since b.Run() blocks
	go b.Run()

	// Block until a shutdown signal is received
	sig := <-sigCh
	slog.Info("received shutdown signal", "signal", sig)
	b.Stop()
	slog.Info("bot stopped")
}
