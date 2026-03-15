package main

import (
	"context"
	"log"

	"binancebot/config"
	"binancebot/internal/apiserver"
	"binancebot/internal/binance"
	"binancebot/internal/db"
)

// Sadece API/dashboard çalıştırmak için (local test). Production'da tek servis = bot (cmd/bot), o da API'yi kendi içinde açar.
func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("config:", err)
	}
	ctx := context.Background()
	store, err := db.NewStore(ctx, cfg)
	if err != nil {
		log.Fatal("db:", err)
	}
	defer store.Close()
	bnClient := binance.NewClient(cfg)
	apiserver.Run(store, bnClient)
}
