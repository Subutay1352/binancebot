package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config uygulama ayarları. .env veya ortam değişkenlerinden okunur.
type Config struct {
	BotEnabled bool // BOT_ENABLED: true=tarama+işlem çalışır, false=sadece API/dashboard ayakta (boş servis)
	Binance    BinanceConfig
	Postgres   PostgresConfig
	Telegram   TelegramConfig
	Trade      TradeConfig
	Strategy   StrategyConfig
}

type BinanceConfig struct {
	APIKey                 string
	SecretKey              string
	Testnet                bool
	TestnetCleanupOnStart  bool // true ise başlangıçta açık pozisyonlar kapatılır ve trades tablosu temizlenir (sadece testnet'te anlamlı)
}

type PostgresConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DB       string
	URL      string // Doluysa tek bağlantı string'i olarak kullanılır
}

type TelegramConfig struct {
	BotToken string
	ChatID   string
}

// TradeConfig: Tüm işlemler USDT çiftleri (BTCUSDT, ETHUSDT vb.). Margin = USDT (USDT-M Futures).
type TradeConfig struct {
	Symbol             string   // Tek sembol (TRADE_SYMBOL), örn. BTCUSDT
	Symbols            []string // Taranacak çiftler (TRADE_SYMBOLS), sadece USDT pair: BTCUSDT, ETHUSDT...
	StopLossPercent    float64
	TakeProfitPercent  float64
	PositionSizeUSD    float64 // Marj per pozisyon; RiskPerTrade > 0 ise marj = bakiye * RiskPerTrade% / StopLoss% (RR bazlı)
	Leverage           int     // Kaldıraç (LEVERAGE); 5 ise 50 USDT marj → 250 USDT notional
	RiskPerTrade       float64 // Her işlemde riske atılan bakiye yüzdesi (örn 1 = %1). 0 ise PositionSizeUSD kullanılır
	Min24hVolumeUSD    float64 // Sadece 24s hacmi bu değerin üstündeki semboller taranır (0=kapalı, örn 100M)
	MaxOpenTrades      int     // Aynı anda en fazla bu kadar açık pozisyon (0 = sınırsız)
	MaxLossesIn12h     int     // Son 12 saatte bu sayıdan fazla zarar varsa yeni işlem açılmaz (0=kapalı)
	MinBalanceShutdown float64 // Futures bakiyesi bu değerin altına inerse bot kapanır (0=kapalı)
	InstanceID         string  // Hangi makine/süreç (BOT_INSTANCE_ID veya hostname)
	ExecutorPollSec    int     // SL/TP kapanış kontrolü kaç saniyede bir (EXECUTOR_POLL_INTERVAL_SEC, 0=varsayılan 60)
}

// StrategyConfig hybrid strateji: trend (EMA) + RSI pullback + hacim + orderbook + volatility + liquidation.
type StrategyConfig struct {
	Interval            string  // Mum aralığı: "5m" (önerilen), "15m" daha stabil
	RSIPeriod           int     // RSI periyodu (pullback için 7 önerilir)
	RSIThresholdLow     float64 // Long pullback: RSI < bu (örn 35)
	RSIThresholdHigh    float64 // Short pullback: RSI > bu (örn 65)
	MinVolumeUSD        float64 // Hacim en az bu kadar USD (ek filtre)
	VolumeRatioVsPrev   float64 // Son mumdan en az bu katı (ek filtre)
	VolumeAvgPeriod     int     // Ortalama hacim için son N mum (20 önerilir)
	VolumeMinRatioToAvg float64 // Mevcut hacim >= ortalama * bu oran (örn 1.8 = volume spike)

	// Trend: EMA 50/200. Long: price > EMA200 && EMA50 > EMA200; Short: price < EMA200 && EMA50 < EMA200
	EMAFast int // EMA hızlı (50 önerilir), 0=trend filtresi kapalı
	EMASlow int // EMA yavaş (200 önerilir)

	// Orderbook imbalance: bid_vol/(bid_vol+ask_vol). > LongMin = alım, < ShortMax = satım
	OrderbookImbalanceEnable  bool    // ORDERBOOK_IMBALANCE_ENABLE
	OrderbookImbalanceLongMin float64 // LONG için imbalance >= bu (0.65)
	OrderbookImbalanceShortMax float64 // SHORT için imbalance <= bu (0.35)
	OrderbookDepthLimit       int     // Depth kaç seviye (20)

	// Volatility expansion: current ATR > avg ATR * ratio
	VolatilityExpansionEnable bool    // VOLATILITY_EXPANSION_ENABLE
	ATRPeriod                 int     // ATR periyodu (14)
	ATRExpansionRatio         float64 // current ATR > avg ATR * bu oran (1.4)

	// Liquidation cascade (WebSocket !forceOrder): son N saniyedeki liquidation hacmi (USD)
	LiquidationCascadeEnable bool    // LIQUIDATION_CASCADE_ENABLE
	LiquidationWindowSec     int     // Son kaç saniye (10)
	LiquidationMinUSD        float64 // En az bu kadar USD (500k; BTC 2M, ETH 1M)
}

