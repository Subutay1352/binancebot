package bot

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"binancebot/config"
	"binancebot/internal/binance"
	"binancebot/internal/db"
	"binancebot/internal/risk"
	"binancebot/internal/strategy"
	"binancebot/internal/telegram"

	"github.com/adshao/go-binance/v2/futures"
)

// Bot iki katmanlı çalışır:
// 1) Scanner (cron): Tüm sembollerde uygun koşulları arar.
// 2) Executor (sembol başına goroutine): Bulunan coinde işlemi açar, kapanana kadar takip eder, bitince goroutine sonlanır.
type Bot struct {
	cfg      *config.Config
	store    *db.Store
	client   *binance.Client
	strat    strategy.Strategy
	telegram *telegram.Notifier
	interval time.Duration

	mu            sync.Mutex
	activeSymbols map[string]struct{} // Şu an executor'da olan semboller (çift giriş engeli)

	openSlotMu    sync.Mutex // Açılış sırasında race önler
	reservedSlots int       // Şu an openPosition() içinde olan executor sayısı (açılış bitince hemen düşer)
}

// New Bot oluşturur. checkInterval = scanner'ın tüm sembolleri tarama sıklığı.
func New(cfg *config.Config, store *db.Store, client *binance.Client, strat strategy.Strategy, tg *telegram.Notifier, checkInterval time.Duration) *Bot {
	if checkInterval <= 0 {
		checkInterval = 2 * time.Minute
	}
	return &Bot{
		cfg:           cfg,
		store:         store,
		client:        client,
		strat:         strat,
		telegram:      tg,
		interval:      checkInterval,
		activeSymbols: make(map[string]struct{}),
	}
}

// Run scanner döngüsünü başlatır. ctx iptal edilene kadar çalışır.
// Her tick'te tüm sembolleri tarar; koşul sağlayan ve henüz pozisyonu olmayan semboller için executor goroutine spawn eder.
func (b *Bot) Run(ctx context.Context) {
	symbols := b.cfg.Trade.Symbols
	if len(symbols) == 0 {
		symbols = []string{b.cfg.Trade.Symbol}
	}
	if b.cfg.Trade.InstanceID == "" {
		b.cfg.Trade.InstanceID, _ = os.Hostname()
	}
	bal, avail, _ := b.client.GetUSDTBalanceDetails(ctx)
	log.Printf("[bot] başlatıldı | instance=%s | bakiye=%.2f USDT (kullanılabilir=%.2f) | tarama aralığı=%v | semboller=%v | max_açık_pozisyon=%d", b.cfg.Trade.InstanceID, bal, avail, b.interval, symbols, b.cfg.Trade.MaxOpenTrades)

	if b.cfg.Binance.Testnet {
		log.Printf("[bot] testnet: açık algo emirleri ve pozisyonlar temizleniyor...")
		if err := b.client.CancelAllOpenAlgoOrders(ctx); err != nil {
			log.Printf("[bot] testnet: algo temizliği atlandı (testnet imza hatası olabilir, bot çalışmaya devam ediyor): %v", err)
		}
		if err := b.client.CloseAllOpenPositions(ctx); err != nil {
			log.Printf("[bot] testnet temizlik (pozisyon): %v", err)
		}
		if err := b.store.DeleteAllTrades(ctx); err != nil {
			log.Printf("[bot] testnet DB temizlik hatası: %v", err)
		} else {
			log.Printf("[bot] testnet: trades tablosu temizlendi")
		}
		log.Printf("[bot] testnet temizlik bitti")
	}

	runScan := func() {
		log.Printf("[scanner] tur başladı | sembol sayısı=%d", len(symbols))
		for _, symbol := range symbols {
			symbol := symbol
			if b.cfg.Trade.MaxOpenTrades > 0 {
				openList, err := b.store.OpenTrades(ctx)
				if err != nil {
					log.Printf("[scanner] açık pozisyon sayısı alınamadı: %v", err)
					break
				}
				if len(openList) >= b.cfg.Trade.MaxOpenTrades {
					log.Printf("[scanner] max açık pozisyona ulaşıldı (açık=%d, max=%d), tarama durduruldu", len(openList), b.cfg.Trade.MaxOpenTrades)
					break
				}
			}
			if err := b.scanOne(ctx, symbol); err != nil {
				log.Printf("[scanner] %s hata: %v", symbol, err)
				_ = b.telegram.Send(ctx, "⚠️ Scanner "+symbol+": "+err.Error())
			}
		}
		log.Printf("[scanner] tur bitti")
	}

	runScan()
	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Printf("[bot] durduruluyor")
			return
		case <-ticker.C:
			runScan()
		}
	}
}

