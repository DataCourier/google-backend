# Stock Widget Backend — Design Doc

## Goal

Android widget showing current stock/crypto prices + distance from ATH. Backend fetches prices from Finnhub, writes static JSON files to GCS (one per ticker). Anonymous daily heartbeat counting drives fetch prioritization.

---

## Requirements

### Functional
- Fetch current price for watched symbols on a schedule (~10 min during market hours)
- Maintain ATH (all-time high) per symbol, updated as new highs are observed
- Serve prices as **one GCS file per ticker** — widget fetches only what it needs
- Provide a **symbol index file** — full universe of available tickers for the app picker
- Anonymous daily subscriber counting per ticker (no device IDs)
- Prioritize fetching tickers with more subscribers
- Skip stock fetches outside market hours; crypto is 24/7

### Non-Functional
- Finnhub free tier: 60 calls/min → ~600 per 10-min cycle
- Delayed prices are fine
- Must be cheap to run ($0/mo at small scale)
- Privacy: zero PII, just anonymous counters

---

## Finnhub Free Tier

- **Rate limit**: 60 calls/min
- **Quote endpoint**: `GET /quote?symbol=AAPL` → `{ c: 150.25, h: 151, l: 149, o: 150, pc: 148, t: 1704067200 }`
  - `c` = current, `h` = high, `l` = low, `o` = open, `pc` = previous close, `t` = timestamp
- **No bulk endpoint** on free tier — one symbol per request
- **Crypto**: prefix with `BINANCE:` e.g. `BINANCE:BTCUSDT`
- **Metrics**: `GET /stock/metric?symbol=AAPL&metric=all` — includes 52-week high (useful for ATH seed)
- **Symbol search**: `GET /search?q=apple` — for building the index

---

## Data Model

### 1. Price Files (GCS — one per ticker)

Path: `prices/{SYMBOL}.json`

```json
{
  "symbol": "AAPL",
  "price": 150.25,
  "high": 151.00,
  "low": 149.00,
  "open": 150.00,
  "previous_close": 148.00,
  "ath": 199.62,
  "ath_date": "2024-01-15",
  "off_ath_pct": -24.73,
  "fetched_at": 1704067200000,
  "earnings": {
    "date": "2026-04-30",
    "days_away": 74,
    "last_eps_actual": 2.84,
    "last_eps_estimate": 2.73,
    "last_eps_surprise_pct": 4.03
  },
  "fundamentals": {
    "pe_ttm": 31.88,
    "ps_ttm": 8.62,
    "pb_quarterly": 45.87,
    "forward_pe": 30.27,
    "peg_ttm": 1.29,
    "dividend_yield_ttm": 0.41,
    "gross_margin_ttm": 47.33,
    "net_margin_ttm": 27.04,
    "roe_ttm": 159.94,
    "debt_equity": 1.03,
    "eps_ttm": 7.90,
    "revenue_growth_yoy": 10.07,
    "eps_growth_yoy": 25.65,
    "week52_high": 288.62,
    "week52_low": 169.21
  },
  "fundamentals_updated_at": 1704067200000
}
```

Widget reads `prices/AAPL.json` directly from GCS. 4 tickers = 4 file reads. Simple.

Earnings and fundamentals are refreshed daily (not per-minute). The `week52_high` from fundamentals seeds ATH on first watch.

### 2. Symbol Index File (GCS — single file)

Path: `symbols/index.json`

The full universe of available tickers. App downloads this for the symbol picker. Updated infrequently (daily or on-demand).

```json
{
  "updated_at": 1704067200000,
  "symbols": [
    { "symbol": "AAPL", "name": "Apple Inc", "type": "stock", "exchange": "US" },
    { "symbol": "BINANCE:BTCUSDT", "name": "Bitcoin / USDT", "type": "crypto", "exchange": "BINANCE" }
  ]
}
```

Source: Finnhub `GET /stock/symbol?exchange=US` + `GET /crypto/symbol?exchange=binance`.
This is a large list (~10k US symbols). Could split by exchange or paginate if needed, or have the app search via API instead of downloading the full list.

### 3. Symbol Registry (Firestore)

Collection: `watched_symbols`
Document ID: symbol (e.g. `AAPL`)

```json
{
  "symbol": "AAPL",
  "ath": 199.62,
  "ath_date": "2024-01-15",
  "subscribers_today": 342,
  "last_fetched_at": 1704067200000
}
```

- `subscribers_today` — daily counter, reset at midnight UTC
- `ath` / `ath_date` — updated whenever fetched price > current ATH

Only symbols with `subscribers_today > 0` (or recently active) get fetched.

### 4. Daily Subscriber Counting

**How it works:**

1. Widget starts up or detects stale price file → calls `POST /heartbeat` once per day
2. Body: `{ "symbols": ["AAPL", "TSLA", "BINANCE:BTCUSDT"] }`
3. Backend: for each symbol, increment `subscribers_today` counter on the Firestore doc
4. Counter resets daily (midnight UTC cron job zeros them all out)

**Staleness-triggered refresh:**
- Widget reads `prices/AAPL.json` from GCS
- If `fetched_at` is older than threshold (e.g. 20 min during market hours), widget also calls `POST /heartbeat` as a signal that this ticker needs attention
- This naturally keeps active tickers refreshed and lets unused ones go stale

**No device IDs, no dedup needed.** The counter is approximate — if a device heartbeats twice in a day, the count is slightly inflated but the relative ranking stays correct. Client enforces once-per-day via local `SharedPreferences` timestamp.

