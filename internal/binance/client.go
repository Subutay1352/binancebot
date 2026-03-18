package binance

import (
	"context"
	"fmt"
	"log"
	"strings"

	"binancebot/config"

	"github.com/adshao/go-binance/v2/futures"
)

// Client Binance USDT-M Futures API sarmalayıcısı.
type Client struct {
	api *futures.Client
}

// NewClient config ile client oluşturur. Testnet için BaseURL testnet adresine ayarlanır.
func NewClient(cfg *config.Config) *Client {
	api := futures.NewClient(cfg.Binance.APIKey, cfg.Binance.SecretKey)
	if cfg.Binance.Testnet {
		api.BaseURL = futures.BaseApiTestnetUrl
	}
	return &Client{api: api}
}

// GetUSDTBalance Futures cüzdanındaki USDT wallet balance döner.
func (c *Client) GetUSDTBalance(ctx context.Context) (float64, error) {
	bal, _, err := c.GetUSDTBalanceDetails(ctx)
	return bal, err
}

// GetUSDTBalanceDetails wallet balance ve kullanılabilir bakiye (marj ayrıldıktan sonra) döner.
// Açık pozisyonlarda availableBalance düşer, balance (toplam) realizasyon olana kadar aynı kalabilir.
func (c *Client) GetUSDTBalanceDetails(ctx context.Context) (balance, availableBalance float64, err error) {
	balances, err := c.api.NewGetBalanceService().Do(ctx)
	if err != nil {
		return 0, 0, err
	}
	for _, b := range balances {
		if b.Asset == "USDT" {
			return parseFloat(b.Balance), parseFloat(b.AvailableBalance), nil
		}
	}
	return 0, 0, nil
}

// GetPrice sembolün anlık fiyatını döner.
func (c *Client) GetPrice(ctx context.Context, symbol string) (float64, error) {
	prices, err := c.api.NewListPricesService().Symbol(symbol).Do(ctx)
	if err != nil {
		return 0, err
	}
	if len(prices) == 0 {
		return 0, fmt.Errorf("fiyat yok: %s", symbol)
	}
	return parseFloat(prices[0].Price), nil
}

// OpenLong market long açar. quantityStr step size hassasiyetinde formatlanmış olmalı (-1111 önlemek için).
func (c *Client) OpenLong(ctx context.Context, symbol string, quantityStr string) (*futures.CreateOrderResponse, error) {
	log.Printf("[binance] OPEN_LONG | symbol=%s quantity=%s", symbol, quantityStr)
	resp, err := c.api.NewCreateOrderService().
		Symbol(symbol).
		Side(futures.SideTypeBuy).
		Type(futures.OrderTypeMarket).
		Quantity(quantityStr).
		Do(ctx)
	if err != nil {
		log.Printf("[binance] OPEN_LONG hata | symbol=%s: %v", symbol, err)
		return nil, err
	}
	log.Printf("[binance] OPEN_LONG OK | symbol=%s order_id=%d avg_price=%s", symbol, resp.OrderID, resp.AvgPrice)
	return resp, nil
}

// OpenShort market short açar. quantityStr step size hassasiyetinde formatlanmış olmalı (-1111 önlemek için).
func (c *Client) OpenShort(ctx context.Context, symbol string, quantityStr string) (*futures.CreateOrderResponse, error) {
	log.Printf("[binance] OPEN_SHORT | symbol=%s quantity=%s", symbol, quantityStr)
	resp, err := c.api.NewCreateOrderService().
		Symbol(symbol).
		Side(futures.SideTypeSell).
		Type(futures.OrderTypeMarket).
		Quantity(quantityStr).
		Do(ctx)
	if err != nil {
		log.Printf("[binance] OPEN_SHORT hata | symbol=%s: %v", symbol, err)
		return nil, err
	}
	log.Printf("[binance] OPEN_SHORT OK | symbol=%s order_id=%d avg_price=%s", symbol, resp.OrderID, resp.AvgPrice)
	return resp, nil
}

// PlaceStopLoss Algo Order API ile STOP_MARKET (closePosition=true; quantity kullanılmaz).
func (c *Client) PlaceStopLoss(ctx context.Context, symbol string, side futures.SideType, quantity, stopPrice string) (*algoOrderResponse, error) {
	return c.PlaceStopLossAlgo(ctx, symbol, side, stopPrice)
}

// PlaceTakeProfit Algo Order API ile TAKE_PROFIT_MARKET (closePosition=true; quantity kullanılmaz).
func (c *Client) PlaceTakeProfit(ctx context.Context, symbol string, side futures.SideType, quantity, stopPrice string) (*algoOrderResponse, error) {
	return c.PlaceTakeProfitAlgo(ctx, symbol, side, stopPrice)
}