// scanOne tek sembolü tarar: açık pozisyon veya aktif executor yoksa stratejiye sorar; Long/Short ise executor başlatır.
func (b *Bot) scanOne(ctx context.Context, symbol string) error {
	if b.isActive(symbol) {
		log.Printf("[scanner] %s atlandı (executor zaten çalışıyor)", symbol)
		return nil
	}
	openTrade, err := b.store.OpenTradeBySymbol(ctx, symbol)
	if err != nil {
		return err
	}
	if openTrade != nil {
		log.Printf("[scanner] %s atlandı (açık pozisyon var, id=%d)", symbol, openTrade.ID)
		return nil
	}

	sig, err := b.strat.Decide(ctx, symbol)
	if err != nil {
		log.Printf("[scanner] %s strateji hata: %v", symbol, err)
		return err
	}
	if sig == strategy.Hold {
		log.Printf("[scanner] %s sinyal=HOLD", symbol)
		return nil
	}

	// Limit: executor başlatmadan hemen önce tekrar kontrol (race / çoklu sembol için)
	if b.cfg.Trade.MaxOpenTrades > 0 {
		openList, err := b.store.OpenTrades(ctx)
		if err != nil {
			return err
		}
		if len(openList) >= b.cfg.Trade.MaxOpenTrades {
			log.Printf("[scanner] %s sinyal=%s ATLANDI | max açık pozisyon (açık=%d, max=%d)", symbol, sig, len(openList), b.cfg.Trade.MaxOpenTrades)
			return nil
		}
	}

	log.Printf("[scanner] %s sinyal=%s → executor başlatılıyor", symbol, sig)
	b.setActive(symbol, true)
	go b.runExecutor(ctx, symbol, sig)
	return nil
}

// runExecutor tek sembol için: pozisyon açar, kapanana kadar bekler, kapanınca DB + bildirim yapar ve goroutine biter.
func (b *Bot) runExecutor(ctx context.Context, symbol string, sig strategy.Signal) {
	defer b.setActive(symbol, false)
	log.Printf("[executor] %s başladı | sinyal=%s", symbol, sig)

	// Limit: açık + "açılış yapıyor" (rezerve) sayısı max'ı geçmesin; rezerve openPosition bitince hemen düşer
	if b.cfg.Trade.MaxOpenTrades > 0 {
		b.openSlotMu.Lock()
		openList, err := b.store.OpenTrades(ctx)
		if err != nil {
			b.openSlotMu.Unlock()
			log.Printf("[executor] %s açık pozisyon sayısı alınamadı: %v", symbol, err)
			return
		}
		used := len(openList) + b.reservedSlots
		if used >= b.cfg.Trade.MaxOpenTrades {
			b.openSlotMu.Unlock()
			log.Printf("[executor] %s atlandı | max (açık=%d + açılışta=%d >= max=%d)", symbol, len(openList), b.reservedSlots, b.cfg.Trade.MaxOpenTrades)
			return
		}
		b.reservedSlots++
		b.openSlotMu.Unlock()
	}

	err := b.openPosition(ctx, symbol, sig)
	if b.cfg.Trade.MaxOpenTrades > 0 {
		b.openSlotMu.Lock()
		b.reservedSlots--
		b.openSlotMu.Unlock()
	}
	if err != nil {
		side := "SHORT"
		if sig == strategy.Long {
			side = "LONG"
		}
		var instID *string
		if b.cfg.Trade.InstanceID != "" {
			instID = &b.cfg.Trade.InstanceID
		}
		if _, dbErr := b.store.InsertPositionOpenError(ctx, symbol, side, err.Error(), instID); dbErr != nil {
			log.Printf("[executor] %s hata DB'ye yazılamadı: %v", symbol, dbErr)
		}
		log.Printf("[executor] %s pozisyon açılamadı: %v", symbol, err)
		_ = b.telegram.Send(ctx, "⚠️ "+symbol+" açılamadı: "+err.Error())
		return
	}
	pollInterval := b.executorPollInterval()
	log.Printf("[executor] %s pozisyon açıldı, kapanış bekleniyor (kontrol aralığı=%v)", symbol, pollInterval)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Printf("[executor] %s iptal (ctx)", symbol)
			return
		case <-ticker.C:
			openTrade, err := b.store.OpenTradeBySymbol(ctx, symbol)
			if err != nil || openTrade == nil {
				log.Printf("[executor] %s açık kayıt yok, bitiriliyor", symbol)
				return
			}
			stillOpen, err := b.isPositionStillOpen(ctx, openTrade)
			if err != nil {
				log.Printf("[executor] %s pozisyon kontrolü hata: %v", symbol, err)
				continue
			}
			if !stillOpen {
				log.Printf("[executor] %s pozisyon borsada kapalı, DB güncelleniyor", symbol)
				_ = b.checkPositionClosed(ctx, openTrade)
				return
			}
		}
	}
}

