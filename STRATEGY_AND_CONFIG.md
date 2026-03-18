# Bot Sistemi – Baştan Sona Rehber

Bu dokümanda botun ne yaptığı, hangi ayarın neyi etkilediği ve stratejinin adım adım nasıl çalıştığı anlatılıyor. Hiç bilmeyen biri için yazıldı.

---

## 1. Bot Ne Yapıyor?

Bot **Binance USDT-M Futures** piyasasında otomatik işlem açar ve kapatır:

1. **Tarama:** Belirlediğin coin listesini (örn. BTCUSDT, ETHUSDT) periyodik olarak tarar.
2. **Sinyal:** Her coin için strateji “Long”, “Short” veya “işlem yok (Hold)” der.
3. **Açılış:** Long/Short sinyali gelirse o coin’de market emirle pozisyon açar, Stop Loss (SL) ve Take Profit (TP) koyar.
4. **Kapanış:** Pozisyon SL veya TP’ye gelince (veya manuel kapatılınca) borsa kapatır; bot bunu algılar, veritabanını günceller ve isteğe bağlı Telegram’a yazar.

Tüm ayarlar **.env** dosyasından okunur. Bazıları ayrıca **dashboard (UI) üzerinden** değiştirilebilir; UI’daki değerler veritabanında `runtime_config` tablosunda tutulur ve **env’i geçersiz kılar** (redeploy gerekmez).

---

## 2. Genel Akış (Kısaca)

```
Başlatma
  → Bakiye / testnet temizliği / Binance–DB senkronu
  → Her X dakikada bir “tarama turu”:
       → Sırayla her sembol:
            → Açık pozisyon var mı? Varsa atla.
            → 24s hacim filtresi (MIN_24H_VOLUME_USD) geçiyor mu?
            → Strateji: Long / Short / Hold
            → Long veya Short ise: pozisyon aç, SL/TP koy, DB’ye kaydet
  → Açık pozisyonlar için ayrı “executor” süreçleri:
       → Borsada pozisyon hâlâ açık mı diye periyodik kontrol
       → Kapandıysa: çıkış fiyatı + realized PnL’yi al, DB’yi güncelle
```

Tarama aralığı, sembol listesi, SL/TP, strateji parametreleri hep config’ten (env + isteğe bağlı runtime) gelir.

---

## 3. Tüm Ayar Değişkenleri (Env ve Ne İşe Yaradıkları)

### 3.1 Binance ve Ortam

| Değişken | Açıklama |
|----------|----------|
| **BINANCE_API_KEY** | Binance Futures API anahtarı. |
| **BINANCE_SECRET_KEY** | Binance Futures API gizli anahtarı. |
| **BINANCE_FUTURES_TESTNET** | `true` = testnet, `false` = canlı piyasa. Testnet’te gerçek para yok; kapanışta realized PnL borsadan gelmeyebildiği için bot kendi hesaplar. |
| **BINANCE_TESTNET_CLEANUP_ON_START** | `true` ise (ve testnet açıksa) bot başlarken açık pozisyonları kapatır ve trades tablosunu temizler. Varsayılan: `false`. |

### 3.2 Veritabanı

| Değişken | Açıklama |
|----------|----------|
| **DATABASE_URL** | Tek satırda PostgreSQL bağlantı adresi (tercih edilir). |
| **POSTGRES_HOST**, **POSTGRES_PORT**, **POSTGRES_USER**, **POSTGRES_PASSWORD**, **POSTGRES_DB** | DATABASE_URL yoksa bunlarla bağlantı kurulur. |

### 3.3 Telegram (İsteğe Bağlı)

| Değişken | Açıklama |
|----------|----------|
| **TELEGRAM_BOT_TOKEN** | Bot token. Boş bırakırsan bildirim gönderilmez. |
| **TELEGRAM_CHAT_ID** | Mesajların gideceği sohbet ID. |

### 3.4 Bot / İşlem Genel

