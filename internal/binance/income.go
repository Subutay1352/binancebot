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

const incomeEndpoint = "/fapi/v1/income"

type incomeRecord struct {
	Symbol     string `json:"symbol"`
	IncomeType string `json:"incomeType"`
	Income     string `json:"income"`
	Asset      string `json:"asset"`
	Info       string `json:"info"`
	Time       int64  `json:"time"`
	TranID     int64  `json:"tranId"`
	TradeID    string `json:"tradeId"`
}

// GetRealizedPnlSince verilen tarihten sonra sembole ait en son REALIZED_PNL kaydını döner (başlangıç senkronu için).
func (c *Client) GetRealizedPnlSince(ctx context.Context, symbol string, since time.Time) (float64, error) {
	startTime := since.UnixMilli()
	vals := url.Values{}
	vals.Set("symbol", symbol)
	vals.Set("incomeType", "REALIZED_PNL")
	vals.Set("startTime", strconv.FormatInt(startTime, 10))
	vals.Set("limit", "1000")
	vals.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	vals.Set("recvWindow", "60000")

	sign, err := common.Hmac(c.api.SecretKey, vals.Encode())
	if err != nil {
		return 0, err
	}
	vals.Set("signature", *sign)

	rawURL := c.api.BaseURL + incomeEndpoint + "?" + vals.Encode()
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

	var list []incomeRecord
	if err := json.Unmarshal(data, &list); err != nil {
		return 0, err
	}
	var latest *incomeRecord
	for i := range list {
		r := &list[i]
		if r.IncomeType != "REALIZED_PNL" || r.Symbol != symbol {
			continue
		}
		if latest == nil || r.Time > latest.Time {
			latest = r
		}
	}
	if latest == nil {
		return 0, nil
	}
	return parseFloat(latest.Income), nil
}

// GetRecentRealizedPnl son ~2 dakikada sembole ait en güncel REALIZED_PNL gelirini döner (gerçek kapanış PnL'i).
// Pozisyon market ile kapandığında fill fiyatı tetik fiyattan farklı olabilir; Binance bu değeri doğru tutar.
func (c *Client) GetRecentRealizedPnl(ctx context.Context, symbol string) (float64, error) {
	startTime := time.Now().Add(-2 * time.Minute).UnixMilli()
	vals := url.Values{}
	vals.Set("symbol", symbol)
	vals.Set("incomeType", "REALIZED_PNL")
	vals.Set("startTime", strconv.FormatInt(startTime, 10))
	vals.Set("limit", "20")
	vals.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	vals.Set("recvWindow", "60000")

	sign, err := common.Hmac(c.api.SecretKey, vals.Encode())
	if err != nil {
		return 0, err
	}
	vals.Set("signature", *sign)

	rawURL := c.api.BaseURL + incomeEndpoint + "?" + vals.Encode()
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

	var list []incomeRecord
	if err := json.Unmarshal(data, &list); err != nil {
		return 0, err
	}
	// En güncel (son) kayıt = az önce kapanan pozisyonun PnL'i (liste genelde en yeniden eskiye)
	if len(list) == 0 {
		return 0, nil
	}
	// Binance en yeniyi önce döndürebilir; time'a göre en büyük olanı al
	var latest *incomeRecord
	for i := range list {
		r := &list[i]
		if r.IncomeType != "REALIZED_PNL" || r.Symbol != symbol {
			continue
		}
		if latest == nil || r.Time > latest.Time {
			latest = r
		}
	}
	if latest == nil {
		return 0, nil
	}
	return parseFloat(latest.Income), nil
}

// fetchIncome REALIZED_PNL gelir kayıtlarını çeker; startTimeMs verilirse o tarihten sonrakiler (limit 1000).
func (c *Client) fetchIncome(ctx context.Context, startTimeMs *int64) ([]incomeRecord, error) {
	vals := url.Values{}
	vals.Set("incomeType", "REALIZED_PNL")
	if startTimeMs != nil {
		vals.Set("startTime", strconv.FormatInt(*startTimeMs, 10))
	}
	vals.Set("limit", "1000")
	vals.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	vals.Set("recvWindow", "60000")

	sign, err := common.Hmac(c.api.SecretKey, vals.Encode())
	if err != nil {
		return nil, err
	}
	vals.Set("signature", *sign)

	rawURL := c.api.BaseURL + incomeEndpoint + "?" + vals.Encode()
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

	var list []incomeRecord
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// GetRealizedPnlSumFromBinance Binance Income API'den REALIZED_PNL toplamını döner (tek kaynak = Binance).
// startTimeMs nil ise son 1000 kayıt toplanır (toplam PnL); set edilirse o tarihten sonrakiler (örn. son 24h, bugün).
func (c *Client) GetRealizedPnlSumFromBinance(ctx context.Context, startTimeMs *int64) (float64, error) {
	list, err := c.fetchIncome(ctx, startTimeMs)
	if err != nil {
		return 0, err
	}
	var sum float64
	for i := range list {
		if list[i].IncomeType != "REALIZED_PNL" {
			continue
		}
		sum += parseFloat(list[i].Income)
	}
	return sum, nil
}
