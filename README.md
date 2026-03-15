# Binance Bot

Binance Futures ile long/short işlem, stop loss / take profit ve Telegram bildirimi.  
Kod: **sade, SOLID, anlaşılır**. Strateji kurallarını sonradan .env veya kodla değiştirebilirsin.

## Yapı

```
binancebot/
├── config/             # Ayarlar (.env + Config)
├── internal/
│   ├── binance/        # Binance API (fiyat, kline, pozisyon, order)
│   ├── db/             # PostgreSQL (Store, trade kayıtları)
│   ├── strategy/       # Sinyal: Long / Short / Hold (örnek: RSI + hacim)
│   ├── risk/           # SL/TP fiyat hesaplama
│   ├── telegram/       # Bildirim
│   └── bot/            # Scanner + Executor (aşağıda)
├── cmd/bot/            # Bot çalıştırma
├── api/                # Dashboard API (Gin)
├── web/                # Dashboard arayüzü (tek sayfa)
└── .env.example
```

### Bot nasıl çalışıyor?

- **Scanner (cron)**: Her N dakikada bir tüm sembolleri (`TRADE_SYMBOLS` veya `TRADE_SYMBOL`) tarar. Koşul sağlayan ve o anda açık pozisyonu/executor’u olmayan semboller için bir **Executor** goroutine başlatır.
- **Executor (sembol başına goroutine)**: Sadece o sembolde pozisyon açar, SL/TP koyar, kapanana kadar periyodik kontrol eder. Pozisyon kapanınca DB günceller, Telegram’a bildirir ve **goroutine sonlanır**. Aynı sembole tekrar giriş ancak bir sonraki scanner turunda, koşul tekrar sağlanırsa yapılır.

## Strateji (örnek – sonradan değiştirilebilir)

Şu an **örnek kural** var: 30 dakikalık RSI + hacim.

- **Long**: 30m RSI &lt; 10 **ve** (hacim ≥ X USD **veya** son mumdan 3x fazla).
- **Short**: 30m RSI &gt; 90 **ve** aynı hacim koşulu.

Kuralları değiştirmek için:

1. **.env** ile: `STRATEGY_RSI_LOW`, `STRATEGY_RSI_HIGH`, `STRATEGY_MIN_VOLUME_USD`, `STRATEGY_VOLUME_RATIO` (bkz. `.env.example`).
2. **Kod** ile: `internal/strategy/example.go` içindeki `Decide` mantığını düzenle veya yeni bir `Strategy` implementasyonu yaz.

## Gereksinimler

- Go 1.21+
- PostgreSQL
- .env (`.env.example` → `.env`)

## Veritabanı

- `DATABASE_URL` doluysa tek satırda kullanılır.
- Yoksa: `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`.

İlk çalıştırmada `trades` tablosu otomatik oluşturulur.

## Çalıştırma

```bash
cp .env.example .env
# .env içini doldur (Binance, PostgreSQL, Telegram)

go mod tidy
```

**Bot** (strateji döngüsü + işlem + Telegram):

```bash
go run ./cmd/bot
```

**Dashboard API + arayüz** (işlem listesi, toplam PnL, açık pozisyon):

```bash
go run ./api
# Tarayıcı: http://localhost:8080
```

İstersen bot ve API’yi aynı anda çalıştır (iki terminal).