| Değişken | Açıklama |
|----------|----------|
| **BOT_ENABLED** | `true` = tarama + işlem açar. `false` = sadece API/dashboard çalışır, pozisyon açılmaz. |
| **TRADE_SYMBOL** | Tek sembol kullanıyorsan (örn. `BTCUSDT`). |
| **TRADE_SYMBOLS** | Virgülle ayrılmış liste (örn. `BTCUSDT,ETHUSDT,SOLUSDT`). Tarama bunların hepsinde yapılır. |
| **STOP_LOSS_PERCENT** | Stop loss, **marj üzerinden** yüzde. Örn. 3 = marjın %3’ü kadar zararda kes. SL fiyatı, kaldıraca göre hesaplanır (fiyat hareketi % = marj riski % / kaldıraç). |
| **TAKE_PROFIT_PERCENT** | Take profit, **marj üzerinden** yüzde. Örn. 6 = marjın %6’sı kadar kârda kapat. TP fiyatı yine kaldıraca göre hesaplanır. |
| **POSITION_SIZE_USD** | Pozisyon başına **marj** (USDT). Sadece **RISK_PER_TRADE=0** iken kullanılır. |
| **LEVERAGE** | Kaldıraç (1–125). Marj × kaldıraç = pozisyon büyüklüğü (notional). |
| **RISK_PER_TRADE** | Her işlemde bakiyenin yüzde kaçı riske atılacak. Örn. 1 = %1. 0 ise **POSITION_SIZE_USD** kullanılır. Marj formülü: `bakiye × (RISK_PER_TRADE/100) / (STOP_LOSS_PERCENT/100)` → SL’de kayıp ≈ bakiyenin RISK_PER_TRADE %’si. |
| **MIN_24H_VOLUME_USD** | Sadece **son 24 saatin işlem hacmi (USDT)** bu değerin üstünde olan coinler taranır. 0 = filtre yok. Örn. 100000000 = 100M USDT. |
| **MAX_OPEN_TRADES** | Aynı anda en fazla kaç açık pozisyon olabilir. 0 = sınırsız. |
| **MAX_LOSSES_IN_12H** | **Aynı sembolde** son 12 saatte bu sayıdan fazla zararla kapanan işlem varsa o sembole tekrar pozisyon açılmaz. 0 = kapatılmış. |
| **COIN_COOLDOWN_MIN** | Bir sembol kapandıktan sonra bu **dakika** boyunca tekrar işlem açılmaz. 0 = kapalı. Varsayılan 30. DB runtime (UI Ayarlar) ile değiştirilebilir. |
| **MAX_TRADE_DURATION_MIN** | Açık pozisyon **bu dakikayı aşarsa** tarama başında zorla market kapatılır. 0 = kapalı. Varsayılan 120. DB runtime (UI Ayarlar) ile değiştirilebilir. |
| **MIN_BALANCE_SHUTDOWN** | Futures bakiyesi bu USDT’nin altına inerse bot **process’i kapatır** (exit). 0 = kapatma yok. |
| **BOT_INSTANCE_ID** | Hangi makine/süreç çalışıyor (log ve DB’de görünür). Boşsa hostname kullanılır. |
| **EXECUTOR_POLL_INTERVAL_SEC** | Açık pozisyonlar kaç saniyede bir “borsada hâlâ açık mı / kapandı mı” diye kontrol edilir. 0 = varsayılan (60 sn). |

### 3.5 Strateji – Zaman Dilimi ve Trend (EMA)

| Değişken | Açıklama |
|----------|----------|
| **STRATEGY_INTERVAL** | Mum (kline) aralığı: `1m`, `3m`, `5m`, `15m`, `30m` vb. Strateji bu aralıktaki mumlara bakarak karar verir. 5m dengeli, 15m daha sakin. |
| **EMA_FAST** | Rezerve (örnek stratejide kullanılmıyor). |
| **EMA_SLOW** | >0 ise rejim: fiyat > EMA → sadece Long; fiyat ≤ EMA → sadece Short. |

### 3.6 Strateji – RSI (trend yönü) ve Hacim

| Değişken | Açıklama |
|----------|----------|
| **STRATEGY_RSI_PERIOD** | RSI hesaplama periyodu (örn. 7). |
| **STRATEGY_RSI_LOW** | **Short** adayı: RSI **<** bu (zayıflık, varsayılan 45). |
| **STRATEGY_RSI_HIGH** | **Long** adayı: RSI **>** bu (momentum, varsayılan 55). **HIGH > LOW** olmalı (orta bantta sinyal yok). |
| **STRATEGY_RSI_SLOPE_ENABLE** | `true` (varsayılan): Long için RSI önceki muma göre **artmalı**; Short için **düşmeli** (range/fake breakout azaltır). `false` ile kapatılır. |
| **STRATEGY_MIN_VOLUME_USD** | Son mumun quote (USDT) hacmi en az bu kadar olmalı (ek filtre). |
| **STRATEGY_VOLUME_RATIO** | Son mumun hacmi, bir önceki mumun en az bu katı olabilir (ek filtre). |
| **STRATEGY_VOLUME_AVG_PERIOD** | Ortalama hacim için geriye kaç mum kullanılacak (örn. 20). |
| **STRATEGY_VOLUME_MIN_RATIO_AVG** | “Volume spike”: Son mumun hacmi ≥ (son N mumun ortalaması) × bu oran. Örn. 1.8 = ortalamanın 1.8 katı. |

### 3.7 Strateji – Orderbook, Volatilite, Liquidation

