package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"cloud.google.com/go/storage"
)

// UniverseIndex is the full list of available symbols, written to GCS.
type UniverseIndex struct {
	UpdatedAt int64           `json:"updated_at"`
	Stocks    []UniverseEntry `json:"stocks"`
	Crypto    []UniverseEntry `json:"crypto"`
}

type UniverseEntry struct {
	Symbol        string `json:"symbol"`
	Name          string `json:"name"`
	Type          string `json:"type,omitempty"`   // "Common Stock", "ETP", etc (stocks only)
	DisplaySymbol string `json:"display_symbol"`
}

// RunUniverseUpdate fetches all available symbols from Finnhub and writes
// symbols/index.json to GCS. Uses 2 API calls (US stocks + Binance crypto).
// These are list endpoints and don't count against the rate limit.
func RunUniverseUpdate(ctx context.Context, gcsBucket *storage.BucketHandle, fh *FinnhubClient) error {
	log.Println("Universe update: fetching stock symbols...")
	stocks, err := fetchSymbolList(fh, "https://finnhub.io/api/v1/stock/symbol?exchange=US&token="+fh.apiKey)
	if err != nil {
		return fmt.Errorf("fetch stock symbols: %w", err)
	}

	log.Println("Universe update: fetching crypto symbols...")
	crypto, err := fetchSymbolList(fh, "https://finnhub.io/api/v1/crypto/symbol?exchange=binance&token="+fh.apiKey)
	if err != nil {
		return fmt.Errorf("fetch crypto symbols: %w", err)
	}

	var stockEntries []UniverseEntry
	for _, s := range stocks {
		stockEntries = append(stockEntries, UniverseEntry{
			Symbol:        s.Symbol,
			Name:          s.Description,
			Type:          s.Type,
			DisplaySymbol: s.DisplaySymbol,
		})
	}

	var cryptoEntries []UniverseEntry
	for _, s := range crypto {
		cryptoEntries = append(cryptoEntries, UniverseEntry{
			Symbol:        s.Symbol,
			Name:          s.Description,
			DisplaySymbol: s.DisplaySymbol,
		})
	}

	index := &UniverseIndex{
		UpdatedAt: time.Now().UnixMilli(),
		Stocks:    stockEntries,
		Crypto:    cryptoEntries,
	}

	// Write to GCS
	obj := gcsBucket.Object("symbols/index.json")
	w := obj.NewWriter(ctx)
	w.ContentType = "application/json"
	w.CacheControl = "public, max-age=3600"

	if err := json.NewEncoder(w).Encode(index); err != nil {
		w.Close()
		return fmt.Errorf("encode universe index: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("write universe index to GCS: %w", err)
	}

	log.Printf("Universe update complete: %d stocks, %d crypto", len(stockEntries), len(cryptoEntries))
	return nil
}

// Raw Finnhub symbol response
type finnhubSymbol struct {
	Symbol        string `json:"symbol"`
	Description   string `json:"description"`
	DisplaySymbol string `json:"displaySymbol"`
	Type          string `json:"type"`
}

// fetchSymbolList calls Finnhub list endpoints (not rate-limited like quote endpoints).
func fetchSymbolList(fh *FinnhubClient, url string) ([]finnhubSymbol, error) {
	resp, err := fh.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("finnhub returned %d", resp.StatusCode)
	}

	var symbols []finnhubSymbol
	if err := json.NewDecoder(resp.Body).Decode(&symbols); err != nil {
		return nil, err
	}
	return symbols, nil
}
