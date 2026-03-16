package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"binancebot/config"
	"binancebot/internal/apiserver"
	"binancebot/internal/binance"
	"binancebot/internal/bot"
	"binancebot/internal/db"
	"binancebot/internal/strategy"
	"binancebot/internal/telegram"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("config:", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store, err := db.NewStore(ctx, cfg)
	if err != nil {
		log.Fatal("db:", err)
	}
	defer store.Close()

	client := binance.NewClient(cfg)

	// Dashboard API aynı process'te (Render'da PORT'ta dinler, UI takip için)
	go apiserver.Run(store, client, cfg)

	if !cfg.BotEnabled {
		log.Println("[bot] BOT_ENABLED=false → sadece API/dashboard çalışıyor, tarama ve işlem kapalı")
		<-ctx.Done()
		log.Println("servis durdu")
		return
	}

	strat := strategy.NewExample(client, cfg.Strategy)
	tg := telegram.New(cfg.Telegram.BotToken, cfg.Telegram.ChatID)

	b := bot.New(cfg, store, client, strat, tg, 2*time.Minute)
	log.Println("bot başladı, Ctrl+C ile durdur")
	b.Run(ctx)
	log.Println("bot durdu")
}
