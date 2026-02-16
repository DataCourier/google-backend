package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/storage"
)

// coinGeckoIDs maps Binance trading pair base symbols to CoinGecko IDs.
// Only need to map the ones people actually track.
var coinGeckoIDs = map[string]string{
	"BTC":   "bitcoin",
	"ETH":   "ethereum",
	"SOL":   "solana",
	"BNB":   "binancecoin",
	"XRP":   "ripple",
	"ADA":   "cardano",
	"DOGE":  "dogecoin",
	"DOT":   "polkadot",
	"AVAX":  "avalanche-2",
	"MATIC": "matic-network",
	"LINK":  "chainlink",
	"UNI":   "uniswap",
	"ATOM":  "cosmos",
	"LTC":   "litecoin",
	"NEAR":  "near",
	"APT":   "aptos",
	"ARB":   "arbitrum",
	"OP":    "optimism",
	"SUI":   "sui",
	"SHIB":  "shiba-inu",
	"PEPE":  "pepe",
	"FIL":   "filecoin",
	"TRX":   "tron",
	"AAVE":  "aave",
	"MKR":   "maker",
}

type coinGeckoMarketData struct {
	MarketData struct {
		ATH     map[string]float64 `json:"ath"`
		ATHDate map[string]string  `json:"ath_date"`
	} `json:"market_data"`
}

// extractBaseSymbol gets the base currency from a Binance pair.
// "BINANCE:BTCUSDT" -> "BTC", "BINANCE:ETHUSDT" -> "ETH"
func extractBaseSymbol(symbol string) string {
	// Remove exchange prefix
	parts := strings.SplitN(symbol, ":", 2)
	if len(parts) != 2 {
		return ""
	}
	pair := parts[1]
	// Remove common quote currencies (including with dash separator)
	for _, quote := range []string{"USDT", "BUSD", "USDC", "USD"} {
		if strings.HasSuffix(pair, quote) {
			base := strings.TrimSuffix(pair, quote)
			base = strings.TrimRight(base, "-")
			return base
		}
	}
	return ""
}

// getCryptoATH fetches ATH from CoinGecko for a crypto symbol.
func getCryptoATH(symbol string) (float64, string, error) {
	base := extractBaseSymbol(symbol)
	if base == "" {
		return 0, "", fmt.Errorf("can't extract base from %s", symbol)
	}

	geckoID, ok := coinGeckoIDs[base]
	if !ok {
		return 0, "", fmt.Errorf("no CoinGecko mapping for %s", base)
	}

	url := fmt.Sprintf("https://api.coingecko.com/api/v3/coins/%s?localization=false&tickers=false&community_data=false&developer_data=false&sparkline=false", geckoID)
	resp, err := http.Get(url)
	if err != nil {
		return 0, "", fmt.Errorf("coingecko request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return 0, "", fmt.Errorf("coingecko rate limited")
	}
	if resp.StatusCode != 200 {
		return 0, "", fmt.Errorf("coingecko returned %d", resp.StatusCode)
	}

	var data coinGeckoMarketData
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0, "", fmt.Errorf("coingecko decode: %w", err)
	}

	ath := data.MarketData.ATH["usd"]
	athDateStr := data.MarketData.ATHDate["usd"]

	// Parse date to YYYY-MM-DD
	athDate := "unknown"
	if t, err := time.Parse(time.RFC3339Nano, athDateStr); err == nil {
		athDate = t.Format("2006-01-02")
	}

	return ath, athDate, nil
}

// RunATHRefresh updates ATH data for all watched symbols.
// - Stocks: refreshes 52-week high from Finnhub metrics (free)
// - Crypto: fetches true ATH from CoinGecko (free)
// Called weekly on Mondays via cron.
func RunATHRefresh(ctx context.Context, fsClient *firestore.Client, gcsBucket *storage.BucketHandle, fh *FinnhubClient) error {
	docs, err := fsClient.Collection("watched_symbols").Documents(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("read watched_symbols: %w", err)
	}

	updated := 0

	for _, doc := range docs {
		var ws WatchedSymbol
		if err := doc.DataTo(&ws); err != nil {
			continue
		}

		// Skip symbols with no recent interest
		if ws.subscriberPeak() <= 0 {
			continue
		}

		var newATH float64
		var newATHDate string

		if isCrypto(ws.Symbol) {
			// CoinGecko for crypto ATH
			ath, athDate, err := getCryptoATH(ws.Symbol)
			if err != nil {
				log.Printf("ATH: skip %s: %v", ws.Symbol, err)
				continue
			}
			newATH = ath
			newATHDate = athDate

			// CoinGecko free tier: 10-30 req/min, be polite
			time.Sleep(2 * time.Second)
		} else {
			// Finnhub 52-week high for stocks
			metrics, err := fh.GetMetrics(ws.Symbol)
			if err != nil {
				log.Printf("ATH: skip %s: %v", ws.Symbol, err)
				continue
			}
			if metrics.Week52High > 0 {
				newATH = metrics.Week52High
				// Finnhub provides the date in the raw response, but we extract it separately
				newATHDate = get52WeekHighDate(fh, ws.Symbol)
			}
		}

		if newATH <= 0 {
			continue
		}

		// Only update if the new ATH is higher than what we have
		// (for crypto, CoinGecko ATH is the true all-time, so always use it)
		if isCrypto(ws.Symbol) || newATH > ws.ATH {
			// Update Firestore
			_, err := fsClient.Collection("watched_symbols").Doc(ws.Symbol).Set(ctx, map[string]interface{}{
				"ath":      newATH,
				"ath_date": newATHDate,
			}, firestore.MergeAll)
			if err != nil {
				log.Printf("ATH: failed to update Firestore for %s: %v", ws.Symbol, err)
				continue
			}

			// Update price file
			pf, err := ReadPriceFile(ctx, gcsBucket, ws.Symbol)
			if err != nil {
				log.Printf("ATH: no price file for %s, skipping GCS update", ws.Symbol)
			} else {
				pf.ATH = newATH
				pf.ATHDate = newATHDate
				if pf.Price > 0 {
					pf.OffATHPct = ((pf.Price - newATH) / newATH) * 100
				}
				if err := WritePriceFile(ctx, gcsBucket, pf); err != nil {
					log.Printf("ATH: failed to write GCS for %s: %v", ws.Symbol, err)
				}
			}

			updated++
			log.Printf("  ATH %s: $%.2f (%s)", ws.Symbol, newATH, newATHDate)
		}
	}

	log.Printf("ATH refresh complete: %d symbols updated", updated)
	return nil
}

// get52WeekHighDate fetches the 52-week high date from Finnhub raw metrics.
func get52WeekHighDate(fh *FinnhubClient, symbol string) string {
	<-fh.ticker.C

	url := fmt.Sprintf("https://finnhub.io/api/v1/stock/metric?symbol=%s&metric=all&token=%s", symbol, fh.apiKey)
	resp, err := fh.httpClient.Get(url)
	if err != nil {
		return "unknown"
	}
	defer resp.Body.Close()

	var raw struct {
		Metric map[string]interface{} `json:"metric"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return "unknown"
	}

	if date, ok := raw.Metric["52WeekHighDate"].(string); ok {
		return date
	}
	return "unknown"
}
