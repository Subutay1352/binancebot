// Backtest-week: bot Decide() ile aynı kurallar (RSI+hacim+EMA+orderbook+ATR).
// Varsayılan: proje kökünde backtest.env (canlı .env / DB’ye dokunmaz). Canlı ayar için: -live [-db].
//
//	go run ./cmd/backtest-week -days 7 -mainnet
//	go run ./cmd/backtest-week -live -days 7 -mainnet -db
//	go run ./cmd/backtest-week -config ./my.env -symbol BTCUSDT
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"binancebot/config"
	"binancebot/internal/db"

	"github.com/adshao/go-binance/v2/futures"
)

func main() {
	symbol := flag.String("symbol", "", "Tek sembol (doluysa sadece o). Boşsa .env TRADE_SYMBOLS’ten -batch kadar karışık")
	batch := flag.Int("batch", 10, "TRADE_SYMBOLS’ten kaç sembol simüle et (karışık sıra)")
	days := flag.Int("days", 7, "Kaç gün geriye")
	intervalFlag := flag.String("interval", "5m", "Mum (-no-env veya .env’de STRATEGY_INTERVAL yoksa)")
	mainnet := flag.Bool("mainnet", true, "true=mainnet veri; false=testnet")
	live := flag.Bool("live", false, "Strateji + semboller: canlı .env (+ isteğe -db); yoksa backtest.env")
	configPath := flag.String("config", "backtest.env", "backtest env dosyası (-live kapalıyken)")
	useDB := flag.Bool("db", false, "Sadece -live: runtime_config üzerine yaz (DATABASE_URL)")
	noEnv := flag.Bool("no-env", false, "Stratejiyi yalnızca flag’lerden al (backtest.env yok)")
	// Sadece -no-env iken kullanılır:
	minVol := flag.Float64("min-vol", 0, "")
	rsiLow := flag.Float64("rsi-low", 45, "")
	rsiHigh := flag.Float64("rsi-high", 55, "")
	rsiPeriod := flag.Int("rsi-period", 7, "")
	emaSlow := flag.Int("ema-slow", 200, "")
	slope := flag.Bool("rsi-slope", true, "")
	volRatio := flag.Float64("vol-ratio", 1.5, "")
	volAvgN := flag.Int("vol-avg-n", 20, "")
	volMinAvg := flag.Float64("vol-min-avg", 1.8, "")
	atrOn := flag.Bool("atr", false, "")
	atrRatio := flag.Float64("atr-ratio", 1.4, "")
	atrPeriod := flag.Int("atr-period", 14, "")
	flag.Parse()

	ctx := context.Background()
	var cfg simCfg
	var interval string
	source := "flag"
	var eff *config.Config
	var tradeList []string

	if *noEnv {
		cfg = simCfg{
			RSIPeriod: *rsiPeriod, RSILow: *rsiLow, RSIHigh: *rsiHigh, RSISlope: *slope,
			MinVolUSD: *minVol, VolRatio: *volRatio, VolAvgN: *volAvgN, VolMinAvg: *volMinAvg,
			EMASlow: *emaSlow, ATR: *atrOn, ATRRatio: *atrRatio, ATRPeriod: *atrPeriod,
		}
		interval = *intervalFlag
	} else if *live {
		base, err := config.Load()
		if err != nil {
			log.Fatalf("config.Load (.env): %v", err)
		}
		eff = base
		if *useDB {
			if base.Postgres.URL == "" && base.Postgres.Host == "" {
				log.Fatal("-db için DATABASE_URL veya POSTGRES_* .env’de olmalı")
			}
			st, err := db.NewStore(ctx, base)
			if err != nil {
				log.Fatalf("DB: %v", err)
			}
			defer st.Close()
			ov, err := st.GetRuntimeConfig(ctx)
			if err != nil {
				log.Fatalf("runtime_config: %v", err)
			}
			logRuntimeConfigFromDB(ov)
			eff = config.ApplyRuntimeOverrides(base, ov)
			source = ".env + DB runtime_config"
		} else {
			source = ".env (config.Load)"
		}
		cfg = strategyToSim(eff.Strategy)
		interval = eff.Strategy.Interval
		if interval == "" {
			interval = *intervalFlag
		}
		fmt.Fprintf(os.Stderr, "=== Strateji kaynağı: %s ===\n", source)
		if *useDB {
			fmt.Fprintf(os.Stderr, "(Aşağıdaki satırlar DB ile birleştirilmiş son değerler.)\n")
		}
		printStrategySim(cfg, interval)
		fmt.Fprintf(os.Stderr, "Kapsam: Decide() = RSI + hacim + EMA_SLOW + [orderbook] + [ATR]. Diğer env alanları yok sayılır.\n\n")
	} else {
		if *useDB {
			log.Fatal("-db yalnızca -live ile kullanılır (canlı .env + runtime_config)")
		}
		var err error
		cfg, interval, tradeList, err = loadBacktestEnv(*configPath)
		if err != nil {
			log.Fatalf("backtest env: %v", err)
		}
		source = *configPath
		fmt.Fprintf(os.Stderr, "=== Strateji kaynağı: %s (canlı .env kullanılmıyor) ===\n", source)
		printStrategySim(cfg, interval)
		fmt.Fprintf(os.Stderr, "Orderbook açıksa backtest’te ORDERBOOK_SIM_IMBALANCE sabit (API yok). Kapsam: Decide() ile aynı mantık.\n\n")
	}

	var syms []string
	if *noEnv || *live {
		syms = pickSymbols(*symbol, *batch, eff)
	} else {
		syms = pickSymbolsFromList(*symbol, *batch, tradeList)
	}
	if len(syms) == 0 {
		log.Fatal("Sembol listesi boş (TRADE_SYMBOLS veya -symbol)")
	}
	fmt.Fprintf(os.Stderr, "=== Simülasyon: %d sembol | %s → %s | %s ===\n\n",
		len(syms), time.Now().UTC().Add(-time.Duration(*days)*24*time.Hour).Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339), interval)
	for i, s := range syms {
		fmt.Fprintf(os.Stderr, "[%d/%d] %s\n", i+1, len(syms), s)
	}

	api := futures.NewClient("", "")
	if !*mainnet {
		api.BaseURL = futures.BaseApiTestnetUrl
	}

	end := time.Now().UTC()
	start := end.Add(-time.Duration(*days) * 24 * time.Hour)
	startMs, endMs := start.UnixMilli(), end.UnixMilli()
	warm := warmupBars(cfg)

	type row struct {
		sym              string
		L, S, H, bars    int
		pnl              float64
		err, skipReason  string
	}
	var rows []row

	for _, sym := range syms {
		nL, nS, nH, pnl, bars, sigs, err := runSymbolBacktest(ctx, api, sym, interval, startMs, endMs, cfg, warm)
		r := row{sym: sym, L: nL, S: nS, H: nH, bars: bars, pnl: pnl}
		if err != nil {
			r.skipReason = err.Error()
		}
		rows = append(rows, r)

		if len(syms) == 1 && err == nil {
			fmt.Printf("\n=== %s | Sinyal özeti (%d bar, warmup=%d) ===\n", sym, bars, warm)
			fmt.Printf("Long: %d | Short: %d | Hold: %d\n", nL, nS, nH)
			fmt.Printf("\n=== Naif flip PnL (1x) ===\n%.4f %%\n", pnl*100)
			fmt.Printf("\n=== Son sinyaller ===\n")
			for j := len(sigs) - 1; j >= 0 && j >= len(sigs)-15; j-- {
				fmt.Printf("%s  %s\n", sigs[j][0], sigs[j][1])
			}
		}
	}

	fmt.Printf("\n%-18s %5s %5s %5s %8s %10s  %s\n", "SYMBOL", "Long", "Shrt", "Hold", "bars", "PnL%", "not")
	fmt.Printf("%s\n", strings.Repeat("-", 78))
	var sumPnL float64
	okN := 0
	for _, r := range rows {
		note := ""
		if r.skipReason != "" {
			note = r.skipReason
		} else {
			sumPnL += r.pnl
			okN++
		}
		fmt.Printf("%-18s %5d %5d %5d %8d %9.2f%%  %s\n", r.sym, r.L, r.S, r.H, r.bars, r.pnl*100, note)
	}
	if okN > 0 {
		fmt.Printf("%s\n", strings.Repeat("-", 78))
		fmt.Printf("%-18s %5s %5s %5s %8s %9.2f%%  (ort. PnL, %d sembol)\n", "TOPLAM/ORT", "", "", "", "", sumPnL/float64(okN)*100, okN)
	}
}

