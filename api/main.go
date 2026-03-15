package main

import (
	"context"
	_ "embed"
	"log"
	"net/http"
	"os"
	"strconv"

	"binancebot/config"
	"binancebot/internal/binance"
	"binancebot/internal/db"

	"github.com/gin-gonic/gin"
)

//go:embed web/index.html
var indexHTML []byte

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

	// Açık pozisyonlar + Binance'tan anlık fiyat ve bekleyen PnL
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
			item := gin.H{
				"id": t.ID, "symbol": t.Symbol, "side": t.Side, "entry_price": t.EntryPrice,
				"quantity": t.Quantity, "stop_loss": t.StopLoss, "take_profit": t.TakeProfit,
				"opened_at": t.OpenedAt,
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
	log.Println("api dinleniyor:", addr)
	if err := r.Run(addr); err != nil {
		log.Fatal(err)
	}
}
