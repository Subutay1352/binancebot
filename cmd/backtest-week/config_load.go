package main

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// loadBacktestEnv backtest.env okur — canlı .env / DB’ye dokunmaz.
func loadBacktestEnv(path string) (cfg simCfg, interval string, tradeSymbols []string, err error) {
	path = resolveBacktestPath(path)
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, "", nil, fmt.Errorf("%w\n→ cp backtest.env.example backtest.env && dosyayı düzenle", err)
	}
	m, err := godotenv.Unmarshal(string(b))
	if err != nil {
		return cfg, "", nil, err
	}
	cfg = simCfgFromMap(m)
	if cfg.RSIHigh <= cfg.RSILow {
		cfg.RSIHigh = cfg.RSILow + 5
	}
	interval = strings.TrimSpace(m["STRATEGY_INTERVAL"])
	if interval == "" {
		interval = "5m"
	}
	tradeSymbols = parseTradeSymbols(m["TRADE_SYMBOLS"])
	return cfg, interval, tradeSymbols, nil
}

func resolveBacktestPath(path string) string {
	if path == "" {
		path = "backtest.env"
	}
	if strings.HasPrefix(path, "/") || strings.HasPrefix(path, "./") {
		return path
	}
	if _, err := os.Stat(path); err == nil {
		return path
	}
	// proje kökü (cmd/backtest-week’ten çalışınca ../../backtest.env)
	for _, p := range []string{path, "../" + path, "../../" + path} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return path
}

func simCfgFromMap(m map[string]string) simCfg {
	ap := mapInt(m, "ATR_PERIOD", 14)
	if ap <= 0 {
		ap = 14
	}
	return simCfg{
		RSIPeriod:           mapInt(m, "STRATEGY_RSI_PERIOD", 7),
		RSILow:              mapFloat(m, "STRATEGY_RSI_LOW", 45),
		RSIHigh:             mapFloat(m, "STRATEGY_RSI_HIGH", 55),
		RSISlope:            mapBool(m, "STRATEGY_RSI_SLOPE_ENABLE", true),
		MinVolUSD:           mapFloat(m, "STRATEGY_MIN_VOLUME_USD", 500_000),
		VolRatio:            mapFloat(m, "STRATEGY_VOLUME_RATIO", 1.5),
		VolAvgN:             mapInt(m, "STRATEGY_VOLUME_AVG_PERIOD", 20),
		VolMinAvg:           mapFloat(m, "STRATEGY_VOLUME_MIN_RATIO_AVG", 1.8),
		EMASlow:             mapInt(m, "EMA_SLOW", 200),
		ATR:                 mapBool(m, "VOLATILITY_EXPANSION_ENABLE", false),
		ATRRatio:            mapFloat(m, "ATR_EXPANSION_RATIO", 1.4),
		ATRPeriod:           ap,
		OrderbookEnabled:       mapBool(m, "ORDERBOOK_IMBALANCE_ENABLE", false),
		OrderbookLongMin:       mapFloat(m, "ORDERBOOK_IMBALANCE_LONG_MIN", 0.65),
		OrderbookShortMax:      mapFloat(m, "ORDERBOOK_IMBALANCE_SHORT_MAX", 0.35),
		OrderbookSimImbalance:  mapFloat(m, "ORDERBOOK_SIM_IMBALANCE", 0.66), // API yok; long eğilimli örnek 0.66, short için ~0.30
	}
}

func parseTradeSymbols(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		s := strings.TrimSpace(strings.ToUpper(p))
		if s == "" {
			continue
		}
		if !strings.HasSuffix(s, "USDT") {
			s += "USDT"
		}
		out = append(out, s)
	}
	return out
}

func pickSymbolsFromList(single string, batch int, list []string) []string {
	single = strings.TrimSpace(strings.ToUpper(single))
	if single != "" {
		if !strings.HasSuffix(single, "USDT") {
			single += "USDT"
		}
		return []string{single}
	}
	if len(list) == 0 {
		return []string{"BTCUSDT"}
	}
	out := append([]string(nil), list...)
	rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	if batch > 0 && len(out) > batch {
		out = out[:batch]
	}
	return out
}

func mapGet(m map[string]string, key, def string) string {
	if v := strings.TrimSpace(m[key]); v != "" {
		return v
	}
	return def
}

func mapInt(m map[string]string, key string, def int) int {
	v := strings.TrimSpace(m[key])
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}

func mapFloat(m map[string]string, key string, def float64) float64 {
	v := strings.TrimSpace(m[key])
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

func mapBool(m map[string]string, key string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(m[key]))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
