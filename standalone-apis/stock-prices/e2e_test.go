package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// E2E tests that work against both local and remote backends.
//
// Usage:
//   Local:  BASE_URL=http://localhost:8081 go test -v -run E2E
//   Remote: BASE_URL=https://stock-prices-xxx.run.app go test -v -run E2E

func baseURL(t *testing.T) string {
	t.Helper()
	u := os.Getenv("BASE_URL")
	if u == "" {
		t.Fatal("BASE_URL not set. Example: BASE_URL=http://localhost:8081 go test -v -run E2E")
	}
	return u
}

func TestE2E_Health(t *testing.T) {
	url := baseURL(t)
	resp, err := http.Get(url + "/health")
	if err != nil {
		t.Fatalf("GET /health failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}
}

func TestE2E_HeartbeatAndFetch(t *testing.T) {
	url := baseURL(t)

	// Use crypto so the test works outside market hours
	symbol := "BINANCE:BTCUSDT"
	body, _ := json.Marshal(heartbeatRequest{Symbols: []string{symbol}})

	resp, err := http.Post(url+"/heartbeat", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /heartbeat failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("Heartbeat: expected 200, got %d", resp.StatusCode)
	}

	var hbResp map[string]string
	json.NewDecoder(resp.Body).Decode(&hbResp)
	if hbResp["status"] != "ok" {
		t.Fatalf("Heartbeat: expected status ok, got %v", hbResp)
	}

	t.Log("Heartbeat sent for", symbol)

	// Step 2: Trigger a fetch cycle
	resp, err = http.Post(url+"/fetch", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /fetch failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("Fetch: expected 200, got %d", resp.StatusCode)
	}

	t.Log("Fetch cycle completed")

	// Step 3: Verify the price file was written to GCS
	// For local: read from fake-gcs
	// For remote: read from the public GCS bucket
	gcsBucket := os.Getenv("GCS_BUCKET")
	if gcsBucket == "" {
		gcsBucket = "stock-prices-dev"
	}

	// Filename uses _ instead of : for crypto symbols
	filename := strings.ReplaceAll(symbol, ":", "_")

	// Try reading the price file via GCS public URL or emulator
	storageHost := os.Getenv("STORAGE_EMULATOR_HOST")
	var priceURL string
	if storageHost != "" {
		// Strip http:// prefix if present for URL construction
		host := strings.TrimPrefix(storageHost, "http://")
		host = strings.TrimPrefix(host, "https://")
		priceURL = fmt.Sprintf("http://%s/storage/v1/b/%s/o/prices%%2F%s.json?alt=media", host, gcsBucket, filename)
	} else {
		priceURL = fmt.Sprintf("https://storage.googleapis.com/%s/prices/%s.json", gcsBucket, filename)
	}

	// Retry a couple times in case of propagation delay
	var priceResp *http.Response
	for i := 0; i < 3; i++ {
		priceResp, err = http.Get(priceURL)
		if err == nil && priceResp.StatusCode == 200 {
			break
		}
		if priceResp != nil {
			priceResp.Body.Close()
		}
		time.Sleep(1 * time.Second)
	}

	if err != nil {
		t.Fatalf("GET price file failed: %v", err)
	}
	defer priceResp.Body.Close()

	if priceResp.StatusCode != 200 {
		t.Fatalf("Price file: expected 200, got %d", priceResp.StatusCode)
	}

	var pf PriceFile
	if err := json.NewDecoder(priceResp.Body).Decode(&pf); err != nil {
		t.Fatalf("Decode price file: %v", err)
	}

	// Validate price file contents
	if pf.Symbol != symbol {
		t.Errorf("Symbol: expected %s, got %s", symbol, pf.Symbol)
	}
	if pf.Price <= 0 {
		t.Errorf("Price should be positive, got %f", pf.Price)
	}
	if pf.FetchedAt <= 0 {
		t.Errorf("FetchedAt should be set, got %d", pf.FetchedAt)
	}
	if pf.ATH <= 0 {
		t.Errorf("ATH should be positive, got %f", pf.ATH)
	}
	if pf.OffATHPct > 0.01 {
		t.Errorf("OffATHPct should be <= 0 (or ~0 at ATH), got %f", pf.OffATHPct)
	}

	t.Logf("Price file OK: %s $%.2f (ATH $%.2f, %.1f%% off)", pf.Symbol, pf.Price, pf.ATH, pf.OffATHPct)
}

func TestE2E_HeartbeatValidation(t *testing.T) {
	url := baseURL(t)

	// Empty symbols should fail
	body, _ := json.Marshal(heartbeatRequest{Symbols: []string{}})
	resp, err := http.Post(url+"/heartbeat", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /heartbeat failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Errorf("Empty symbols: expected 400, got %d", resp.StatusCode)
	}
}

func TestE2E_HeartbeatMultipleSymbols(t *testing.T) {
	url := baseURL(t)

	symbols := []string{"AAPL", "MSFT", "BINANCE:BTCUSDT"}
	body, _ := json.Marshal(heartbeatRequest{Symbols: symbols})

	resp, err := http.Post(url+"/heartbeat", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /heartbeat failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	t.Logf("Heartbeat sent for %d symbols", len(symbols))
}

func TestE2E_FetchCycleMultiple(t *testing.T) {
	url := baseURL(t)

	// Use crypto so test works outside market hours
	symbols := []string{"BINANCE:BTCUSDT", "BINANCE:ETHUSDT"}
	body, _ := json.Marshal(heartbeatRequest{Symbols: symbols})
	resp, err := http.Post(url+"/heartbeat", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}
	resp.Body.Close()

	// Trigger fetch
	resp, err = http.Post(url+"/fetch", "application/json", nil)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("Fetch: expected 200, got %d", resp.StatusCode)
	}

	t.Log("Fetch cycle with multiple symbols completed")
}

func TestE2E_Universe(t *testing.T) {
	url := baseURL(t)

	resp, err := http.Post(url+"/universe", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /universe failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	// Read the index file from GCS
	gcsBucket := os.Getenv("GCS_BUCKET")
	if gcsBucket == "" {
		gcsBucket = "stock-prices-dev"
	}

	storageHost := os.Getenv("STORAGE_EMULATOR_HOST")
	var indexURL string
	if storageHost != "" {
		host := strings.TrimPrefix(storageHost, "http://")
		host = strings.TrimPrefix(host, "https://")
		indexURL = fmt.Sprintf("http://%s/storage/v1/b/%s/o/symbols%%2Findex.json?alt=media", host, gcsBucket)
	} else {
		indexURL = fmt.Sprintf("https://storage.googleapis.com/%s/symbols/index.json", gcsBucket)
	}

	indexResp, err := http.Get(indexURL)
	if err != nil {
		t.Fatalf("GET index file failed: %v", err)
	}
	defer indexResp.Body.Close()

	var index UniverseIndex
	json.NewDecoder(indexResp.Body).Decode(&index)

	if len(index.Stocks) < 1000 {
		t.Errorf("Expected >1000 stocks, got %d", len(index.Stocks))
	}
	if len(index.Crypto) < 100 {
		t.Errorf("Expected >100 crypto, got %d", len(index.Crypto))
	}
	if index.UpdatedAt <= 0 {
		t.Errorf("UpdatedAt should be set")
	}

	t.Logf("Universe OK: %d stocks, %d crypto", len(index.Stocks), len(index.Crypto))
}

func TestE2E_Cron(t *testing.T) {
	url := baseURL(t)

	// Ensure there's a heartbeat so fetch has something to do
	body, _ := json.Marshal(heartbeatRequest{Symbols: []string{"BINANCE:BTCUSDT"}})
	http.Post(url+"/heartbeat", "application/json", bytes.NewReader(body))

	resp, err := http.Post(url+"/cron", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /cron failed: %v", err)
	}
	defer resp.Body.Close()

	var results map[string]string
	json.NewDecoder(resp.Body).Decode(&results)

	// Fetch should always run
	if results["fetch"] != "ok" {
		t.Errorf("Fetch result: %s", results["fetch"])
	}

	t.Logf("Cron results: %v", results)
}

func TestE2E_Fundamentals(t *testing.T) {
	url := baseURL(t)

	// Heartbeat for a stock (not crypto — fundamentals only apply to stocks)
	body, _ := json.Marshal(heartbeatRequest{Symbols: []string{"AAPL"}})
	resp, err := http.Post(url+"/heartbeat", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}
	resp.Body.Close()

	// First do a price fetch so there's a price file to merge into
	// (need market open or we skip — so we trigger fetch which may skip AAPL on weekends)
	http.Post(url+"/fetch", "application/json", nil)

	// Trigger fundamentals
	resp, err = http.Post(url+"/fundamentals", "application/json", nil)
	if err != nil {
		t.Fatalf("Fundamentals failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("Fundamentals: expected 200, got %d", resp.StatusCode)
	}

	// Read the price file and verify fundamentals are present
	gcsBucket := os.Getenv("GCS_BUCKET")
	if gcsBucket == "" {
		gcsBucket = "stock-prices-dev"
	}

	storageHost := os.Getenv("STORAGE_EMULATOR_HOST")
	var priceURL string
	if storageHost != "" {
		host := strings.TrimPrefix(storageHost, "http://")
		host = strings.TrimPrefix(host, "https://")
		priceURL = fmt.Sprintf("http://%s/storage/v1/b/%s/o/prices%%2FAAPL.json?alt=media", host, gcsBucket)
	} else {
		priceURL = fmt.Sprintf("https://storage.googleapis.com/%s/prices/AAPL.json", gcsBucket)
	}

	var priceResp *http.Response
	for i := 0; i < 3; i++ {
		priceResp, err = http.Get(priceURL)
		if err == nil && priceResp.StatusCode == 200 {
			break
		}
		if priceResp != nil {
			priceResp.Body.Close()
		}
		time.Sleep(1 * time.Second)
	}

	if err != nil || priceResp.StatusCode != 200 {
		t.Fatalf("Could not read AAPL price file")
	}
	defer priceResp.Body.Close()

	var pf PriceFile
	json.NewDecoder(priceResp.Body).Decode(&pf)

	if pf.Fundamentals == nil {
		t.Fatal("Fundamentals should be populated")
	}
	if pf.Fundamentals.PETTM <= 0 {
		t.Errorf("PE TTM should be positive, got %f", pf.Fundamentals.PETTM)
	}
	if pf.Fundamentals.EPSTTM <= 0 {
		t.Errorf("EPS TTM should be positive, got %f", pf.Fundamentals.EPSTTM)
	}
	if pf.Fundamentals.Week52High <= 0 {
		t.Errorf("52-week high should be positive, got %f", pf.Fundamentals.Week52High)
	}
	if pf.FundamentalsUpdatedAt <= 0 {
		t.Errorf("FundamentalsUpdatedAt should be set")
	}

	t.Logf("Fundamentals OK: AAPL PE=%.1f PS=%.1f EPS=$%.2f 52wH=$%.2f",
		pf.Fundamentals.PETTM, pf.Fundamentals.PSTTM, pf.Fundamentals.EPSTTM, pf.Fundamentals.Week52High)

	if pf.Earnings != nil {
		t.Logf("Earnings: date=%s days_away=%d eps_actual=%.2f surprise=%.1f%%",
			pf.Earnings.Date, pf.Earnings.DaysAway, pf.Earnings.LastEPSActual, pf.Earnings.LastEPSSurprisePct)
	}

	// ATH should be seeded from 52-week high
	if pf.ATH <= 0 {
		t.Errorf("ATH should be seeded from 52-week high, got %f", pf.ATH)
	}
	t.Logf("ATH: $%.2f", pf.ATH)
}