func logRuntimeConfigFromDB(ov map[string]string) {
	fmt.Fprintf(os.Stderr, "\n=== PostgreSQL runtime_config (DB’den okunan) ===\n")
	if len(ov) == 0 {
		fmt.Fprintf(os.Stderr, "  (kayıt yok — strateji sadece .env)\n\n")
		return
	}
	keys := make([]string, 0, len(ov))
	for k := range ov {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(os.Stderr, "  %s = %s\n", k, ov[k])
	}
	fmt.Fprintf(os.Stderr, "  — toplam %d anahtar\n\n", len(ov))
}

func pickSymbols(single string, batch int, eff *config.Config) []string {
	single = strings.TrimSpace(strings.ToUpper(single))
	if single != "" {
		if !strings.HasSuffix(single, "USDT") {
			single += "USDT"
		}
		return []string{single}
	}
	if eff == nil {
		return []string{"BTCUSDT"}
	}
	list := eff.Trade.Symbols
	if len(list) == 0 {
		s := strings.TrimSpace(strings.ToUpper(eff.Trade.Symbol))
		if s == "" {
			s = "BTCUSDT"
		}
		return []string{s}
	}
	out := make([]string, len(list))
	copy(out, list)
	for i := range out {
		out[i] = strings.TrimSpace(strings.ToUpper(out[i]))
	}
	rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	if batch > 0 && len(out) > batch {
		out = out[:batch]
	}
	return out
}

