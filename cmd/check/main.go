// check: Config, PostgreSQL ve isteğe bağlı Binance bağlantısını test eder. İşlem açmaz.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"binancebot/config"
	"binancebot/internal/binance"
	"binancebot/internal/db"
)

func main() {
	log.Println("1. Config yükleniyor...")
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("   Config hatası: %v", err)
	}
	log.Println("   OK")

	log.Println("2. PostgreSQL bağlantısı...")
	ctx := context.Background()
	store, err := db.NewStore(ctx, cfg)
	if err != nil {
		log.Fatalf("   PostgreSQL hatası: %v", err)
	}
	store.Close()
	log.Println("   OK")

	if cfg.Binance.APIKey != "" && cfg.Binance.SecretKey != "" {
		log.Println("3. Binance (fiyat) testi...")
		client := binance.NewClient(cfg)
		price, err := client.GetPrice(ctx, cfg.Trade.Symbol)
		if err != nil {
			log.Printf("   Uyarı (API key/network): %v", err)
		} else {
			log.Printf("   OK - %s fiyat: %v", cfg.Trade.Symbol, price)
		}
	} else {
		log.Println("3. Binance atlandı (API key yok)")
	}

	if cfg.Telegram.BotToken != "" && cfg.Telegram.ChatID != "" {
		log.Println("4. Telegram testi (opsiyonel)...")
		log.Println("   Bot token ve chat_id dolu - çalışırken bildirim gider.")
	} else {
		log.Println("4. Telegram atlandı (token/chat_id yok)")
	}

	fmt.Fprintln(os.Stdout, "\nTüm kontroller tamam. Bot'u çalıştırmak için: go run ./cmd/bot")
}
