# Test Etmeye Başlama

Aşağıdaki sırayla ilerle. Önce **gerçek para kullanmadan** Binance Futures **Testnet** ile test et.

---

## 1. PostgreSQL

- PostgreSQL kurulu ve çalışıyor olmalı.
- Veritabanı ve kullanıcı oluştur:

```bash
# Örnek (psql ile)
createuser -P binancebot   # şifre sorar
createdb -O binancebot binancebot
```

- Veya tek satır URL kullanacaksan: `DATABASE_URL=postgres://binancebot:SIFRE@localhost:5432/binancebot`

---

## 2. Binance Futures Testnet

1. https://testnet.binancefuture.com adresine git.
2. Giriş yap (Binance hesabınla veya testnet için e-posta ile).
3. **API Management** → yeni API key oluştur.
4. **API Key** ve **Secret**’ı kopyala (secret sadece bir kez gösterilir).

`.env` içine:

```
BINANCE_API_KEY=...
BINANCE_SECRET_KEY=...
BINANCE_FUTURES_TESTNET=true
```

---

## 3. Telegram (opsiyonel ama önerilir)

1. Telegram’da [@BotFather](https://t.me/BotFather) → `/newbot` → bot adı ver, token al.
2. Botuna bir mesaj at (örn. "merhaba").
3. Chat ID almak için: `https://api.telegram.org/bot<TOKEN>/getUpdates` tarayıcıda aç; `"chat":{"id": 123456789}` değerini kopyala.

`.env`:

```
TELEGRAM_BOT_TOKEN=...
TELEGRAM_CHAT_ID=123456789
```

---

## 4. .env dosyası

```bash
cp .env.example .env
```

Tüm alanları doldur (en azından Binance + PostgreSQL). Telegram boş bırakılırsa bildirim gitmez, bot yine çalışır.

Örnek minimal `.env`:

```
BINANCE_API_KEY=testnet_key
BINANCE_SECRET_KEY=testnet_secret
BINANCE_FUTURES_TESTNET=true

POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_USER=binancebot
POSTGRES_PASSWORD=your_password
POSTGRES_DB=binancebot

TELEGRAM_BOT_TOKEN=
TELEGRAM_CHAT_ID=

TRADE_SYMBOL=BTCUSDT
STOP_LOSS_PERCENT=2
TAKE_PROFIT_PERCENT=3
POSITION_SIZE_USD=50
```

---

## 5. Tek sembol ile test (SL/TP Algo vb.)

Sadece bir çiftte denemek için `.env` içinde:

```
TRADE_SYMBOLS=1000PEPEUSDT
```

(veya `BTCUSDT`, `ADAUSDT` vb.) Böylece scanner yalnızca bu sembolü tarar; loglar karışmaz, imza/Algo hatalarını net görürsün.

---

## 6. Botu çalıştırma

```bash
go run ./cmd/bot
```

- Scanner her 2 dakikada sembolleri tarayacak.
- Strateji koşulu sağlanırsa (örnek: RSI + hacim) o sembolde pozisyon açılır, SL/TP konur.
- Testnet’te gerçek para yok; pozisyonlar test bakiyesi ile açılır.

Durdurmak: **Ctrl+C**.

Tüm işlemler loglanır (`[bot]`, `[scanner]`, `[executor]`, `[trade]`, `[binance]`, `[db]`). Dosyaya kaydetmek için:

```bash
go run ./cmd/bot 2>&1 | tee bot.log
# veya sadece dosyaya:
go run ./cmd/bot >> bot.log 2>&1
```

---

## 7. Dashboard (işlemleri görmek)

Bot zaten çalışırken dashboard aynı process’te açık. Tarayıcıda http://localhost:8080 (PORT env’e göre). Açık pozisyonlar ve toplam PnL orada.

---

## Sık karşılaşılanlar

| Sorun | Kontrol |
|-------|--------|
| PostgreSQL bağlanamıyor | Host/port/user/password, `pg_isready -h localhost -p 5432` |
| Binance 401 / -2015 | Testnet key kullanıyor musun? `BINANCE_FUTURES_TESTNET=true` |
| Binance -404 / connection | Testnet URL’leri açık mı, firewall/VPN |
| Telegram gitmiyor | Token ve Chat ID doğru mu? Bot’a en az bir mesaj attın mı? |
| Pozisyon açılmıyor | Strateji koşulu (örn. RSI 10’un altı) nadiren sağlanır; test için `STRATEGY_RSI_LOW=50` gibi gevşetebilirsin. |

---

## Testnet sonrası (gerçek borsa)

- `BINANCE_FUTURES_TESTNET=false`
- Gerçek Binance Futures API key (Futures hesabında oluştur).
- Önce çok küçük `POSITION_SIZE_USD` ile dene.
