package binance

import (
	"context"
	"encoding/json"
	"io"
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

// GetRecentCloseFillPrice kapanan pozisyonun gerçek fill fiyatını döner (userTrades: LONG kapanış=SELL, SHORT kapanış=BUY).
// since ile verilen süre içindeki ilgili tarafın en son işleminin fiyatı kullanılır.
func (c *Client) GetRecentCloseFillPrice(ctx context.Context, symbol string, positionSide string, since time.Time) (float64, error) {
	vals := url.Values{}
	vals.Set("symbol", symbol)
	vals.Set("startTime", strconv.FormatInt(since.UnixMilli(), 10))
	vals.Set("limit", "100")
	vals.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	vals.Set("recvWindow", "60000")

	sign, err := common.Hmac(c.api.SecretKey, vals.Encode())
	if err != nil {
		return 0, err
	}
	vals.Set("signature", *sign)

	rawURL := c.api.BaseURL + userTradesEndpoint + "?" + vals.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("X-MBX-APIKEY", c.api.APIKey)

	client := c.api.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		apiErr := new(common.APIError)
		_ = json.Unmarshal(data, apiErr)
		if apiErr.IsValid() {
			return 0, apiErr
		}
		return 0, &common.APIError{Code: int64(resp.StatusCode), Message: string(data)}
	}

	var list []userTrade
	if err := json.Unmarshal(data, &list); err != nil {
		return 0, err
	}
	// LONG kapanış = SELL (buyer=false), SHORT kapanış = BUY (buyer=true); en son (time en büyük) olanı al
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
