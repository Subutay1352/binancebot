package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config uygulama ayarları. .env veya ortam değişkenlerinden okunur.
type Config struct {
	Binance  BinanceConfig
	Postgres PostgresConfig
	Telegram TelegramConfig
	Trade    TradeConfig
	Strategy StrategyConfig
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
	PositionSizeUSD   float64  // Pozisyon büyüklüğü USDT cinsinden
	MaxOpenTrades     int      // Aynı anda en fazla bu kadar açık pozisyon (0 = sınırsız)
	InstanceID        string   // Hangi makine/süreç (BOT_INSTANCE_ID veya hostname); DB'de kim yazdı görmek için
	ExecutorPollSec   int      // SL/TP kapanış kontrolü kaç saniyede bir (EXECUTOR_POLL_INTERVAL_SEC, 0=varsayılan 60)
}

// StrategyConfig örnek strateji için. Kuralları sonradan .env ile değiştirebilirsin.
type StrategyConfig struct {
	Interval30m       string  // "30m"
	RSIPeriod        int     // 14
	RSIThresholdLow  float64 // Long için RSI bu değerin altındaysa (örn 10)
	RSIThresholdHigh float64 // Short için RSI bu değerin üstündeyse (örn 90)
	MinVolumeUSD     float64 // Hacim en az bu kadar USD (örn 1_000_000)
	VolumeRatioVsPrev float64 // Son mumdan en az bu katı (örn 3)
}

// Load .env dosyasını yükler ve Config döner.
func Load() (*Config, error) {
	_ = godotenv.Load()

	return &Config{
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
			Interval30m:        env("STRATEGY_INTERVAL", "30m"),
			RSIPeriod:          envInt("STRATEGY_RSI_PERIOD", 14),
			RSIThresholdLow:    envFloat("STRATEGY_RSI_LOW", 10),
			RSIThresholdHigh:   envFloat("STRATEGY_RSI_HIGH", 90),
			MinVolumeUSD:       envFloat("STRATEGY_MIN_VOLUME_USD", 500_000),
			VolumeRatioVsPrev:  envFloat("STRATEGY_VOLUME_RATIO", 3),
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
		MaxOpenTrades:     envInt("MAX_OPEN_TRADES", 5),
		InstanceID:        os.Getenv("BOT_INSTANCE_ID"),
		ExecutorPollSec:   envInt("EXECUTOR_POLL_INTERVAL_SEC", 10),
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