// ClosePositionMarket pozisyonu ters yönde market emirle kapatır. Dönen orderId ile fill VWAP alınabilir.
func (c *Client) ClosePositionMarket(ctx context.Context, symbol string, side string, quantityStr string) (orderID int64, err error) {
	closeSide := futures.SideTypeSell
	if side == "SHORT" {
		closeSide = futures.SideTypeBuy
	}
	log.Printf("[binance] CLOSE_POSITION_MARKET | symbol=%s side=%s quantity=%s", symbol, closeSide, quantityStr)
	resp, err := c.api.NewCreateOrderService().
		Symbol(symbol).
		Side(closeSide).
		Type(futures.OrderTypeMarket).
		Quantity(quantityStr).
		ReduceOnly(true).
		Do(ctx)
	if err != nil {
		log.Printf("[binance] CLOSE_POSITION_MARKET hata | symbol=%s: %v", symbol, err)
		return 0, err
	}
	oid := resp.OrderID
	log.Printf("[binance] CLOSE_POSITION_MARKET OK | symbol=%s orderId=%d", symbol, oid)
	return oid, nil
}

// GetPosition sembol için açık pozisyon; yoksa nil.
func (c *Client) GetPosition(ctx context.Context, symbol string) (*futures.PositionRisk, error) {
	positions, err := c.api.NewGetPositionRiskService().Do(ctx)
	if err != nil {
		return nil, err
	}
	for i := range positions {
		if positions[i].Symbol == symbol && parseFloat(positions[i].PositionAmt) != 0 {
			return positions[i], nil
		}
	}
	return nil, nil
}

// GetPositionEntryPrice borsadaki pozisyonun gerçek ortalama giriş fiyatını döner (Binance PositionRisk.entryPrice).
func (c *Client) GetPositionEntryPrice(ctx context.Context, symbol string) (float64, error) {
	pos, err := c.GetPosition(ctx, symbol)
	if err != nil || pos == nil {
		return 0, err
	}
	return parseFloat(pos.EntryPrice), nil
}

// GetOpenPositions pozisyonu olan (positionAmt != 0) tüm sembolleri döner.
func (c *Client) GetOpenPositions(ctx context.Context) ([]*futures.PositionRisk, error) {
	positions, err := c.api.NewGetPositionRiskService().Do(ctx)
	if err != nil {
		return nil, err
	}
	var out []*futures.PositionRisk
	for _, p := range positions {
		if parseFloat(p.PositionAmt) != 0 {
			out = append(out, p)
		}
	}
	return out, nil
}

// CloseAllOpenPositions tüm açık pozisyonları market ile kapatır.
func (c *Client) CloseAllOpenPositions(ctx context.Context) error {
	positions, err := c.GetOpenPositions(ctx)
	if err != nil {
		return err
	}
	for _, p := range positions {
		amt := parseFloat(p.PositionAmt)
		if amt == 0 {
			continue
		}
		side := "LONG"
		qtyStr := strings.TrimPrefix(p.PositionAmt, "-")
		if amt < 0 {
			side = "SHORT"
		}
		if _, err := c.ClosePositionMarket(ctx, p.Symbol, side, qtyStr); err != nil {
			log.Printf("[binance] pozisyon kapatma hata | symbol=%s: %v", p.Symbol, err)
			continue
		}
		log.Printf("[binance] pozisyon kapatıldı | symbol=%s side=%s qty=%s", p.Symbol, side, qtyStr)
	}
	return nil
}

// Klines mum verisi (interval örn: "5m", limit örn: 200).
func (c *Client) Klines(ctx context.Context, symbol, interval string, limit int) ([]*futures.Kline, error) {
	return c.api.NewKlinesService().Symbol(symbol).Interval(interval).Limit(limit).Do(ctx)
}

// Get24hQuoteVolumeUSD sembolün son 24 saatteki işlem hacmini USDT (quote) cinsinden döner.
func (c *Client) Get24hQuoteVolumeUSD(ctx context.Context, symbol string) (float64, error) {
	stats, err := c.api.NewListPriceChangeStatsService().Symbol(symbol).Do(ctx)
	if err != nil || len(stats) == 0 {
		return 0, err
	}
	return parseFloat(stats[0].QuoteVolume), nil
}

// ExchangeInfo sembol bilgisi (lot size vb.).
func (c *Client) ExchangeInfo(ctx context.Context, symbol string) (*futures.Symbol, error) {
	info, err := c.api.NewExchangeInfoService().Do(ctx)
	if err != nil {
		return nil, err
	}
	for i := range info.Symbols {
		if info.Symbols[i].Symbol == symbol {
			return &info.Symbols[i], nil
		}
	}
	return nil, fmt.Errorf("sembol yok: %s", symbol)
}

// AllFuturesSymbols işlemde olan tüm USDT vadeli çiftlerini döner (Binance Futures exchange info).
func (c *Client) AllFuturesSymbols(ctx context.Context) ([]string, error) {
	info, err := c.api.NewExchangeInfoService().Do(ctx)
	if err != nil {
		return nil, err
	}
	var list []string
	for _, s := range info.Symbols {
		if s.Status != "TRADING" {
			continue
		}
		if s.QuoteAsset != "USDT" {
			continue
		}
		list = append(list, s.Symbol)
	}
	return list, nil
}
