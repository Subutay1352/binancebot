// list-symbols: Binance Futures'ta işlemde olan tüm USDT çiftlerini listeler.
// .env'deki BINANCE_* ve BINANCE_FUTURES_TESTNET kullanır (testnet/mainnet).
package main

import (
	"context"
	"fmt"
	"log"
	"sort"

	"binancebot/config"
	"binancebot/internal/binance"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("config:", err)
	}
	client := binance.NewClient(cfg)
	ctx := context.Background()

	symbols, err := client.AllFuturesSymbols(ctx)
	if err != nil {
		log.Fatal("semboller alınamadı:", err)
	}
	sort.Strings(symbols)

	fmt.Printf("Toplam %d USDT futures çifti (TRADING):\n\n", len(symbols))
	for _, s := range symbols {
		fmt.Println(s)
	}
	fmt.Println()
	fmt.Println("TRADE_SYMBOLS için (virgülle kopyala):")
	fmt.Println(join(symbols, ","))
}

func join(a []string, sep string) string {
	if len(a) == 0 {
		return ""
	}
	s := a[0]
	for i := 1; i < len(a); i++ {
		s += sep + a[i]
	}
	return s
}
