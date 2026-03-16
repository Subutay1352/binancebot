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
	APIKey    string
	SecretKey string
	Testnet   bool
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
	Symbol            string   // Tek sembol (TRADE_SYMBOL), örn. BTCUSDT
	Symbols           []string // Taranacak çiftler (TRADE_SYMBOLS), sadece USDT pair: BTCUSDT, ETHUSDT...
	StopLossPercent   float64
	TakeProfitPercent float64
	PositionSizeUSD   float64 // Marj (kilitleyeceğin USDT) per pozisyon; işlem büyüklüğü = PositionSizeUSD * Leverage
	Leverage          int     // Kaldıraç (LEVERAGE); 5 ise 50 USDT marj → 250 USDT notional
	MaxOpenTrades     int     // Aynı anda en fazla bu kadar açık pozisyon (0 = sınırsız)
	MaxLossesIn12h    int     // Son 12 saatte bu sayıdan fazla zarar varsa yeni işlem açılmaz (0=kapalı)
	MinBalanceShutdown float64 // Futures bakiyesi bu değerin altına inerse bot kapanır, yeniden başlatılsa da tekrar kapanır (0=kapalı)
	InstanceID        string  // Hangi makine/süreç (BOT_INSTANCE_ID veya hostname); DB'de kim yazdı görmek için
	ExecutorPollSec   int     // SL/TP kapanış kontrolü kaç saniyede bir (EXECUTOR_POLL_INTERVAL_SEC, 0=varsayılan 60)
}

// StrategyConfig örnek strateji için. Kuralları sonradan .env ile değiştirebilirsin.
type StrategyConfig struct {
	Interval30m         string  // "30m"
	RSIPeriod           int     // 14
	RSIThresholdLow     float64 // Long için RSI bu değerin altındaysa (örn 10)
	RSIThresholdHigh    float64 // Short için RSI bu değerin üstündeyse (örn 90)
	MinVolumeUSD        float64 // Hacim en az bu kadar USD
	VolumeRatioVsPrev   float64 // Son mumdan en az bu katı
	VolumeAvgPeriod     int     // Ortalama hacim için son N mum (0=kapalı)
	VolumeMinRatioToAvg float64 // Mevcut hacim >= ortalama * bu oran (örn 1.0)
}

// Load .env dosyasını yükler ve Config döner.
func Load() (*Config, error) {
	_ = godotenv.Load()

	return &Config{
		BotEnabled: envBool("BOT_ENABLED", true),
		Binance: BinanceConfig{
			APIKey:    os.Getenv("BINANCE_API_KEY"),
			SecretKey: os.Getenv("BINANCE_SECRET_KEY"),
			Testnet:   envBool("BINANCE_FUTURES_TESTNET", true),
		},
		Postgres: postgresFromEnv(),
		Telegram: TelegramConfig{
			BotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
			ChatID:   os.Getenv("TELEGRAM_CHAT_ID"),
		},
		Trade: tradeConfigFromEnv(),
		Strategy: StrategyConfig{
			Interval30m:         env("STRATEGY_INTERVAL", "30m"),
			RSIPeriod:           envInt("STRATEGY_RSI_PERIOD", 14),
			RSIThresholdLow:     envFloat("STRATEGY_RSI_LOW", 10),
			RSIThresholdHigh:    envFloat("STRATEGY_RSI_HIGH", 90),
			MinVolumeUSD:        envFloat("STRATEGY_MIN_VOLUME_USD", 500_000),
			VolumeRatioVsPrev:   envFloat("STRATEGY_VOLUME_RATIO", 3),
			VolumeAvgPeriod:     envInt("STRATEGY_VOLUME_AVG_PERIOD", 20),
			VolumeMinRatioToAvg: envFloat("STRATEGY_VOLUME_MIN_RATIO_AVG", 1.0),
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
		Symbol:            sym,
		Symbols:           symbols,
		StopLossPercent:   envFloat("STOP_LOSS_PERCENT", 2.0),
		TakeProfitPercent: envFloat("TAKE_PROFIT_PERCENT", 3.0),
		PositionSizeUSD:   envFloat("POSITION_SIZE_USD", 100.0),
		Leverage:          envInt("LEVERAGE", 3),
		MaxOpenTrades:     envInt("MAX_OPEN_TRADES", 5),
		MaxLossesIn12h:     envInt("MAX_LOSSES_IN_12H", 2),
		MinBalanceShutdown: envFloat("MIN_BALANCE_SHUTDOWN", 0),
		InstanceID:         os.Getenv("BOT_INSTANCE_ID"),
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
		out.Strategy.Interval30m = s
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
		"STRATEGY_INTERVAL":               cfg.Strategy.Interval30m,
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
