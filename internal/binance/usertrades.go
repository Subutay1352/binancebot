package binance

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/adshao/go-binance/v2/common"
)

const userTradesEndpoint = "/fapi/v1/userTrades"

type userTrade struct {
	Symbol     string `json:"symbol"`
	ID         int64  `json:"id"`
	OrderID    int64  `json:"orderId"`
	Price      string `json:"price"`
	Qty        string `json:"qty"`
	Commission string `json:"commission"`
	Time       int64  `json:"time"`
	Buyer      bool   `json:"buyer"`
}

func (c *Client) getUserTrades(ctx context.Context, symbol string, startTimeMs int64, orderID int64, limit int) ([]userTrade, error) {
	if limit <= 0 {
		limit = 500
	}
	if limit > 1000 {
		limit = 1000
	}
	vals := url.Values{}
	vals.Set("symbol", symbol)
	if startTimeMs > 0 {
		vals.Set("startTime", strconv.FormatInt(startTimeMs, 10))
	}
	if orderID > 0 {
		vals.Set("orderId", strconv.FormatInt(orderID, 10))
	}
	vals.Set("limit", strconv.Itoa(limit))
	vals.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	vals.Set("recvWindow", "60000")

	sign, err := common.Hmac(c.api.SecretKey, vals.Encode())
	if err != nil {
		return nil, err
	}
	vals.Set("signature", *sign)

	rawURL := c.api.BaseURL + userTradesEndpoint + "?" + vals.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-MBX-APIKEY", c.api.APIKey)

	client := c.api.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		apiErr := new(common.APIError)
		_ = json.Unmarshal(data, apiErr)
		if apiErr.IsValid() {
			return nil, apiErr
		}
		return nil, &common.APIError{Code: int64(resp.StatusCode), Message: string(data)}
	}

	var list []userTrade
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// GetFillVWAPByOrderID bir emrin tüm fill'lerinin hacim ağırlıklı ortalama fiyatı (mobil "ortalama kapanış" ile uyumlu).
func (c *Client) GetFillVWAPByOrderID(ctx context.Context, symbol string, orderID int64) (vwap float64, totalQty float64, err error) {
	if orderID <= 0 {
		return 0, 0, nil
	}
	list, err := c.getUserTrades(ctx, symbol, 0, orderID, 500)
	if err != nil {
		return 0, 0, err
	}
	var sumPQ, sumQ float64
	for i := range list {
		if list[i].Symbol != symbol {
			continue
		}
		p := parseFloat(list[i].Price)
		q := parseFloat(list[i].Qty)
		sumPQ += p * q
		sumQ += q
	}
	if sumQ <= 0 {
		return 0, 0, nil
	}
	return sumPQ / sumQ, sumQ, nil
}

// WaitExitVWAPAfterMarketClose orderId için fill'lerin yazılmasını bekler; VWAP döner.
func (c *Client) WaitExitVWAPAfterMarketClose(ctx context.Context, symbol string, orderID int64) (vwap float64, totalQty float64) {
	if orderID <= 0 {
		return 0, 0
	}
	for attempt := 0; attempt < 12; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return 0, 0
			case <-time.After(400 * time.Millisecond):
			}
		}
		v, q, err := c.GetFillVWAPByOrderID(ctx, symbol, orderID)
		if err == nil && v > 0 && q > 0 {
			return v, q
		}
	}
	return 0, 0
}

// GetExitPriceVWAPAfterClose SL/TP veya dış kapanış: son penceredeki kapanış yönlü emirlerden, miktarı DB'deki pozisyona en yakın olanı seçer (VWAP).
// LONG kapanış = SELL (buyer=false), SHORT kapanış = BUY (buyer=true).
func (c *Client) GetExitPriceVWAPAfterClose(ctx context.Context, symbol string, positionSide string, expectedQty float64, since time.Time) (float64, error) {
	list, err := c.getUserTrades(ctx, symbol, since.UnixMilli(), 0, 1000)
	if err != nil || len(list) == 0 {
		return 0, err
	}
	wantBuyer := positionSide == "SHORT"
	type agg struct {
		sumPQ, sumQ float64
		maxTime     int64
	}
	byOrder := make(map[int64]*agg)
	for i := range list {
		t := &list[i]
		if t.Symbol != symbol || t.Buyer != wantBuyer {
			continue
		}
		p := parseFloat(t.Price)
		q := parseFloat(t.Qty)
		if byOrder[t.OrderID] == nil {
			byOrder[t.OrderID] = &agg{}
		}
		a := byOrder[t.OrderID]
		a.sumPQ += p * q
		a.sumQ += q
		if t.Time > a.maxTime {
			a.maxTime = t.Time
		}
	}
	tol := math.Max(expectedQty*0.003, 1e-10)
	if expectedQty < 1e-12 {
		tol = 1e-8
	}
	var bestVWAP float64
	var bestTime int64
	for _, a := range byOrder {
		if a.sumQ <= 0 {
			continue
		}
		if math.Abs(a.sumQ-expectedQty) <= tol && a.maxTime > bestTime {
			bestTime = a.maxTime
			bestVWAP = a.sumPQ / a.sumQ
		}
	}
	if bestTime > 0 {
		return bestVWAP, nil
	}
	// Miktara en yakın emir (en güncel)
	var bestOrder int64
	bestDiff := math.MaxFloat64
	bestTime = 0
	for oid, a := range byOrder {
		if a.sumQ <= 0 {
			continue
		}
		diff := math.Abs(a.sumQ - expectedQty)
		if diff < bestDiff || (diff == bestDiff && a.maxTime > bestTime) {
			bestDiff = diff
			bestTime = a.maxTime
			bestOrder = oid
			bestVWAP = a.sumPQ / a.sumQ
		}
	}
	if bestOrder != 0 && expectedQty > 0 && bestDiff <= expectedQty*0.15 {
		return bestVWAP, nil
	}
	return 0, nil
}

// GetRecentCloseFillPrice en son kapanış yönündeki tek fill (yedek; asıl VWAP için GetExitPriceVWAPAfterClose).
func (c *Client) GetRecentCloseFillPrice(ctx context.Context, symbol string, positionSide string, since time.Time) (float64, error) {
	list, err := c.getUserTrades(ctx, symbol, since.UnixMilli(), 0, 100)
	if err != nil {
		return 0, err
	}
	wantBuyer := positionSide == "SHORT"
	var latest *userTrade
	for i := range list {
		t := &list[i]
		if t.Symbol != symbol || t.Buyer != wantBuyer {
			continue
		}
		if latest == nil || t.Time > latest.Time {
			latest = t
		}
	}
	if latest == nil {
		return 0, nil
	}
	return parseFloat(latest.Price), nil
}
