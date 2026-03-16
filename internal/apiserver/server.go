package apiserver

import (
	_ "embed"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"binancebot/config"
	"binancebot/internal/binance"
	"binancebot/internal/db"

	"github.com/gin-gonic/gin"
)

//go:embed static/index.html
var indexHTML []byte

func logOutboundIP() {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://api.ipify.org")
	if err != nil {
		log.Printf("[api] sunucu outbound IP alınamadı: %v", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	ip := strings.TrimSpace(string(body))
	if ip != "" {
		log.Printf("[api] sunucu outbound IP (Binance kısıtı için): %s", ip)
	}
}

// Run dashboard API'yi PORT'ta başlatır ve bloke eder. cfg UI ayarları varsayılanları için kullanılır.
func Run(store *db.Store, bnClient *binance.Client, cfg *config.Config) {
	logOutboundIP()
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.GET("/", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
	})
	r.GET("/api/trades", func(c *gin.Context) {
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
		list, err := store.ListTrades(c.Request.Context(), limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if list == nil {
			list = []db.Trade{}
		}
		c.JSON(http.StatusOK, list)
	})
	r.GET("/api/open-positions", func(c *gin.Context) {
		list, err := store.OpenTrades(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if list == nil {
			list = []db.Trade{}
		}
		c.JSON(http.StatusOK, list)
	})
	r.POST("/api/close-position", func(c *gin.Context) {
		var body struct {
			Symbol string `json:"symbol"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || body.Symbol == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "symbol gerekli (örn. {\"symbol\": \"BTCUSDT\"})"})
			return
		}
		ctx := c.Request.Context()
		symbol := strings.TrimSpace(strings.ToUpper(body.Symbol))
		trade, err := store.OpenTradeBySymbol(ctx, symbol)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if trade == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "bu sembol için açık pozisyon yok"})
			return
		}
		info, err := bnClient.ExchangeInfo(ctx, symbol)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "sembol bilgisi alınamadı: " + err.Error()})
			return
		}
		stepSize := info.MarketLotSizeFilter().StepSize
		qtyStr := binance.FormatQuantityForAPI(trade.Quantity, stepSize)
		if err := bnClient.ClosePositionMarket(ctx, symbol, trade.Side, qtyStr); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "pozisyon kapatılamadı: " + err.Error()})
			return
		}
		time.Sleep(500 * time.Millisecond)
		var exitPrice float64
		if cfg.Binance.Testnet {
			exitPrice, _ = bnClient.GetPrice(ctx, symbol)
		} else {
			since := time.Now().Add(-2 * time.Minute)
			exitPrice, err = bnClient.GetRecentCloseFillPrice(ctx, symbol, trade.Side, since)
			if err != nil {
				exitPrice, _ = bnClient.GetPrice(ctx, symbol)
			}
		}
		if exitPrice <= 0 {
			exitPrice, _ = bnClient.GetPrice(ctx, symbol)
		}
		var realizedPnl float64
		if cfg.Binance.Testnet {
			qty := trade.Quantity
			entry := trade.EntryPrice
			if trade.Side == "LONG" {
				realizedPnl = (exitPrice - entry) * qty
			} else {
				realizedPnl = (entry - exitPrice) * qty
			}
		} else {
			realizedPnl, _ = bnClient.GetRecentRealizedPnl(ctx, symbol)
		}
		balanceAfter, _ := bnClient.GetUSDTBalance(ctx)
		if err := store.CloseTrade(ctx, trade.ID, exitPrice, realizedPnl, &balanceAfter, "MANUAL"); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "kayıt güncellenemedi: " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "symbol": symbol, "exit_price": exitPrice, "realized_pnl": realizedPnl})
	})
	r.GET("/api/open-positions-live", func(c *gin.Context) {
		list, err := store.OpenTrades(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if list == nil {
			list = []db.Trade{}
		}
		ctx := c.Request.Context()
		out := make([]gin.H, 0, len(list))
		for _, t := range list {
			notional := t.Quantity * t.EntryPrice
			lev := t.Leverage
			if lev < 1 {
				lev = 1
			}
			marginUSD := notional / float64(lev)
			if t.MarginUsdt != nil {
				marginUSD = *t.MarginUsdt
			}
			if t.NotionalUsdt != nil {
				notional = *t.NotionalUsdt
			}
			item := gin.H{
				"id": t.ID, "symbol": t.Symbol, "side": t.Side, "entry_price": t.EntryPrice,
				"quantity": t.Quantity, "stop_loss": t.StopLoss, "take_profit": t.TakeProfit,
				"opened_at": t.OpenedAt,
				"leverage":  lev, "margin_usdt": marginUSD, "notional_usdt": notional,
			}
			curPrice, errPrice := bnClient.GetPrice(ctx, t.Symbol)
			if errPrice == nil {
				item["current_price"] = curPrice
				qty := t.Quantity
				entry := t.EntryPrice
				var unrealized float64
				if t.Side == "LONG" {
					unrealized = (curPrice - entry) * qty
				} else {
					unrealized = (entry - curPrice) * qty
				}
				item["unrealized_pnl"] = unrealized
				if marginUSD > 0 {
					item["unrealized_pnl_pct"] = (unrealized / marginUSD) * 100
				} else {
					item["unrealized_pnl_pct"] = nil
				}
			} else {
				item["current_price"] = nil
				item["unrealized_pnl"] = nil
				item["unrealized_pnl_pct"] = nil
			}
			out = append(out, item)
		}
		c.JSON(http.StatusOK, out)
	})
	r.GET("/api/balance", func(c *gin.Context) {
		bal, avail, err := bnClient.GetUSDTBalanceDetails(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"balance": nil, "available_balance": nil, "balance_in_use": nil, "error": err.Error()})
			return
		}
		var inUse *float64
		if bal >= 0 && avail >= 0 && bal >= avail {
			used := bal - avail
			inUse = &used
		}
		c.JSON(http.StatusOK, gin.H{"balance": bal, "available_balance": avail, "balance_in_use": inUse, "futures": true})
	})
	r.GET("/api/my-ip", func(c *gin.Context) {
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get("https://api.ipify.org")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"ip": nil, "error": err.Error()})
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		ip := strings.TrimSpace(string(body))
		c.JSON(http.StatusOK, gin.H{"ip": ip, "hint": "Binance 'Restrict access to trusted IPs' listesine bu IP'yi ekle"})
	})
	r.GET("/api/settings", func(c *gin.Context) {
		ctx := c.Request.Context()
		overrides, err := store.GetRuntimeConfig(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		base := config.RuntimeConfigFromConfig(cfg)
		if base == nil {
			base = make(map[string]string)
		}
		out := make([]gin.H, 0, len(db.RuntimeConfigKeys))
		for _, key := range db.RuntimeConfigKeys {
			val := overrides[key]
			if val == "" {
				val = base[key]
			}
			out = append(out, gin.H{"key": key, "value": val})
		}
		c.JSON(http.StatusOK, gin.H{"settings": out})
	})
	r.PUT("/api/settings", func(c *gin.Context) {
		var body map[string]string
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "geçersiz JSON: " + err.Error()})
			return
		}
		ctx := c.Request.Context()
		toSave := make(map[string]string)
		for _, key := range db.RuntimeConfigKeys {
			if v, ok := body[key]; ok {
				toSave[key] = strings.TrimSpace(v)
			}
		}
		if err := store.SaveRuntimeConfig(ctx, toSave); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	r.GET("/api/summary", func(c *gin.Context) {
		ctx := c.Request.Context()
		// Toplam / 24h / bugün: hep DB'den (kapanışta Binance'ten alıp yazıyoruz, burada sadece topluyoruz)
		total, err := store.TotalRealizedPnl(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		pnl24h, _ := store.RealizedPnlLast24h(ctx)
		pnlToday, _ := store.RealizedPnlToday(ctx)
		openList, err := store.OpenTrades(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		openCount, closedCount, _ := store.TradeCounts(ctx)
		openPositions := make([]gin.H, 0, len(openList))
		for _, o := range openList {
			openPositions = append(openPositions, gin.H{
				"symbol": o.Symbol, "side": o.Side, "entry": o.EntryPrice,
				"quantity": o.Quantity, "opened_at": o.OpenedAt,
			})
		}
		c.JSON(http.StatusOK, gin.H{
			"total_pnl":      total,
			"pnl_last_24h":   pnl24h,
			"pnl_today":      pnlToday,
			"open_positions": openPositions,
			"open_count":     openCount,
			"closed_count":   closedCount,
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port
	log.Println("[api] dashboard dinleniyor:", addr)
	if err := r.Run(addr); err != nil {
		log.Fatal("[api] ", err)
	}
}