// Load .env dosyasını yükler ve Config döner.
func Load() (*Config, error) {
	_ = godotenv.Load()

	testnet := envBool("BINANCE_FUTURES_TESTNET", true)
	return &Config{
		BotEnabled: envBool("BOT_ENABLED", true),
		Binance: BinanceConfig{
			APIKey:                os.Getenv("BINANCE_API_KEY"),
			SecretKey:             os.Getenv("BINANCE_SECRET_KEY"),
			Testnet:               testnet,
			TestnetCleanupOnStart: testnet && envBool("BINANCE_TESTNET_CLEANUP_ON_START", false),
		},
		Postgres: postgresFromEnv(),
		Telegram: TelegramConfig{
			BotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
			ChatID:   os.Getenv("TELEGRAM_CHAT_ID"),
		},
		Trade: tradeConfigFromEnv(),
		Strategy: StrategyConfig{
			Interval:            env("STRATEGY_INTERVAL", "5m"),
			RSIPeriod:           envInt("STRATEGY_RSI_PERIOD", 7),
			RSIThresholdLow:     envFloat("STRATEGY_RSI_LOW", 35),
			RSIThresholdHigh:    envFloat("STRATEGY_RSI_HIGH", 65),
			MinVolumeUSD:        envFloat("STRATEGY_MIN_VOLUME_USD", 500_000),
			VolumeRatioVsPrev:   envFloat("STRATEGY_VOLUME_RATIO", 1.5),
			VolumeAvgPeriod:     envInt("STRATEGY_VOLUME_AVG_PERIOD", 20),
			VolumeMinRatioToAvg: envFloat("STRATEGY_VOLUME_MIN_RATIO_AVG", 1.8),

			EMAFast: envInt("EMA_FAST", 50),
			EMASlow: envInt("EMA_SLOW", 200),

			OrderbookImbalanceEnable:   envBool("ORDERBOOK_IMBALANCE_ENABLE", false),
			OrderbookImbalanceLongMin:  envFloat("ORDERBOOK_IMBALANCE_LONG_MIN", 0.65),
			OrderbookImbalanceShortMax: envFloat("ORDERBOOK_IMBALANCE_SHORT_MAX", 0.35),
			OrderbookDepthLimit:        envInt("ORDERBOOK_DEPTH_LIMIT", 20),

			VolatilityExpansionEnable: envBool("VOLATILITY_EXPANSION_ENABLE", false),
			ATRPeriod:                 envInt("ATR_PERIOD", 14),
			ATRExpansionRatio:         envFloat("ATR_EXPANSION_RATIO", 1.4),

			LiquidationCascadeEnable: envBool("LIQUIDATION_CASCADE_ENABLE", false),
			LiquidationWindowSec:     envInt("LIQUIDATION_WINDOW_SEC", 10),
			LiquidationMinUSD:        envFloat("LIQUIDATION_MIN_USD", 500_000),
		},
	}, nil
}

func postgresFromEnv() PostgresConfig {
	p := PostgresConfig{
		URL:  os.Getenv("DATABASE_URL"),
		Host: env("POSTGRES_HOST", "localhost"),
		Port: envInt("POSTGRES_PORT", 5432),
		User: env("POSTGRES_USER", "binancebot"),
		DB:   env("POSTGRES_DB", "binancebot"),
	}
	p.Password = os.Getenv("POSTGRES_PASSWORD")
	return p
}

