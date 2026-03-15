# 200 Sembol ile Bot Akışı

`.env` içinde örneğin **TRADE_SYMBOLS** ile 200 token/sembol verdiğinde bot **sırayla** şunları yapar.

---

## 1. Bot açılışı

- **Config** okunur: 200 sembol listesi, `MAX_OPEN_TRADES=5`, tarama aralığı (varsayılan **2 dakika**), strateji ayarları.
- **Testnet** açıksa: önce açık algo emirleri ve pozisyonlar temizlenmeye çalışılır (imza hatası olursa atlanır).
- Log’ta görürsün: `semboller=[SYM1, SYM2, ...]`, `max_açık_pozisyon=5`.

---

## 2. Her 2 dakikada bir: “Tarama turu” (scanner)

Bot bir **timer** ile her **2 dakikada** bir “tur” çalıştırır. Bir turda:

- **Sırayla** listeki her sembole gider (1, 2, 3, … 200).
- Her sembol için **önce** limit, **sonra** o sembole özel kontroller yapılır.

---

## 3. Her sembol için sırayla ne olur?

Sembol sırası **env’deki TRADE_SYMBOLS sırasıdır** (virgülle ayrılmış liste).

### Adım A: Limit kontrolü (her sembolün başında)

- `MAX_OPEN_TRADES > 0` ise:
  - DB’den **açık pozisyon sayısı** alınır.
  - Buna **“şu an işlem açmakta olan executor sayısı”** eklenir → toplam **dolu slot** sayısı.
  - Bu sayı **≥ MAX_OPEN_TRADES** ise:
    - Log: `max açık pozisyona ulaşıldı (açık=X + devam eden=Y, max=5), tarama durduruldu`
    - **Bu turda kalan semboller işlenmez** (sıradaki 6., 7., … 200. sembole bu turda hiç bakılmaz).
  - Yani aynı anda **en fazla 5** “açık pozisyon + açılışta olan” olur.

### Adım B: Bu sembol zaten “işlemde” mi?

- Bu sembol için **executor** zaten çalışıyorsa (pozisyon açılıyor veya açık pozisyon takip ediliyorsa):
  - Log: `SYMBOL atlandı (executor zaten çalışıyor)`
  - Bir şey yapılmaz, **sıradaki sembole** geçilir.

### Adım C: Bu sembolde DB’de açık pozisyon var mı?

- DB’de bu sembole ait **closed_at = NULL** bir kayıt varsa:
  - Log: `SYMBOL atlandı (açık pozisyon var, id=...)`
  - Yeni pozisyon açılmaz, **sıradaki sembole** geçilir.

### Adım D: Strateji kararı (Long / Short / Hold)

- **Strateji** çalıştırılır: bu sembol için sinyal istenir (ör. RSI vb.).
- Sonuç:
  - **HOLD** → Log: `SYMBOL sinyal=HOLD`, pozisyon açılmaz, **sıradaki sembole** geçilir.
  - **Hata** → Log + isteğe bağlı Telegram uyarısı, **sıradaki sembole** geçilir.

### Adım E: LONG veya SHORT sinyali + limit tekrar

- Sinyal **LONG** veya **SHORT** ise:
  - Limit **bir kez daha** kontrol edilir (açık + “açılışta olan” ≥ max ise):
    - Log: `SYMBOL sinyal=LONG/SHORT ATLANDI | max açık pozisyon (açık=X + devam=Y >= max=5)`
    - Bu sembol için executor **başlatılmaz**, **sıradaki sembole** geçilir.
  - Limit müsaitse:
    - Log: `SYMBOL sinyal=LONG/SHORT → executor başlatılıyor`
    - Bu sembol için **bir executor** başlatılır (arka planda goroutine) ve **sıradaki sembole** geçilir (beklemeden).

---

## 4. Executor ne yapar? (Sembol başına 1 tane)

- **Pozisyon açar:** Binance’ta market long/short.
- **SL/TP koyar:** Algo order ile stop loss ve take profit.
- **DB’ye yazar:** Açılan trade kaydı (trade_id, order_id, entry, sl, tp vb.).
- **Telegram:** “Pozisyon açıldı” mesajı (hata verirse sadece log’ta kalır; pozisyon yine açıktır).
- Sonra **sürekli döngüde** (ör. her 1 dakika):
  - Bu sembolde borsada pozisyon hâlâ açık mı diye bakar.
  - Kapandıysa (SL/TP veya manuel): DB’yi günceller, “Pozisyon kapandı” Telegram’ı, executor biter.

Yani **bir sembol için en fazla 1 açık pozisyon**; o kapanana kadar o sembole tekrar “yeni açılış” yapılmaz (Adım B ve C sayesinde).

---

## 5. 200 sembol + MAX_OPEN_TRADES=5 ile özet

| Ne olur? | Açıklama |
|----------|----------|
| **Sıra** | Her turda semboller **env sırasıyla** (1’den 200’e) işlenir. |
| **Limit** | Aynı anda **en fazla 5** açık pozisyon (+ açılışta olan). 5 doluysa tur erken biter, kalan 195 sembole bu turda hiç bakılmaz. |
| **Executor** | Sinyal alan ve limiti aşmayan semboller için **sırayla** executor başlar; executor’lar **aynı anda** çalışır (paralel). |
| **Tur süresi** | 200 sembol × (DB + strateji isteği) → bir tur birkaç dakika sürebilir. |
| **Tur sıklığı** | Varsayılan **2 dakikada bir** yeni tur. |

Örnek:

- Tur 1: Sembol 1–5 LONG/SHORT sinyali alıp limiti dolduruyor → 5 executor başlar, 6–200 atlanır.
- 2 dakika sonra Tur 2: 1–5’te zaten açık pozisyon/executor var → atlanır; 6–10’da sinyal + limit müsaitse yine en fazla 5 yeni açılış (toplam açık yine 5’i geçmez).
- Bir pozisyon kapanınca slot açılır; bir sonraki turda sıradaki sembollerden biri yeni pozisyon açabilir.

---

## 6. Dikkat etmen gerekenler

1. **200 sembol** → Her turda 200 kez strateji + DB sorgusu; tur süresi ve Binance/DB yükü artar.
2. **MAX_OPEN_TRADES=5** → En fazla 5 açık pozisyon; 6. sinyal bu turda “ATLANDI” olur.
3. **Sıra** → Önce gelen semboller (env’deki liste sırası) daha sık şans bulur; 201. sembol ancak öndekiler kapandıkça slot açılırsa işlenir.
4. **Telegram 401** → Sadece bildirim gönderilemez; **Binance’ta pozisyon açılmış ve DB’ye yazılmış olur**, executor da pozisyonu takip etmeye devam eder (Telegram’ı düzeltirsen bir sonraki mesajlar gelir).

Bu akış, env’e 200 token koyduğunda botun **sırayla ve limit dahilinde** ne işlem yaptığının özetidir.
