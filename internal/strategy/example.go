package strategy

import (
	"context"
	"math"
	"strconv"

	"binancebot/config"
	"binancebot/internal/binance"

	"github.com/adshao/go-binance/v2/futures"
)

// Örnek strateji: 30m RSI + hacim kuralları.
// Kural örneği: 30m RSI 10’dan küçükse VE (hacim X USD üstü VEYA son mumdan 3x) → Long.
// Short: RSI 90’dan büyükse + hacim koşulu → Short. Kuralları sonradan config’den değiştirirsin.
type Example struct {
	client *binance.Client
	cfg    config.StrategyConfig
}

// NewExample örnek strateji. Kurallar cfg ile gelir.
func NewExample(client *binance.Client, cfg config.StrategyConfig) *Example {
	return &Example{client: client, cfg: cfg}
}

// Decide 30m RSI ve hacim koşullarına göre sinyal verir.
func (e *Example) Decide(ctx context.Context, symbol string) (Signal, error) {
	klines, err := e.client.Klines(ctx, symbol, e.cfg.Interval30m, e.cfg.RSIPeriod+10)
	if err != nil || len(klines) < e.cfg.RSIPeriod+1 {
		return Hold, err
	}

	closes := klineCloses(klines)
	quoteVolumes := klineQuoteVolumes(klines)

	rsiVal := rsi(closes, e.cfg.RSIPeriod)
	if math.IsNaN(rsiVal) {
		return Hold, nil
	}

	n := len(quoteVolumes)
	currentVol := quoteVolumes[n-1]
	prevVol := 0.0
	if n >= 2 {
		prevVol = quoteVolumes[n-2]
	}
	volumeOK := currentVol >= e.cfg.MinVolumeUSD || (prevVol > 0 && currentVol >= prevVol*e.cfg.VolumeRatioVsPrev)

	if rsiVal < e.cfg.RSIThresholdLow && volumeOK {
		return Long, nil
	}
	if rsiVal > e.cfg.RSIThresholdHigh && volumeOK {
		return Short, nil
	}
	return Hold, nil
}

func klineCloses(klines []*futures.Kline) []float64 {
	out := make([]float64, 0, len(klines))
	for _, k := range klines {
		out = append(out, mustFloat(k.Close))
	}
	return out
}

func klineQuoteVolumes(klines []*futures.Kline) []float64 {
	out := make([]float64, 0, len(klines))
	for _, k := range klines {
		out = append(out, mustFloat(k.QuoteAssetVolume))
	}
	return out
}

func mustFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func rsi(closes []float64, period int) float64 {
	if len(closes) < period+1 {
		return math.NaN()
	}
	gains := 0.0
	losses := 0.0
	for i := len(closes) - period; i < len(closes); i++ {
		ch := closes[i] - closes[i-1]
		if ch > 0 {
			gains += ch
		} else {
			losses -= ch
		}
	}
	avgGain := gains / float64(period)
	avgLoss := losses / float64(period)
	if avgLoss == 0 {
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - 100/(1+rs)
}
