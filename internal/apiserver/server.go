package apiserver

import (
	_ "embed"
	"log"
	"net/http"
	"os"
	"strconv"

	"binancebot/internal/binance"
	"binancebot/internal/db"

	"github.com/gin-gonic/gin"
)

//go:embed static/index.html
var indexHTML []byte

// Run dashboard API'yi PORT'ta başlatır ve bloke eder. Bot ile aynı process'te çalışacaksa goroutine'de çağır: go apiserver.Run(store, bnClient)
func Run(store *db.Store, bnClient *binance.Client) {
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
				"leverage": lev, "margin_usdt": marginUSD, "notional_usdt": notional,
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
			} else {
				item["current_price"] = nil
				item["unrealized_pnl"] = nil
			}
			out = append(out, item)
		}
		c.JSON(http.StatusOK, out)
	})
	r.GET("/api/balance", func(c *gin.Context) {
		bal, avail, err := bnClient.GetUSDTBalanceDetails(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"balance": nil, "available_balance": nil, "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"balance": bal, "available_balance": avail})
	})
	r.GET("/api/summary", func(c *gin.Context) {
		total, err := store.TotalRealizedPnl(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		openList, err := store.OpenTrades(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		openCount, closedCount, _ := store.TradeCounts(c.Request.Context())
		openPositions := make([]gin.H, 0, len(openList))
		for _, o := range openList {
			openPositions = append(openPositions, gin.H{
				"symbol": o.Symbol, "side": o.Side, "entry": o.EntryPrice,
				"quantity": o.Quantity, "opened_at": o.OpenedAt,
			})
		}
		c.JSON(http.StatusOK, gin.H{
			"total_pnl":      total,
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
