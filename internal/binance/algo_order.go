package binance

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/adshao/go-binance/v2/common"
	"github.com/adshao/go-binance/v2/futures"
)

const (
	algoOrderEndpoint     = "/fapi/v1/algoOrder"
	openAlgoOrdersEndpoint = "/fapi/v1/openAlgoOrders"
)

// algoOrderResponse Binance Algo Order yanıtı (algoId log için).
type algoOrderResponse struct {
	AlgoId int64 `json:"algoId"`
}

// placeAlgoOrder CONDITIONAL algo order gönderir (STOP_MARKET veya TAKE_PROFIT_MARKET).
// closePosition=true ise quantity gönderilmez; tetiklenince tüm pozisyon kapanır.
func (c *Client) placeAlgoOrder(ctx context.Context, symbol string, side futures.SideType, orderType, triggerPrice string, closePosition bool) (*algoOrderResponse, error) {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	form := url.Values{}
	form.Set("algoType", "CONDITIONAL")
	form.Set("symbol", symbol)
	form.Set("side", string(side))
	form.Set("type", orderType)
	form.Set("triggerPrice", triggerPrice)
	form.Set("timestamp", ts)
	form.Set("recvWindow", recvWindow)
	if closePosition {
		form.Set("closePosition", "true")
	}
	bodyStr := form.Encode()

	sign, err := common.Hmac(c.api.SecretKey, bodyStr)
	if err != nil {
		return nil, err
	}
	form.Set("signature", *sign)
	bodyStr = form.Encode()

	rawURL := c.api.BaseURL + algoOrderEndpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader([]byte(bodyStr)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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

	var out algoOrderResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PlaceStopLossAlgo Algo Order API ile STOP_MARKET (closePosition=true).
func (c *Client) PlaceStopLossAlgo(ctx context.Context, symbol string, side futures.SideType, triggerPrice string) (*algoOrderResponse, error) {
	log.Printf("[binance] PLACE_SL_ALGO | symbol=%s side=%s trigger_price=%s", symbol, side, triggerPrice)
	resp, err := c.placeAlgoOrder(ctx, symbol, side, "STOP_MARKET", triggerPrice, true)
	if err != nil {
		log.Printf("[binance] PLACE_SL_ALGO hata | symbol=%s: %v", symbol, err)
		return nil, err
	}
	log.Printf("[binance] PLACE_SL_ALGO OK | symbol=%s algo_id=%d", symbol, resp.AlgoId)
	return resp, nil
}

// PlaceTakeProfitAlgo Algo Order API ile TAKE_PROFIT_MARKET (closePosition=true).
func (c *Client) PlaceTakeProfitAlgo(ctx context.Context, symbol string, side futures.SideType, triggerPrice string) (*algoOrderResponse, error) {
	log.Printf("[binance] PLACE_TP_ALGO | symbol=%s side=%s trigger_price=%s", symbol, side, triggerPrice)
	resp, err := c.placeAlgoOrder(ctx, symbol, side, "TAKE_PROFIT_MARKET", triggerPrice, true)
	if err != nil {
		log.Printf("[binance] PLACE_TP_ALGO hata | symbol=%s: %v", symbol, err)
		return nil, err
	}
	log.Printf("[binance] PLACE_TP_ALGO OK | symbol=%s algo_id=%d", symbol, resp.AlgoId)
	return resp, nil
}

// openAlgoOrderItem açık algo emri (liste yanıtından).
type openAlgoOrderItem struct {
	AlgoId   int64  `json:"algoId"`
	Symbol   string `json:"symbol"`
	AlgoStatus string `json:"algoStatus"`
}

const recvWindow = "60000"

// getOpenAlgoOrders tüm açık algo emirlerini döner (sembol filtresi yok).
func (c *Client) getOpenAlgoOrders(ctx context.Context) ([]openAlgoOrderItem, error) {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	query := url.Values{}
	query.Set("timestamp", ts)
	query.Set("recvWindow", recvWindow)
	payload := query.Encode()
	sign, err := common.Hmac(c.api.SecretKey, payload)
	if err != nil {
		return nil, err
	}
	query.Set("signature", *sign)
	rawURL := c.api.BaseURL + openAlgoOrdersEndpoint + "?" + query.Encode()
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
	var list []openAlgoOrderItem
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// cancelAlgoOrder tek algo emrini iptal eder.
func (c *Client) cancelAlgoOrder(ctx context.Context, algoId int64) error {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	form := url.Values{}
	form.Set("algoId", strconv.FormatInt(algoId, 10))
	form.Set("timestamp", ts)
	form.Set("recvWindow", recvWindow)
	bodyStr := form.Encode()
	sign, err := common.Hmac(c.api.SecretKey, bodyStr)
	if err != nil {
		return err
	}
	form.Set("signature", *sign)
	bodyStr = form.Encode()
	rawURL := c.api.BaseURL + algoOrderEndpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, rawURL, bytes.NewReader([]byte(bodyStr)))
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
	data, _ := io.ReadAll(resp.Body)
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

// CancelAllOpenAlgoOrders tüm açık SL/TP (algo) emirlerini iptal eder.
func (c *Client) CancelAllOpenAlgoOrders(ctx context.Context) error {
	list, err := c.getOpenAlgoOrders(ctx)
	if err != nil {
		return err
	}
	for _, o := range list {
		if o.AlgoStatus != "NEW" {
			continue
		}
		if err := c.cancelAlgoOrder(ctx, o.AlgoId); err != nil {
			log.Printf("[binance] algo iptal hata | algoId=%d symbol=%s: %v", o.AlgoId, o.Symbol, err)
			continue
		}
		log.Printf("[binance] algo iptal OK | algoId=%d symbol=%s", o.AlgoId, o.Symbol)
	}
	return nil
}