| Değişken | Açıklama |
|----------|----------|
| **ORDERBOOK_IMBALANCE_ENABLE** | `true` ise orderbook dengesi de kontrol edilir. |
| **ORDERBOOK_IMBALANCE_LONG_MIN** | Long için: bid_vol / (bid_vol + ask_vol) ≥ bu değer (örn. 0.65). |
| **ORDERBOOK_IMBALANCE_SHORT_MAX** | Short için: imbalance ≤ bu değer (örn. 0.35). |
| **ORDERBOOK_DEPTH_LIMIT** | Orderbook’ta kaç seviye kullanılacak (5, 10, 20, 50, 100). |
| **VOLATILITY_EXPANSION_ENABLE** | `true` ise volatilite genişlemesi (ATR) şartı aranır. |
| **ATR_PERIOD** | ATR periyodu (örn. 14). |
| **ATR_EXPANSION_RATIO** | “Volatilite genişlemesi”: Güncel ATR ≥ ortalama ATR × bu oran (örn. 1.4). |
| **LIQUIDATION_CASCADE_ENABLE** | `true` planlanan: Son N saniyede belli USD üstü liquidation olunca sinyal. Şu an kodda TODO. |
| **LIQUIDATION_WINDOW_SEC** | Liquidation için “son kaç saniye” (örn. 10). |
| **LIQUIDATION_MIN_USD** | En az bu kadar USD liquidation (örn. 500000). |

---

## 4. Veritabanında “Runtime” Ayar (UI’dan Değişenler)

Dashboard’daki **Ayarlar** panelinden değiştirilen değerler **PostgreSQL**’de `runtime_config` tablosunda **key–value** olarak saklanır. Bot her turda önce bu tabloyu okur; bir anahtar varsa **env yerine bu değer** kullanılır. Yani deploy etmeden sadece UI’dan güncelleyebilirsin.

**UI’dan değiştirilebilir anahtarlar** (çoğu env ile aynı isim):

- **MAX_DIRECTION_WEIGHT** — **Sadece DB / UI** (`.env` yok). `0` veya boş = kapalı. Örn. `0.6` ve **MAX_OPEN_TRADES=10** iken tek yönde en fazla **6** açık LONG veya **6** açık SHORT (açılış sırasında bekleyen executor’lar da sayılır). `MAX_OPEN_TRADES` 0 iken bu limit uygulanmaz.

- **STOP_LOSS_PERCENT**
- **TAKE_PROFIT_PERCENT**
- **STRATEGY_INTERVAL**
- **STRATEGY_RSI_PERIOD**
- **STRATEGY_RSI_LOW**
- **STRATEGY_RSI_HIGH**
- **STRATEGY_RSI_SLOPE_ENABLE**
- **STRATEGY_MIN_VOLUME_USD**
- **STRATEGY_VOLUME_RATIO**
- **STRATEGY_VOLUME_AVG_PERIOD**
- **STRATEGY_VOLUME_MIN_RATIO_AVG**

Özet: SL/TP + mum aralığı + RSI + hacim parametreleri hem env’den hem DB’den (UI) gelebilir; DB’de varsa o kullanılır.

---

## 5. Strateji Adım Adım (Decide)

Her tarama turunda, her sembol için strateji şu sırayla çalışır. **Tek bir koşul bile sağlanmazsa** sinyal **Hold** olur; Long veya Short için tüm ilgili adımlar uyumlu olmalı.

### 5.1 Veri

- **Interval** (STRATEGY_INTERVAL) ile son **N** mum çekilir. N, RSI periyodu, hacim ortalaması, ATR ve EMA200 için yeterli olacak şekilde büyütülür (en az 200+ mum gerekebilir).
- Bu mumlardan: kapanış fiyatları, quote hacimler, high/low (ATR için) kullanılır.

### 5.2 RSI + Hacim (momentum / zayıflık + volume)

- **RSI** (Wilder) son mum için hesaplanır.
- **Long adayı:** RSI **>** **STRATEGY_RSI_HIGH** (varsayılan 55) ve hacim OK; ayrıca **STRATEGY_RSI_SLOPE_ENABLE** açıksa güncel RSI **>** bir önceki mumun RSI’sı (eğim yukarı).
- **Short adayı:** RSI **<** **STRATEGY_RSI_LOW** (varsayılan 45) ve hacim OK; slope açıksa güncel RSI **<** önceki mum RSI (eğim aşağı).
- **LOW < RSI < HIGH** → Hold (nötr bant).
- Hacim şartları (aynı):
  - Son mum USDT hacmi ≥ **STRATEGY_MIN_VOLUME_USD** veya önceki mumun **STRATEGY_VOLUME_RATIO** katı.
  - İsteğe bağlı volume spike: ortalama × **STRATEGY_VOLUME_MIN_RATIO_AVG**.

### 5.3 EMA rejim filtresi