func runSymbolBacktest(ctx context.Context, api *futures.Client, sym, interval string, startMs, endMs int64, cfg simCfg, warm int) (nLong, nShort, nHold int, pnl float64, bars int, signals [][2]string, err error) {
	klines, e := fetchAllKlines(ctx, api, sym, interval, startMs, endMs)
	if e != nil {
		return 0, 0, 0, 0, 0, nil, e
	}
	if len(klines) < warm+1 {
		return 0, 0, 0, 0, 0, nil, fmt.Errorf("yetersiz mum (%d<%d)", len(klines), warm+1)
	}
	var sigs [][2]string
	for i := warm; i < len(klines); i++ {
		sig := signalAt(klines[:i+1], cfg)
		switch sig {
		case "L":
			nLong++
		case "S":
			nShort++
		default:
			nHold++
		}
		ts := time.UnixMilli(klines[i].OpenTime).UTC().Format("01-02 15:04")
		sigs = append(sigs, [2]string{ts, sig})
	}
	bars = len(klines) - warm
	pnl = naiveFlipPnL(klines, sigs, warm)
	return nLong, nShort, nHold, pnl, bars, sigs, nil
}

type simCfg struct {
	RSIPeriod int
	RSILow    float64
	RSIHigh   float64
	RSISlope  bool
	MinVolUSD float64
	VolRatio  float64
	VolAvgN   int
	VolMinAvg float64
	EMASlow   int
	ATR       bool
	ATRRatio  float64
	ATRPeriod int
	// Orderbook (backtest’te ORDERBOOK_SIM_IMBALANCE sabit; canlıda gerçek orderbook)
	OrderbookEnabled      bool
	OrderbookLongMin      float64
	OrderbookShortMax     float64
	OrderbookSimImbalance float64
}

func strategyToSim(s config.StrategyConfig) simCfg {
	ap := s.ATRPeriod
	if ap <= 0 {
		ap = 14
	}
	simImb := 0.66
	if s.OrderbookImbalanceEnable && simImb < s.OrderbookImbalanceLongMin {
		simImb = s.OrderbookImbalanceLongMin + 0.01
	}
	return simCfg{
		RSIPeriod: s.RSIPeriod, RSILow: s.RSIThresholdLow, RSIHigh: s.RSIThresholdHigh, RSISlope: s.RSISlopeEnable,
		MinVolUSD: s.MinVolumeUSD, VolRatio: s.VolumeRatioVsPrev, VolAvgN: s.VolumeAvgPeriod, VolMinAvg: s.VolumeMinRatioToAvg,
		EMASlow: s.EMASlow, ATR: s.VolatilityExpansionEnable, ATRRatio: s.ATRExpansionRatio, ATRPeriod: ap,
		OrderbookEnabled: s.OrderbookImbalanceEnable, OrderbookLongMin: s.OrderbookImbalanceLongMin,
		OrderbookShortMax: s.OrderbookImbalanceShortMax, OrderbookSimImbalance: simImb,
	}
}

