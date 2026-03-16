package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// depthResponse Binance Futures GET /fapi/v1/depth yanıtı (public).
type depthResponse struct {
	LastUpdateID int64      `json:"lastUpdateId"`
	Bids         [][]string `json:"bids"`
	Asks         [][]string `json:"asks"`
}

// GetOrderbookImbalance bid ve ask tarafının quote volume (fiyat*qty) toplamından
// imbalance = bidVol / (bidVol + askVol) döner. 1'e yakın = alım baskısı, 0'a yakın = satım baskısı.
// limit: 5, 10, 20, 50, 100.
func (c *Client) GetOrderbookImbalance(ctx context.Context, symbol string, limit int) (imbalance float64, err error) {
	if limit <= 0 {
		limit = 20
	}
	u := c.api.BaseURL + "/fapi/v1/depth?symbol=" + url.QueryEscape(symbol) + "&limit=" + fmt.Sprintf("%d", limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	client := c.api.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("depth %s: %s", resp.Status, string(body))
	}
	var d depthResponse
	if err := json.Unmarshal(body, &d); err != nil {
		return 0, err
	}
	bidVol := sumQuoteVolume(d.Bids)
	askVol := sumQuoteVolume(d.Asks)
	total := bidVol + askVol
	if total == 0 {
		return 0.5, nil // nötr
	}
	return bidVol / total, nil
}

func sumQuoteVolume(levels [][]string) float64 {
	var sum float64
	for _, row := range levels {
		if len(row) < 2 {
			continue
		}
		price := parseFloat(row[0])
		qty := parseFloat(row[1])
		sum += price * qty
	}
	return sum
}