- **EMA_SLOW** > 0 ise: son kapanış **>** EMA(EMA_SLOW) → yalnızca Long geçer; **≤** EMA → yalnızca Short. Ters yöndeki aday Hold olur.

### 5.4 Orderbook İmbalance (İsteğe Bağlı)

- **ORDERBOOK_IMBALANCE_ENABLE** = true ise:
  - Binance’tan orderbook (depth) alınır, **ORDERBOOK_DEPTH_LIMIT** seviye kullanılır.
  - İmbalance = bid tarafı quote hacmi / (bid + ask quote hacmi).
  - **Long** için: imbalance ≥ **ORDERBOOK_IMBALANCE_LONG_MIN** (örn. 0.65) olmalı; değilse Hold.
  - **Short** için: imbalance ≤ **ORDERBOOK_IMBALANCE_SHORT_MAX** (örn. 0.35) olmalı; değilse Hold.

### 5.5 Volatilite Genişlemesi (İsteğe Bağlı)

- **VOLATILITY_EXPANSION_ENABLE** = true ise:
  - **ATR(ATR_PERIOD)** son mum için ve son birkaç mumun ortalaması hesaplanır.
  - “Genişleme”: son ATR ≥ ortalama ATR × **ATR_EXPANSION_RATIO** (örn. 1.4).
  - Bu sağlanmıyorsa sinyal Hold’a çevrilir.

### 5.6 Liquidation (Planlanan)

- **LIQUIDATION_CASCADE_ENABLE** = true olsa bile şu an kodda sadece TODO var; WebSocket ile liquidation verisi entegre edildikten sonra “son LIQUIDATION_WINDOW_SEC saniyede en az LIQUIDATION_MIN_USD liquidation” gibi bir filtre eklenecek.

### 5.7 Sonuç

- Tüm filtreler geçildiyse strateji **Long** veya **Short** döner; aksi halde **Hold**. Bot sadece Long/Short’ta pozisyon açar.

---

## 6. Pozisyon Büyüklüğü (Marj) Nasıl Belirlenir?

- **RISK_PER_TRADE > 0** ise:
  - Marj (USDT) = `Bakiye × (RISK_PER_TRADE / 100) / (STOP_LOSS_PERCENT / 100)`.
  - Böylece pozisyon SL’e gelince kayıp ≈ bakiyenin **RISK_PER_TRADE** yüzdesi olur.
- **RISK_PER_TRADE = 0** ise:
  - Marj = **POSITION_SIZE_USD**.
- İşlem büyüklüğü (notional) = Marj × **LEVERAGE**. Bot bu notional’a göre miktar (quantity) hesaplar.

---

## 7. Özet Tablo: Hangi Değişken Neyi Etkiler?

| Ne yapmak istiyorsun? | Hangi değişken(ler) |
|------------------------|----------------------|
| Hangi coinler taranacak | TRADE_SYMBOLS, MIN_24H_VOLUME_USD |
| Ne kadar risk / pozisyon | RISK_PER_TRADE, STOP_LOSS_PERCENT, POSITION_SIZE_USD, LEVERAGE |
| SL/TP seviyeleri | STOP_LOSS_PERCENT, TAKE_PROFIT_PERCENT |
| Mum zaman dilimi | STRATEGY_INTERVAL |
| Trend (yukarı/aşağı) filtresi | EMA_FAST, EMA_SLOW |
| RSI bantları + eğim | STRATEGY_RSI_PERIOD, STRATEGY_RSI_LOW, STRATEGY_RSI_HIGH, STRATEGY_RSI_SLOPE_ENABLE |
| Hacim / volume spike | STRATEGY_MIN_VOLUME_USD, STRATEGY_VOLUME_RATIO, STRATEGY_VOLUME_AVG_PERIOD, STRATEGY_VOLUME_MIN_RATIO_AVG |
| Orderbook onayı | ORDERBOOK_IMBALANCE_ENABLE, ORDERBOOK_IMBALANCE_LONG_MIN, ORDERBOOK_IMBALANCE_SHORT_MAX, ORDERBOOK_DEPTH_LIMIT |
| Volatilite filtresi | VOLATILITY_EXPANSION_ENABLE, ATR_PERIOD, ATR_EXPANSION_RATIO |
| Aynı anda max pozisyon | MAX_OPEN_TRADES |
| Çok zarar sonrası durma | MAX_LOSSES_IN_12H |
| Bakiye düşünce kapanma | MIN_BALANCE_SHUTDOWN |
| UI’dan deploy etmeden değişenler | runtime_config’teki 10 anahtar (yukarıda listelendi) |

Bu yapı ile sistem: **trend (EMA) + RSI pullback + hacim + isteğe bağlı orderbook + isteğe bağlı volatilite** bir arada çalışacak şekilde tasarlanmış durumda; tüm mantık `internal/strategy/example.go` içindeki `Decide` fonksiyonunda toplanıyor.