func env(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envInt(key string, defaultVal int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultVal
}

func envFloat(key string, defaultVal float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return defaultVal
}

func envBool(key string, defaultVal bool) bool {
	if v := os.Getenv(key); v != "" {
		b, _ := strconv.ParseBool(v)
		return b
	}
	return defaultVal
}

func tradeConfigFromEnv() TradeConfig {
	symbols := symbolsFromEnv()
	sym := "BTCUSDT"
	if len(symbols) > 0 {
		sym = symbols[0]
	}
	return TradeConfig{
		Symbol:             sym,
		Symbols:            symbols,
		StopLossPercent:    envFloat("STOP_LOSS_PERCENT", 3.0),
		TakeProfitPercent:  envFloat("TAKE_PROFIT_PERCENT", 6.0),
		PositionSizeUSD:    envFloat("POSITION_SIZE_USD", 100.0),
		Leverage:           envInt("LEVERAGE", 3),
		RiskPerTrade:      envFloat("RISK_PER_TRADE", 1.0),
		Min24hVolumeUSD:   envFloat("MIN_24H_VOLUME_USD", 100_000_000),
		MaxOpenTrades:     envInt("MAX_OPEN_TRADES", 5),
		MaxLossesIn12h:    envInt("MAX_LOSSES_IN_12H", 2),
		MinBalanceShutdown: envFloat("MIN_BALANCE_SHUTDOWN", 0),
		InstanceID:        os.Getenv("BOT_INSTANCE_ID"),
		ExecutorPollSec:   envInt("EXECUTOR_POLL_INTERVAL_SEC", 10),
	}
}

// ApplyRuntimeOverrides base config üzerine DB'den gelen runtime ayarlarını uygular.
// Sadece RuntimeConfigKeys ile tanımlı alanlar override edilir; boş string yok sayılır.
func ApplyRuntimeOverrides(base *Config, overrides map[string]string) *Config {
	if base == nil || overrides == nil {
		return base
	}
	out := *base
	out.Trade = base.Trade
	out.Strategy = base.Strategy
	parseFloat := func(s string) (float64, bool) {
		s = strings.TrimSpace(s)
		if s == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(s, 64)
		return f, err == nil
	}
	parseInt := func(s string) (int, bool) {
		s = strings.TrimSpace(s)
		if s == "" {
			return 0, false
		}
		i, err := strconv.Atoi(s)
		return i, err == nil
	}
	if v, ok := parseFloat(overrides["STOP_LOSS_PERCENT"]); ok {
		out.Trade.StopLossPercent = v
	}
	if v, ok := parseFloat(overrides["TAKE_PROFIT_PERCENT"]); ok {
		out.Trade.TakeProfitPercent = v
	}
	if s := strings.TrimSpace(overrides["STRATEGY_INTERVAL"]); s != "" {
		out.Strategy.Interval = s
	}
	if v, ok := parseInt(overrides["STRATEGY_RSI_PERIOD"]); ok && v > 0 {
		out.Strategy.RSIPeriod = v
	}
	if v, ok := parseFloat(overrides["STRATEGY_RSI_LOW"]); ok {
		out.Strategy.RSIThresholdLow = v
	}
	if v, ok := parseFloat(overrides["STRATEGY_RSI_HIGH"]); ok {
		out.Strategy.RSIThresholdHigh = v
	}
	if v, ok := parseFloat(overrides["STRATEGY_MIN_VOLUME_USD"]); ok {
		out.Strategy.MinVolumeUSD = v
	}
	if v, ok := parseFloat(overrides["STRATEGY_VOLUME_RATIO"]); ok {
		out.Strategy.VolumeRatioVsPrev = v
	}
	if v, ok := parseInt(overrides["STRATEGY_VOLUME_AVG_PERIOD"]); ok {
		out.Strategy.VolumeAvgPeriod = v
	}
	if v, ok := parseFloat(overrides["STRATEGY_VOLUME_MIN_RATIO_AVG"]); ok {
		out.Strategy.VolumeMinRatioToAvg = v
	}
	return &out
}

// RuntimeConfigFromConfig mevcut config değerlerini runtime key-value map olarak döner (UI varsayılanları için).
func RuntimeConfigFromConfig(cfg *Config) map[string]string {
	if cfg == nil {
		return nil
	}
	return map[string]string{
		"STOP_LOSS_PERCENT":               strconv.FormatFloat(cfg.Trade.StopLossPercent, 'f', -1, 64),
		"TAKE_PROFIT_PERCENT":             strconv.FormatFloat(cfg.Trade.TakeProfitPercent, 'f', -1, 64),
		"STRATEGY_INTERVAL":               cfg.Strategy.Interval,
		"STRATEGY_RSI_PERIOD":             strconv.Itoa(cfg.Strategy.RSIPeriod),
		"STRATEGY_RSI_LOW":                strconv.FormatFloat(cfg.Strategy.RSIThresholdLow, 'f', -1, 64),
		"STRATEGY_RSI_HIGH":               strconv.FormatFloat(cfg.Strategy.RSIThresholdHigh, 'f', -1, 64),
		"STRATEGY_MIN_VOLUME_USD":         strconv.FormatFloat(cfg.Strategy.MinVolumeUSD, 'f', -1, 64),
		"STRATEGY_VOLUME_RATIO":           strconv.FormatFloat(cfg.Strategy.VolumeRatioVsPrev, 'f', -1, 64),
		"STRATEGY_VOLUME_AVG_PERIOD":      strconv.Itoa(cfg.Strategy.VolumeAvgPeriod),
		"STRATEGY_VOLUME_MIN_RATIO_AVG":   strconv.FormatFloat(cfg.Strategy.VolumeMinRatioToAvg, 'f', -1, 64),
	}
}

// symbolsFromEnv sadece TRADE_SYMBOLS (virgülle ayrılmış) kullanır. Boşsa varsayılan [BTCUSDT].
func symbolsFromEnv() []string {
	s := os.Getenv("TRADE_SYMBOLS")
	if s == "" {
		return []string{"BTCUSDT"}
	}
	var list []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(strings.ToUpper(part))
		if part == "" {
			continue
		}
		if !strings.HasSuffix(part, "USDT") {
			part = part + "USDT"
		}
		list = append(list, part)
	}
	if len(list) == 0 {
		return []string{"BTCUSDT"}
	}
	return list
}
