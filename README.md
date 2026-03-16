# Binance Futures Bot

Binance USDT-M vadeli işlemlerde long/short açan, stop loss ve take profit koyan bir bot. İstersen Telegram’a bildirim de atıyor.

## Ne yapıyor?

Bot periyodik olarak belirlediğin sembolleri tarıyor. Strateji koşulu sağlanan bir coin’de açık pozisyon yoksa pozisyon açıyor, SL/TP koyuyor. Pozisyon kapanınca (SL/TP veya manuel) veritabanını güncelliyor. Dashboard’dan açık pozisyonları ve geçmiş işlemleri görebilirsin.

Strateji trend (EMA 50/200) + RSI pullback + hacim + isteğe bağlı orderbook ve volatilite filtreleriyle çalışıyor. Tüm eşikler `.env` üzerinden (ve bir kısmı dashboard’dan) ayarlanıyor. **Hangi env/db değişkeninin neyi değiştirdiği ve stratejinin adım adım nasıl işlediği** için `STRATEGY_AND_CONFIG.md` dosyasına bak.

## Gereksinimler

- Go 1.21+
- PostgreSQL
- Binance Futures API key (gerçek veya testnet)

## Kurulum

```bash
cp .env.example .env
```

`.env` içinde en az şunları doldur: Binance API key/secret, veritabanı bilgisi. Telegram istemiyorsan `TELEGRAM_BOT_TOKEN` ve `TELEGRAM_CHAT_ID` boş bırak, mesaj atmaz.

Veritabanı için ya tek satırda `DATABASE_URL` verirsin ya da `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` ayrı ayrı. İlk çalıştırmada `trades` tablosu kendisi oluşuyor.

```bash
go mod tidy
```

## Çalıştırma

```bash
go run ./cmd/bot
```


Bot çalışırken dashboard aynı process'te açılır. Tarayıcıda `http://localhost:8080` (PORT env ile değiştirilebilir). Açık pozisyonlar ve PnL oradan takip edilir.

Testnet kullanacaksan `.env`’de `BINANCE_FUTURES_TESTNET=true` yap; API key’leri de testnet.binancefuture.com üzerinden al.

## Proje yapısı

- `cmd/bot` — Bot’un ana giriş noktası
- `internal/bot` — Tarama döngüsü, executor’lar, pozisyon açma/kapatma
- `internal/strategy` — Sinyal mantığı (örnek: RSI + hacim + MA)
- `internal/binance` — Binance API çağrıları
- `internal/db` — Trade kayıtları, açık pozisyon sorguları
- `internal/risk` — SL/TP fiyat hesaplama
- `internal/telegram` — Bildirim (token/chatID boşsa devre dışı)
- `internal/apiserver` — Dashboard (Gin, tek sayfa; bot ile aynı process’te)

Stratejiyi değiştirmek için `internal/strategy/example.go` içindeki `Decide` fonksiyonunu veya `.env`’deki `STRATEGY_*` değişkenlerini kullan.
