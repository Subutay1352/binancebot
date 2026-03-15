package strategy

import (
	"context"
	"math"
	"strconv"

	"binancebot/config"
	"binancebot/internal/binance"

	"github.com/adshao/go-binance/v2/futures"
)

// Örnek strateji: Wilder RSI + ortalama hacim + MA trend + çok zaman dilimi RSI.
type Example struct {
	client *binance.Client
	cfg    config.StrategyConfig
}

// NewExample örnek strateji. Kurallar cfg ile gelir.
func NewExample(client *binance.Client, cfg config.StrategyConfig) *Example {
	return &Example{client: client, cfg: cfg}
}

// Decide Wilder RSI, hacim (ortalama dahil), MA trend ve üst TF RSI ile sinyal verir.
func (e *Example) Decide(ctx context.Context, symbol string) (Signal, error) {
	// Ana TF (30m) – yeterli mum: RSI + MA + hacim ortalaması
	needCandles := e.cfg.RSIPeriod + 25
	if e.cfg.MAPeriod > 0 && e.cfg.MAPeriod > needCandles {
		needCandles = e.cfg.MAPeriod + 5
	}
	if e.cfg.VolumeAvgPeriod > 0 && e.cfg.VolumeAvgPeriod > needCandles {
		needCandles = e.cfg.VolumeAvgPeriod + 5
	}
	klines, err := e.client.Klines(ctx, symbol, e.cfg.Interval30m, needCandles)
	if err != nil || len(klines) < e.cfg.RSIPeriod+2 {
		return Hold, err
	}

	closes := klineCloses(klines)
	quoteVolumes := klineQuoteVolumes(klines)

	// 1) Wilder RSI (son değer)
	rsiVal := rsiWilder(closes, e.cfg.RSIPeriod)
	if math.IsNaN(rsiVal) {
		return Hold, nil
	}

	// 2) Hacim: mevcut kurallar + ortalama hacim filtresi
	n := len(quoteVolumes)
	currentVol := quoteVolumes[n-1]
	prevVol := 0.0
	if n >= 2 {
		prevVol = quoteVolumes[n-2]
	}
	volumeOK := currentVol >= e.cfg.MinVolumeUSD || (prevVol > 0 && currentVol >= prevVol*e.cfg.VolumeRatioVsPrev)
	if e.cfg.VolumeAvgPeriod > 0 && e.cfg.VolumeMinRatioToAvg > 0 {
		avgVol := avgVolume(quoteVolumes, n-1, e.cfg.VolumeAvgPeriod)
		if avgVol > 0 && currentVol < avgVol*e.cfg.VolumeMinRatioToAvg {
			volumeOK = false
		}
	}

	// 3) Trend: Long sadece fiyat MA üstünde, Short sadece MA altında
	priceAboveMA := true
	priceBelowMA := true
	if e.cfg.MAPeriod > 0 && len(closes) >= e.cfg.MAPeriod {
		ma := sma(closes, len(closes)-1, e.cfg.MAPeriod)
		if !math.IsNaN(ma) {
			lastClose := closes[len(closes)-1]
			priceAboveMA = lastClose > ma
			priceBelowMA = lastClose < ma
		}
	}

	// 4) Üst zaman dilimi RSI (örn. 1h)
	higherTFOKLong := true
	higherTFOKShort := true
	if e.cfg.HigherTFInterval != "" && e.cfg.HigherTFRSIPeriod > 0 {
		htfKlines, errH := e.client.Klines(ctx, symbol, e.cfg.HigherTFInterval, e.cfg.HigherTFRSIPeriod+15)
		if errH == nil && len(htfKlines) >= e.cfg.HigherTFRSIPeriod+2 {
			htfCloses := klineCloses(htfKlines)
			htfRSI := rsiWilder(htfCloses, e.cfg.HigherTFRSIPeriod)
			if !math.IsNaN(htfRSI) {
				higherTFOKLong = htfRSI < e.cfg.HigherTFRSILongMax
				higherTFOKShort = htfRSI > e.cfg.HigherTFRSIShortMin
			}
		}
	}

	if rsiVal < e.cfg.RSIThresholdLow && volumeOK && priceAboveMA && higherTFOKLong {
		return Long, nil
	}
	if rsiVal > e.cfg.RSIThresholdHigh && volumeOK && priceBelowMA && higherTFOKShort {
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

// rsiWilder Wilder smoothing ile RSI (son mum için). closes en az period+2 olmalı.
func rsiWilder(closes []float64, period int) float64 {
	if len(closes) < period+2 {
		return math.NaN()
	}
	var avgGain, avgLoss float64
	for i := 1; i <= period; i++ {
		ch := closes[i] - closes[i-1]
		if ch > 0 {
			avgGain += ch
		} else {
			avgLoss -= ch
		}
	}
	avgGain /= float64(period)
	avgLoss /= float64(period)
	for i := period + 1; i < len(closes); i++ {
		ch := closes[i] - closes[i-1]
		gain, loss := 0.0, 0.0
		if ch > 0 {
			gain = ch
		} else {
			loss = -ch
		}
		avgGain = (avgGain*float64(period-1) + gain) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + loss) / float64(period)
	}
	if avgLoss == 0 {
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - 100/(1+rs)
}

func avgVolume(vols []float64, endIdx, period int) float64 {
	start := endIdx - period + 1
	if start < 0 {
		start = 0
	}
	if start >= endIdx {
		return 0
	}
	sum := 0.0
	for i := start; i <= endIdx; i++ {
		sum += vols[i]
	}
	return sum / float64(endIdx-start+1)
}

func sma(closes []float64, endIdx, period int) float64 {
	start := endIdx - period + 1
	if start < 0 {
		return math.NaN()
	}
	sum := 0.0
	for i := start; i <= endIdx; i++ {
		sum += closes[i]
	}
	return sum / float64(period)
}