func (b *Bot) isPositionStillOpen(ctx context.Context, t *db.Trade) (bool, error) {
	pos, err := b.client.GetPosition(ctx, t.Symbol)
	if err != nil {
		return true, err
	}
	if pos == nil {
		return false, nil
	}
	amt := parseFloat(pos.PositionAmt)
	if t.Side == "LONG" && amt > 0 {
		return true, nil
	}
	if t.Side == "SHORT" && amt < 0 {
		return true, nil
	}
	return false, nil
}

func (b *Bot) isActive(symbol string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.activeSymbols[symbol]
	return ok
}

// isSymbolAllowed sembolün TRADE_SYMBOLS (veya Trade.Symbol) listesinde olup olmadığını döner.
func (b *Bot) executorPollInterval() time.Duration {
	sec := b.cfg.Trade.ExecutorPollSec
	if sec <= 0 {
		sec = 60
	}
	return time.Duration(sec) * time.Second
}

func (b *Bot) isSymbolAllowed(symbol string) bool {
	symbol = strings.TrimSpace(strings.ToUpper(symbol))
	list := b.cfg.Trade.Symbols
	if len(list) == 0 {
		return symbol == strings.TrimSpace(strings.ToUpper(b.cfg.Trade.Symbol))
	}
	for _, s := range list {
		if strings.TrimSpace(strings.ToUpper(s)) == symbol {
			return true
		}
	}
	return false
}

func (b *Bot) activeCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.activeSymbols)
}

func (b *Bot) setActive(symbol string, add bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if add {
		b.activeSymbols[symbol] = struct{}{}
	} else {
		delete(b.activeSymbols, symbol)
	}
}

func (b *Bot) checkPositionClosed(ctx context.Context, t *db.Trade) error {
	exitPrice, _ := b.client.GetPrice(ctx, t.Symbol)
	realizedPnl := b.realizedPnl(t, exitPrice)
	balanceAfter, _ := b.client.GetUSDTBalance(ctx)
	balanceAfterPtr := &balanceAfter
	log.Printf("[trade] KAPANIŞ | symbol=%s side=%s trade_id=%d entry=%.4f exit=%.4f quantity=%.6f realized_pnl=%.4f | bakiye_öncesi=%.2f USDT bakiye_sonrası=%.2f USDT",
		t.Symbol, t.Side, t.ID, t.EntryPrice, exitPrice, t.Quantity, realizedPnl, ptrFloat(t.BalanceBeforeUsdt), balanceAfter)
	if err := b.store.CloseTrade(ctx, t.ID, exitPrice, realizedPnl, balanceAfterPtr, "SL_TP_OR_MANUAL"); err != nil {
		log.Printf("[trade] KAPANIŞ DB hatası: %v", err)
		return err
	}
	msg := fmt.Sprintf("🔴 Pozisyon kapandı\n%s %s\nÇıkış: %.8f | PnL: %.4f | Bakiye: %.4f USDT", t.Symbol, t.Side, exitPrice, realizedPnl, balanceAfter)
	return b.telegram.Send(ctx, msg)
}

func (b *Bot) realizedPnl(t *db.Trade, exitPrice float64) float64 {
	if t.Side == "LONG" {
		return (exitPrice - t.EntryPrice) * t.Quantity
	}
	return (t.EntryPrice - exitPrice) * t.Quantity
}