func warmupBars(c simCfg) int {
	w := c.EMASlow
	if w < c.RSIPeriod+25 {
		w = c.RSIPeriod + 25
	}
	if c.VolAvgN+5 > w {
		w = c.VolAvgN + 5
	}
	if c.ATR {
		ap := c.ATRPeriod
		if ap <= 0 {
			ap = 14
		}
		if w < ap+10 {
			w = ap + 10
		}
	}
	return w
}

func printStrategySim(c simCfg, iv string) {
	ap := c.ATRPeriod
	if ap <= 0 {
		ap = 14
	}
	fmt.Fprintf(os.Stderr, "  STRATEGY_INTERVAL=%s RSI %d L/H %.0f/%.0f slope=%v | minVol=%.0f volRatio=%.2f volAvgN=%d volMinAvg=%.2f | EMA_SLOW=%d | ATR=%v period=%d ratio=%.3f | OB=%v simImb=%.2f\n",
		iv, c.RSIPeriod, c.RSILow, c.RSIHigh, c.RSISlope, c.MinVolUSD, c.VolRatio, c.VolAvgN, c.VolMinAvg, c.EMASlow, c.ATR, ap, c.ATRRatio,
		c.OrderbookEnabled, c.OrderbookSimImbalance)
}

func signalAt(all []*futures.Kline, c simCfg) string {
	n := len(all)
	closes := make([]float64, n)
	vols := make([]float64, n)
	highs := make([]float64, n)
	lows := make([]float64, n)
	for i, k := range all {
		closes[i] = pf(k.Close)
		vols[i] = pf(k.QuoteAssetVolume)
		highs[i] = pf(k.High)
		lows[i] = pf(k.Low)
	}
	rsiVal := rsiWilder(closes, c.RSIPeriod)
	if math.IsNaN(rsiVal) {
		return "H"
	}
	curV, prevV := vols[n-1], 0.0
	if n >= 2 {
		prevV = vols[n-2]
	}
	volOK := curV >= c.MinVolUSD || (prevV > 0 && curV >= prevV*c.VolRatio)
	if c.VolAvgN > 0 && c.VolMinAvg > 0 {
		avg := avgVol(vols, n-1, c.VolAvgN)
		if avg > 0 && curV < avg*c.VolMinAvg {
			volOK = false
		}
	}
	prevRsi := rsiWilder(closes[:n-1], c.RSIPeriod)
	slopeUp := !math.IsNaN(prevRsi) && rsiVal > prevRsi
	slopeDn := !math.IsNaN(prevRsi) && rsiVal < prevRsi

	var sig string
	if !volOK {
		sig = "H"
	} else if rsiVal > c.RSIHigh {
		if c.RSISlope && !slopeUp {
			sig = "H"
		} else {
			sig = "L"
		}
	} else if rsiVal < c.RSILow {
		if c.RSISlope && !slopeDn {
			sig = "H"
		} else {
			sig = "S"
		}
	} else {
		sig = "H"
	}
	if c.EMASlow > 0 && n >= c.EMASlow && sig != "H" {
		price := closes[n-1]
		e := ema(closes, c.EMASlow)
		if price > e && sig == "S" {
			return "H"
		}
		if price <= e && sig == "L" {
			return "H"
		}
	}
	if c.OrderbookEnabled && sig != "H" {
		imb := c.OrderbookSimImbalance
		if sig == "L" && imb < c.OrderbookLongMin {
			return "H"
		}
		if sig == "S" && imb > c.OrderbookShortMax {
			return "H"
		}
	}
	if c.ATR && sig != "H" {
		ap := c.ATRPeriod
		if ap <= 0 {
			ap = 14
		}
		if !volatilityOK(highs, lows, closes, ap, c.ATRRatio) {
			return "H"
		}
	}
	return sig
}