---

## Architecture

```
┌─────────────┐                              ┌──────────────┐
│ Android     │  GET prices/AAPL.json         │              │
│ Widget      │ ◄─────────────────────────── │  GCS Bucket  │
│             │  (direct, no API)             │  prices/     │
│             │                               │  symbols/    │
│             │  POST /heartbeat              └──────────────┘
│             │ ─────────────────────────────►┌──────────────┐
└─────────────┘                               │ Cloud Run    │
                                              │ game-service │
                                              └──────┬───────┘
                                                     │
                                        Cloud Scheduler (every 10 min)
                                                     │
                                                     ▼
                                              ┌──────────────┐
                                              │ Fetch Job    │
                                              └──────┬───────┘
                                                     │
                                   ┌─────────────────┼─────────────────┐
                                   ▼                 ▼                 ▼
                              Finnhub API      Firestore           GCS Bucket
                              (quote)          (watched_symbols)   (prices/*.json)
```

---

## Fetch Job (Cloud Scheduler → Cloud Run, every 1 min)

Runs every minute. Budget: **50 symbols per tick** (headroom under 60/min Finnhub limit).

### Freshness Tiers

| Subscribers | Target Freshness | Rationale |
|-------------|-----------------|-----------|
| 1-2         | 15 min          | Long tail, no rush |
| 3-9         | 8 min           | Moderate interest |
| 10+         | 4 min           | Hot tickers (~2 min avg staleness) |

At 50 fetches/min, top 200 tickers refresh every 4 min comfortably.

### Priority Algorithm

```
for each symbol where subscribers_today > 0:
    target = target_freshness(subscribers_today)  // 15m, 5m, or 2m
    staleness = now - last_fetched_at
    priority = staleness / target                 // >1.0 means overdue

sort by priority DESC
take top 50

// Filter by market hours:
//   - If market closed: skip stock symbols, only fetch crypto
//   - If market open: fetch both
```

A 10-subscriber ticker fetched 2.5 min ago (priority 1.25) beats a 1-subscriber ticker fetched 14 min ago (priority 0.93). Popular stuff stays fresh. Long tail never starves out hot tickers but still gets serviced.

### Per-Tick Execution

```
1. Read all watched_symbols from Firestore (cache in memory if Cloud Run instance stays warm)
2. Compute priority for each, filter by market hours
3. Take top 50
4. For each symbol:
   a. GET /quote?symbol={SYMBOL} from Finnhub
   b. If price > ath → update ath + ath_date in Firestore
   c. Compute off_ath_pct
   d. Write prices/{SYMBOL}.json to GCS
   e. Update last_fetched_at in Firestore
```

### Capacity

- 50 fetches/min = **200 hot tickers on a 4-min cycle** (avg 2 min stale)
- Long tail fills in the gaps when hot tickers are still fresh
- No batch API on Finnhub free tier — 1 request = 1 ticker
- Plenty of headroom for launch; if we outgrow it, add a second source or upgrade

---

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/heartbeat` | Daily heartbeat: `{"symbols": ["AAPL", "TSLA"]}`. Increments subscriber count. |
| `GET` | `/symbols/search?q=apple` | Proxy to Finnhub search (for symbol picker, if we skip the full index) |

Price reads go **directly to GCS** — no API endpoint needed.

---

## Market Hours Logic

US stock market: Mon-Fri, 9:30 AM - 4:00 PM ET (13:30 - 20:00 UTC).

```
is_market_open():
  - day is Mon-Fri
  - time is 13:30-20:00 UTC
  - (ignore holidays for now, can add later)

fetch_job():
  if is_market_open():
    fetch stocks + crypto
  else:
    fetch crypto only
```

This saves ~80% of API budget during off-hours (assuming most watchers track stocks).

---

## Open Questions

1. **Symbol index** — Full download (~10k symbols as JSON) or search-as-you-type via API proxy? Full download is simpler for the widget but ~1-2MB. Could gzip it. Or just provide search endpoint.

2. **ATH seed** — RESOLVED: Use `week52_high` from Finnhub `/stock/metric` endpoint on first daily fundamentals fetch. Updated to true ATH whenever price exceeds it.

3. **Counter reset** — Midnight UTC cron zeros `subscribers_today`. Should we keep a rolling average (last 7 days) for smoother prioritization, or is daily reset sufficient?

4. **GCS auth** — Public-read bucket (simplest) or signed URLs? Public is fine since it's just price data, nothing sensitive.

---

## Cost Estimate (free tier)

| Component | Free Tier | Our Usage |
|-----------|-----------|-----------|
| Finnhub | 60 req/min | ~50/min per tick |
| Cloud Run | 2M requests/mo | Fetch job: ~1440/day (1/min) + heartbeats |
| GCS | 5GB, 50k reads/day | Tiny JSON files, well within |
| Firestore | 50k reads, 20k writes/day | Symbol docs only |
| Cloud Scheduler | 3 free jobs | 2 jobs (1/min fetch + midnight reset) |

**$0/mo** at small scale.

---

## Next Steps

- [ ] Prototype Finnhub `/quote` call in Go
- [ ] Build fetch job with rate limiting
- [ ] Set up GCS bucket + write price files
- [ ] Build `/heartbeat` endpoint
- [ ] Build daily counter reset cron
- [ ] Seed symbol index file
- [ ] Build Android widget that reads GCS files
