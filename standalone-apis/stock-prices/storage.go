package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cloud.google.com/go/storage"
)

type PriceFile struct {
	Symbol        string  `json:"symbol"`
	Price         float64 `json:"price"`
	High          float64 `json:"high"`
	Low           float64 `json:"low"`
	Open          float64 `json:"open"`
	PreviousClose float64 `json:"previous_close"`
	ATH           float64 `json:"ath"`
	ATHDate       string  `json:"ath_date"`
	OffATHPct     float64 `json:"off_ath_pct"`
	FetchedAt     int64   `json:"fetched_at"`

	// Populated by daily fundamentals job (stocks only)
	Earnings              *EarningsData  `json:"earnings,omitempty"`
	Fundamentals          *Fundamentals  `json:"fundamentals,omitempty"`
	FundamentalsUpdatedAt int64          `json:"fundamentals_updated_at,omitempty"`
}

type EarningsData struct {
	Date              string  `json:"date"`
	DaysAway          int     `json:"days_away"`
	LastEPSActual     float64 `json:"last_eps_actual"`
	LastEPSEstimate   float64 `json:"last_eps_estimate"`
	LastEPSSurprisePct float64 `json:"last_eps_surprise_pct"`
}

func ReadPriceFile(ctx context.Context, bucket *storage.BucketHandle, symbol string) (*PriceFile, error) {
	filename := strings.ReplaceAll(symbol, ":", "_")
	obj := bucket.Object(fmt.Sprintf("prices/%s.json", filename))

	r, err := obj.NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("read price file: %w", err)
	}
	defer r.Close()

	var pf PriceFile
	if err := json.NewDecoder(r).Decode(&pf); err != nil {
		return nil, fmt.Errorf("decode price file: %w", err)
	}
	return &pf, nil
}

func WritePriceFile(ctx context.Context, bucket *storage.BucketHandle, pf *PriceFile) error {
	// Use symbol as filename, replacing : with _ for crypto (BINANCE:BTCUSDT → BINANCE_BTCUSDT)
	filename := strings.ReplaceAll(pf.Symbol, ":", "_")
	obj := bucket.Object(fmt.Sprintf("prices/%s.json", filename))

	w := obj.NewWriter(ctx)
	w.ContentType = "application/json"
	w.CacheControl = "public, max-age=60" // CDN can cache for 1 min

	if err := json.NewEncoder(w).Encode(pf); err != nil {
		w.Close()
		return fmt.Errorf("encode price file: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("write price file to GCS: %w", err)
	}

	return nil
}