func naiveFlipPnL(klines []*futures.Kline, sig [][2]string, warm int) float64 {
	var pos string // "", "L", "S"
	var entry float64
	var pnl float64
	sigIdx := 0
	for i := warm; i < len(klines); i++ {
		px := pf(klines[i].Close)
		if sigIdx < len(sig) {
			s := sig[sigIdx][1]
			sigIdx++
			if s == "L" || s == "S" {
				if pos == "L" {
					pnl += (px - entry) / entry
				} else if pos == "S" {
					pnl += (entry - px) / entry
				}
				pos = s
				entry = px
			}
		}
	}
	if pos != "" {
		last := pf(klines[len(klines)-1].Close)
		if pos == "L" {
			pnl += (last - entry) / entry
		} else {
			pnl += (entry - last) / entry
		}
	}
	return pnl
}

func fetchAllKlines(ctx context.Context, api *futures.Client, sym, iv string, startMs, endMs int64) ([]*futures.Kline, error) {
	var all []*futures.Kline
	cur := startMs
	for cur < endMs {
		batch, err := api.NewKlinesService().Symbol(sym).Interval(iv).StartTime(cur).EndTime(endMs).Limit(1500).Do(ctx)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		all = append(all, batch...)
		last := batch[len(batch)-1].OpenTime
		if len(batch) < 1500 || last >= endMs-1 {
			break
		}
		cur = last + 1
	}
	by := make(map[int64]*futures.Kline)
	for _, k := range all {
		by[k.OpenTime] = k
	}
	ts := make([]int64, 0, len(by))
	for t := range by {
		ts = append(ts, t)
	}
	sort.Slice(ts, func(i, j int) bool { return ts[i] < ts[j] })
	out := make([]*futures.Kline, 0, len(ts))
	for _, t := range ts {
		out = append(out, by[t])
	}
	return out, nil
}

func pf(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func rsiWilder(closes []float64, period int) float64 {
	if len(closes) < period+2 {
		return math.NaN()
	}
	var ag, al float64
	for i := 1; i <= period; i++ {
		ch := closes[i] - closes[i-1]
		if ch > 0 {
			ag += ch
		} else {
			al -= ch
		}
	}
	ag /= float64(period)
	al /= float64(period)
	for i := period + 1; i < len(closes); i++ {
		ch := closes[i] - closes[i-1]
		g, l := 0.0, 0.0
		if ch > 0 {
			g = ch
		} else {
			l = -ch
		}
		ag = (ag*float64(period-1) + g) / float64(period)
		al = (al*float64(period-1) + l) / float64(period)
	}
	if al == 0 {
		return 100
	}
	rs := ag / al
	return 100 - 100/(1+rs)
}

func ema(closes []float64, period int) float64 {
	if period <= 0 || len(closes) < period {
		return 0
	}
	k := 2.0 / float64(period+1)
	var sum float64
	for i := 0; i < period; i++ {
		sum += closes[i]
	}
	v := sum / float64(period)
	for i := period; i < len(closes); i++ {
		v = closes[i]*k + v*(1-k)
	}
	return v
}

func avgVol(vols []float64, endIdx, period int) float64 {
	start := endIdx - period + 1
	if start < 0 {
		start = 0
	}
	if start > endIdx {
		return 0
	}
	var s float64
	for i := start; i <= endIdx; i++ {
		s += vols[i]
	}
	return s / float64(endIdx-start+1)
}

func volatilityOK(highs, lows, closes []float64, period int, ratio float64) bool {
	n := len(closes)
	if n < period+5 || ratio <= 0 {
		return true
	}
	atrCur := atr1(highs, lows, closes, n-1, period)
	var sum float64
	cnt := 0
	for i := period; i < n-1; i++ {
		sum += atr1(highs, lows, closes, i, period)
		cnt++
	}
	if cnt == 0 {
		return true
	}
	avg := sum / float64(cnt)
	if avg == 0 {
		return true
	}
	return atrCur >= avg*ratio
}

func atr1(highs, lows, closes []float64, endIdx, period int) float64 {
	if endIdx < period {
		return 0
	}
	var trSum float64
	for i := endIdx - period + 1; i <= endIdx; i++ {
		if i <= 0 {
			continue
		}
		tr := max3(highs[i]-lows[i], math.Abs(highs[i]-closes[i-1]), math.Abs(lows[i]-closes[i-1]))
		trSum += tr
	}
	return trSum / float64(period)
}

func max3(a, b, c float64) float64 {
	if a >= b && a >= c {
		return a
	}
	if b >= c {
		return b
	}
	return c
}

