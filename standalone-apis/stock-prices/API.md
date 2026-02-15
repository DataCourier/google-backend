# Stock Prices API

**Base URL:** `https://stock-prices-133184350530.us-central1.run.app`
**GCS Bucket:** `https://storage.googleapis.com/stock-widget/`

## How It Works

The service fetches stock and crypto prices from Finnhub and writes them as static JSON files to a public GCS bucket. Android widgets read prices directly from GCS — no API calls needed for reads.

### Architecture

```
Android Widget  ──GET──>  GCS (static JSON files)
                            ▲
                            │ writes
Cloud Scheduler ──POST /cron──> Stock Prices Service ──> Finnhub API
                            │
Android App ──POST /heartbeat──┘
```

### Fetch Priority

Tickers are fetched based on subscriber count and staleness:

| Subscribers (7-day peak) | Target freshness |
|--------------------------|-----------------|
| 1–2                      | 15 min          |
| 3–9                      | 8 min           |
| 10+                      | 4 min           |

Each cron tick fetches the top 50 most overdue tickers, sorted by `staleness / target_freshness`. Stocks are only fetched during US trading hours (weekdays 8:00–00:00 UTC); crypto is 24/7.

### Cron Schedule (UTC)

| Time          | Job                      |
|---------------|--------------------------|
| Every 5 min   | Fetch prices (top 50)    |
| 13:00         | Universe update          |
| 13:05–13:55   | Fundamentals + earnings  |
| 13:15         | Health report (Telegram) |
| 00:00         | Rotate subscriber counts |

---

## Service Endpoints

### `GET /health`

Health check.

**Response:** `200 OK` — `ok`

---

### `POST /heartbeat`

Register interest in symbols. Increments the daily subscriber count.

**Request:**
```json
{
  "symbols": ["AAPL", "TSLA", "BINANCE:BTCUSDT"]
}
```

**Constraints:** 1–50 symbols per request.

**Response:** `200 OK`
```json
{"status": "ok"}
```

---

### `POST /cron`

Triggered by Cloud Scheduler every 5 minutes. Runs fetch cycle and time-based jobs.

**Response:** `200 OK`
```json
{
  "fetch": "ok",
  "universe": "ok",
  "fundamentals": "ok"
}
```
Only the jobs that ran in this window appear in the response.

---

### `POST /fetch`

Manually trigger a price fetch cycle.

**Response:** `200 OK`
```json
{"status": "ok"}
```

---

### `POST /fundamentals`

Manually trigger fundamentals + earnings fetch for all watched stocks.

**Response:** `200 OK`
```json
{"status": "ok"}
```

---

### `POST /universe`

Manually trigger universe update (all available symbols from Finnhub).

**Response:** `200 OK`
```json
{"status": "ok"}
```

---

### `POST /health-report`

Send a health summary to Telegram.

**Response:** `200 OK`
```json
{"status": "ok"}
```

---

## GCS Static Files

All files are publicly readable. Clients read these directly — no auth needed.

### Price File

**URL:** `https://storage.googleapis.com/stock-widget/prices/{SYMBOL}.json`

Crypto symbols use `_` instead of `:` in filenames (e.g., `BINANCE_BTCUSDT.json`).

**Example:** `GET https://storage.googleapis.com/stock-widget/prices/AAPL.json`

```json
{
  "symbol": "AAPL",
  "price": 255.79,
  "high": 262.23,
  "low": 255.45,
  "open": 262.02,
  "previous_close": 261.73,
  "ath": 288.62,
  "ath_date": "2025-12-03",
  "off_ath_pct": -11.38,
  "fetched_at": 1771016400000,
  "earnings": {
    "date": "2026-01-29",
    "days_away": -17,
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
  "fundamentals_updated_at": 1771016400000
}
```

| Field | Type | Description |
|-------|------|-------------|
| `symbol` | string | Ticker symbol |
| `price` | float | Current/last price |
| `high` | float | Day high |
| `low` | float | Day low |
| `open` | float | Day open |
| `previous_close` | float | Previous day close |
| `ath` | float | All-time high (tracked since first fetch) |
| `ath_date` | string | Date ATH was set (YYYY-MM-DD) |
| `off_ath_pct` | float | % below ATH (0 = at ATH, -11.38 = 11.38% below) |
| `fetched_at` | int64 | Unix millis when price was last fetched |
| `earnings` | object? | Omitted if no earnings data. Stocks only. |
| `earnings.date` | string | Next/last earnings date |
| `earnings.days_away` | int | Days until earnings (negative = past) |
| `earnings.last_eps_actual` | float | Most recent actual EPS |
| `earnings.last_eps_estimate` | float | Most recent estimated EPS |
| `earnings.last_eps_surprise_pct` | float | EPS surprise % |
| `fundamentals` | object? | Omitted for crypto or if not yet fetched |
| `fundamentals_updated_at` | int64 | Unix millis when fundamentals were last updated |

### Universe Index

**URL:** `https://storage.googleapis.com/stock-widget/symbols/index.json`

Updated daily at 13:00 UTC. ~30k stocks + ~3.5k crypto symbols.

```json
{
  "updated_at": 1771016400000,
  "stocks": [
    {
      "symbol": "AAPL",
      "name": "APPLE INC",
      "type": "Common Stock",
      "display_symbol": "AAPL"
    }
  ],
  "crypto": [
    {
      "symbol": "BINANCE:BTCUSDT",
      "name": "Binance BTC/USDT",
      "display_symbol": "BTC/USDT"
    }
  ]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `updated_at` | int64 | Unix millis when index was last refreshed |
| `stocks[].symbol` | string | Finnhub symbol (use for heartbeat + price file lookup) |
| `stocks[].name` | string | Company name |
| `stocks[].type` | string | "Common Stock", "ETP", "ADR", etc. |
| `stocks[].display_symbol` | string | Human-readable ticker |
| `crypto[].symbol` | string | Exchange-prefixed symbol (e.g., `BINANCE:BTCUSDT`) |
| `crypto[].name` | string | Pair description |
| `crypto[].display_symbol` | string | Human-readable pair (e.g., `BTC/USDT`) |
