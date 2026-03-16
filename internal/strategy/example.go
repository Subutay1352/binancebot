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

// SetConfig runtime’da strateji ayarlarını günceller (UI’dan değişince bot bunu çağırır).
func (e *Example) SetConfig(cfg config.StrategyConfig) {
	e.cfg = cfg
}

// Decide Trend (EMA) + RSI pullback + hacim + orderbook + volatility ile sinyal verir.
func (e *Example) Decide(ctx context.Context, symbol string) (Signal, error) {
	needCandles := e.cfg.RSIPeriod + 25
	if e.cfg.VolumeAvgPeriod > 0 && e.cfg.VolumeAvgPeriod > needCandles {
		needCandles = e.cfg.VolumeAvgPeriod + 5
	}
	atrPeriod := e.cfg.ATRPeriod
	if atrPeriod <= 0 {
		atrPeriod = 14
	}
	if e.cfg.VolatilityExpansionEnable && needCandles < atrPeriod+10 {
		needCandles = atrPeriod + 10
	}
	if e.cfg.EMASlow > 0 && needCandles < e.cfg.EMASlow+5 {
		needCandles = e.cfg.EMASlow + 5
	}
	klines, err := e.client.Klines(ctx, symbol, e.cfg.Interval, needCandles)
	if err != nil || len(klines) < e.cfg.RSIPeriod+2 {
		return Hold, err
	}

	closes := klineCloses(klines)
	quoteVolumes := klineQuoteVolumes(klines)
	n := len(closes)

	// RSI pullback + hacim: Long = RSI < low, Short = RSI > high
	rsiVal := rsiWilder(closes, e.cfg.RSIPeriod)
	if math.IsNaN(rsiVal) {
		return Hold, nil
	}
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
	var rsiSignal Signal
	if rsiVal < e.cfg.RSIThresholdLow && volumeOK {
		rsiSignal = Long
	} else if rsiVal > e.cfg.RSIThresholdHigh && volumeOK {
		rsiSignal = Short
	} else {
		rsiSignal = Hold
	}

	// Trend filtresi: Long sadece uptrend (price > EMA200, EMA50 > EMA200), Short sadece downtrend
	if e.cfg.EMASlow > 0 && n >= e.cfg.EMASlow && rsiSignal != Hold {
		price := closes[n-1]
		emaSlow := ema(closes, e.cfg.EMASlow)
		fastPeriod := e.cfg.EMAFast
		if fastPeriod <= 0 {
			fastPeriod = 50
		}
		emaFastVal := ema(closes, fastPeriod)
		uptrend := price > emaSlow && emaFastVal > emaSlow
		downtrend := price < emaSlow && emaFastVal < emaSlow
		if rsiSignal == Long && !uptrend {
			return Hold, nil
		}
		if rsiSignal == Short && !downtrend {
			return Hold, nil
		}
	}

	// Orderbook imbalance: LONG için imbalance >= LongMin, SHORT için <= ShortMax (ek filtre)
	if e.cfg.OrderbookImbalanceEnable && rsiSignal != Hold {
		limit := e.cfg.OrderbookDepthLimit
		if limit <= 0 {
			limit = 20
		}
		imb, errImb := e.client.GetOrderbookImbalance(ctx, symbol, limit)
		if errImb != nil {
			return Hold, errImb
		}
		if rsiSignal == Long && imb < e.cfg.OrderbookImbalanceLongMin {
			return Hold, nil
		}
		if rsiSignal == Short && imb > e.cfg.OrderbookImbalanceShortMax {
			return Hold, nil
		}
	}

	// Volatility expansion: current ATR > avg ATR * ratio
	if e.cfg.VolatilityExpansionEnable && rsiSignal != Hold {
		highs, lows := klineHighsLows(klines)
		ok := volatilityExpansion(highs, lows, closes, atrPeriod, e.cfg.ATRExpansionRatio)
		if !ok {
			return Hold, nil
		}
	}

	// Liquidation cascade (şimdilik devre dışı; WebSocket eklenince doldurulacak)
	if e.cfg.LiquidationCascadeEnable {
		// TODO: liquidation feed'den son N sn hacim kontrolü
	}

	return rsiSignal, nil
}

// volatilityExpansion son mum için current ATR > ortalama ATR * ratio ise true.
func volatilityExpansion(highs, lows, closes []float64, period int, ratio float64) bool {
	if len(closes) < period+5 || ratio <= 0 || len(highs) != len(closes) || len(lows) != len(closes) {
		return true
	}
	atrCur := atr(highs, lows, closes, len(closes)-1, period)
	avg := 0.0
	n := 0
	for i := period; i < len(closes)-1; i++ {
		avg += atr(highs, lows, closes, i, period)
		n++
	}
	if n == 0 {
		return true
	}
	avg /= float64(n)
	if avg == 0 {
		return true
	}
	return atrCur >= avg*ratio
}

func klineHighsLows(klines []*futures.Kline) (highs, lows []float64) {
	highs = make([]float64, 0, len(klines))
	lows = make([]float64, 0, len(klines))
	for _, k := range klines {
		highs = append(highs, mustFloat(k.High))
		lows = append(lows, mustFloat(k.Low))
	}
	return highs, lows
}

// atr True Range ile ATR(period), endIdx dahil son period mum. Wilder smoothing yerine basit ort.
func atr(highs, lows, closes []float64, endIdx, period int) float64 {
	if endIdx < period || len(closes) <= endIdx || len(highs) <= endIdx || len(lows) <= endIdx {
		return 0
	}
	trSum := 0.0
	for i := endIdx - period + 1; i <= endIdx; i++ {
		if i <= 0 {
			continue
		}
		tr := highs[i] - lows[i]
		if i > 0 {
			tr = max(highs[i]-lows[i], math.Abs(highs[i]-closes[i-1]), math.Abs(lows[i]-closes[i-1]))
		}
		trSum += tr
	}
	return trSum / float64(period)
}

func max(a, b, c float64) float64 {
	if a >= b && a >= c {
		return a
	}
	if b >= c {
		return b
	}
	return c
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

// ema son indeksteki EMA(period) değerini döner. closes en az period uzunlukta olmalı.
func ema(closes []float64, period int) float64 {
	if period <= 0 || len(closes) < period {
		return 0
	}
	k := 2.0 / float64(period+1)
	// İlk EMA = ilk period mumun kapanış ortalaması
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += closes[i]
	}
	emaVal := sum / float64(period)
	for i := period; i < len(closes); i++ {
		emaVal = closes[i]*k + emaVal*(1-k)
	}
	return emaVal
}
