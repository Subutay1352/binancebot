package binance

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/adshao/go-binance/v2/common"
)

const leverageEndpoint = "/fapi/v1/leverage"

// SetLeverage sembol için kaldıracı ayarlar (1–125). Açılamazsa hata döner; çağıran diğer sembole geçebilir.
func (c *Client) SetLeverage(ctx context.Context, symbol string, leverage int) error {
	if leverage < 1 {
		leverage = 1
	}
	if leverage > 125 {
		leverage = 125
	}
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	form := url.Values{}
	form.Set("symbol", symbol)
	form.Set("leverage", strconv.Itoa(leverage))
	form.Set("timestamp", ts)
	form.Set("recvWindow", "60000")
	bodyStr := form.Encode()

	sign, err := common.Hmac(c.api.SecretKey, bodyStr)
	if err != nil {
		return err
	}
	form.Set("signature", *sign)
	bodyStr = form.Encode()

	rawURL := c.api.BaseURL + leverageEndpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader([]byte(bodyStr)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-MBX-APIKEY", c.api.APIKey)

	client := c.api.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		apiErr := new(common.APIError)
		_ = json.Unmarshal(data, apiErr)
		if apiErr.IsValid() {
			return apiErr
		}
		return &common.APIError{Code: int64(resp.StatusCode), Message: string(data)}
	}
	return nil
}