func (b *Bot) openPosition(ctx context.Context, symbol string, sig strategy.Signal) error {
	if !b.isSymbolAllowed(symbol) {
		return fmt.Errorf("sembol config'te yok, işlem açılmıyor: %s (izinli: %v)", symbol, b.cfg.Trade.Symbols)
	}
	balBefore, availBefore, _ := b.client.GetUSDTBalanceDetails(ctx)
	log.Printf("[trade] AÇILIŞ hazırlanıyor | symbol=%s sinyal=%s | bakiye=%.2f USDT (kullanılabilir=%.2f)", symbol, sig, balBefore, availBefore)

	lev := b.cfg.Trade.Leverage
	if lev < 1 {
		lev = 1
	}
	if err := b.client.SetLeverage(ctx, symbol, lev); err != nil {
		log.Printf("[trade] %s kaldıraç ayarlanamadı (atlanıyor): %v", symbol, err)
		return err
	}

	price, err := b.client.GetPrice(ctx, symbol)
	if err != nil {
		return err
	}

	info, err := b.client.ExchangeInfo(ctx, symbol)
	if err != nil {
		return err
	}

	quantity := b.quantity(price, info)
	if quantity <= 0 {
		return fmt.Errorf("quantity 0 veya negatif")
	}
	log.Printf("[trade] AÇILIŞ | symbol=%s quantity=%.6f fiyat=%.4f", symbol, quantity, price)

	var orderResp *futures.CreateOrderResponse
	side := "LONG"
	if sig == strategy.Long {
		orderResp, err = b.client.OpenLong(ctx, symbol, quantity)
	} else {
		orderResp, err = b.client.OpenShort(ctx, symbol, quantity)
		side = "SHORT"
	}
	if err != nil {
		return err
	}

	entryPrice := price
	if orderResp.AvgPrice != "" {
		entryPrice = parseFloat(orderResp.AvgPrice)
	}
	if entryPrice == 0 {
		entryPrice = price
	}

	// SL/TP yüzdeleri marj bazlı: marj_risk% = fiyat_hareket% * kaldıraç → fiyat_hareket% = marj_risk% / kaldıraç
	slPct := b.cfg.Trade.StopLossPercent / float64(lev)
	tpPct := b.cfg.Trade.TakeProfitPercent / float64(lev)
	rawSL, rawTP := risk.Prices(entryPrice, slPct, tpPct, side)
	tickSize := info.PriceFilter().TickSize
	slPrice := roundToTick(rawSL, tickSize)
	tpPrice := roundToTick(rawTP, tickSize)
	// Düşük fiyatlı coinlerde tick size büyükse (örn. 0.01) SL/TP 0'a yuvarlanabilir; entry ile aynı hassasiyeti kullan
	entryDecimals := priceDecimals(entryPrice)
	if slPrice == 0 && rawSL != 0 {
		slPrice = roundToDecimals(rawSL, entryDecimals)
	}
	if tpPrice == 0 && rawTP != 0 {
		tpPrice = roundToDecimals(rawTP, entryDecimals)
	}
	slSide := futures.SideTypeSell
	if side == "SHORT" {
		slSide = futures.SideTypeBuy
	}
	qtyStr := formatQty(quantity)
	slStr := formatPriceForAPI(slPrice, tickSize, entryDecimals)
	tpStr := formatPriceForAPI(tpPrice, tickSize, entryDecimals)

	_, errSL := b.client.PlaceStopLoss(ctx, symbol, slSide, qtyStr, slStr)
	if errSL != nil {
		log.Printf("[trade] SL KONAMADI | symbol=%s: %v → pozisyon kapatılıyor", symbol, errSL)
		_ = b.client.ClosePositionMarket(ctx, symbol, side, quantity)
		return fmt.Errorf("stop loss konamadı: %w", errSL)
	}
	_, errTP := b.client.PlaceTakeProfit(ctx, symbol, slSide, qtyStr, tpStr)
	if errTP != nil {
		log.Printf("[trade] TP KONAMADI | symbol=%s: %v → pozisyon kapatılıyor", symbol, errTP)
		_ = b.client.ClosePositionMarket(ctx, symbol, side, quantity)
		return fmt.Errorf("take profit konamadı: %w", errTP)
	}

	orderIDStr := strconv.FormatInt(orderResp.OrderID, 10)
	balanceBeforePtr := &balBefore
	instanceID := b.cfg.Trade.InstanceID
	marginUSD := b.cfg.Trade.PositionSizeUSD
	notionalUSD := quantity * entryPrice
	t := &db.Trade{
		Symbol:             symbol,
		Side:               side,
		EntryPrice:         entryPrice,
		Quantity:           quantity,
		StopLoss:           &slPrice,
		TakeProfit:         &tpPrice,
		BinanceOrderID:     &orderIDStr,
		BalanceBeforeUsdt:  balanceBeforePtr,
		OpenedAt:           time.Now(),
		Leverage:           lev,
		MarginUsdt:         &marginUSD,
		NotionalUsdt:       &notionalUSD,
	}
	if instanceID != "" {
		t.InstanceID = &instanceID
	}
	if _, err := b.store.InsertTrade(ctx, t); err != nil {
		log.Printf("[trade] AÇILIŞ DB hatası: %v", err)
		return err
	}
	log.Printf("[trade] AÇILIŞ tamamlandı | symbol=%s side=%s trade_id=%d order_id=%s entry=%.8f sl=%.8f tp=%.8f | bakiye_önce=%.2f USDT",
		symbol, side, t.ID, db.StrVal(t.BinanceOrderID), entryPrice, slPrice, tpPrice, balBefore)

	msg := fmt.Sprintf("🟢 Pozisyon açıldı\n%s %s | Miktar: %s | Giriş: %.8f | SL: %.8f | TP: %.8f\nAna para: %.2f USDT | Kaldıraç: %dx | Genişlik: %.2f USDT",
		symbol, side, qtyStr, entryPrice, slPrice, tpPrice, marginUSD, lev, notionalUSD)
	return b.telegram.Send(ctx, msg)
}

func (b *Bot) quantity(price float64, info *futures.Symbol) float64 {
	margin := b.cfg.Trade.PositionSizeUSD
	lev := b.cfg.Trade.Leverage
	if lev < 1 {
		lev = 1
	}
	notionalUSD := margin * float64(lev)
	qty := notionalUSD / price
	return roundToStep(qty, info.MarketLotSizeFilter().StepSize)
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func formatQty(q float64) string {
	return fmt.Sprintf("%.8f", q)
}

func roundToTick(price float64, tickSizeStr string) float64 {
	tick := parseFloat(tickSizeStr)
	if tick <= 0 {
		return price
	}
	inv := 1.0 / tick
	return float64(int64(price*inv)) * tick
}

// priceDecimals entry fiyatına uygun ondalık basamak sayısı (coin hassasiyeti; düşük fiyatlı coinler 8).
func priceDecimals(entryPrice float64) int {
	if entryPrice <= 0 {
		return 8
	}
	if entryPrice >= 1 {
		return 2
	}
	// Entry < 1: düşük fiyatlı coinlerde SL/TP 0'a düşmesin diye yeterli hassasiyet
	if entryPrice < 0.1 {
		return 8
	}
	return 4
}

func roundToDecimals(price float64, decimals int) float64 {
	if decimals < 0 {
		decimals = 8
	}
	mult := 1.0
	for i := 0; i < decimals; i++ {
		mult *= 10
	}
	return float64(int64(price*mult+0.5)) / mult
}

// formatPriceForAPI Binance'a gönderilecek fiyat string'i; tick size veya entry hassasiyeti kullanır.
func formatPriceForAPI(price float64, tickSizeStr string, entryDecimals int) string {
	tick := parseFloat(tickSizeStr)
	if tick > 0 && price >= tick {
		decimals := tickDecimals(tick)
		return fmt.Sprintf("%.*f", decimals, price)
	}
	return fmt.Sprintf("%.*f", entryDecimals, price)
}

func formatPriceByTick(price float64, tickSizeStr string) string {
	tick := parseFloat(tickSizeStr)
	if tick <= 0 {
		return fmt.Sprintf("%.8f", price)
	}
	decimals := tickDecimals(tick)
	return fmt.Sprintf("%.*f", decimals, price)
}

func tickDecimals(tick float64) int {
	if tick >= 1 || tick <= 0 {
		return 2
	}
	d := 0
	for tick < 1 {
		d++
		tick *= 10
	}
	return d
}

func ptrFloat(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func roundToStep(qty float64, stepStr string) float64 {
	step := parseFloat(stepStr)
	if step <= 0 {
		return qty
	}
	inv := 1.0 / step
	return float64(int64(qty*inv)) * step
}
