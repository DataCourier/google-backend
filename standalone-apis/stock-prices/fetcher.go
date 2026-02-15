package main

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/storage"
)

type WatchedSymbol struct {
	Symbol                string    `firestore:"symbol"`
	ATH                   float64   `firestore:"ath"`
	ATHDate               string    `firestore:"ath_date"`
	SubscribersToday      int       `firestore:"subscribers_today"`
	SubscribersHistory    []int     `firestore:"subscribers_history"` // last 7 days, [0]=today, [1]=yesterday, ...
	LastFetchedAt         int64     `firestore:"last_fetched_at"`
	FundamentalsCheckedAt int64     `firestore:"fundamentals_checked_at"`
}

// subscriberPeak returns the max subscriber count over the 7-day window.
// Used for freshness tier — a ticker that had 200 subs any day this week
// stays in the fast lane. Only drops tier after a full week of lower numbers.
func (ws *WatchedSymbol) subscriberPeak() int {
	peak := ws.SubscribersToday
	for _, v := range ws.SubscribersHistory {
		if v > peak {
			peak = v
		}
	}
	return peak
}

type symbolWithPriority struct {
	WatchedSymbol
	priority float64
}

func targetFreshness(subscribers int) time.Duration {
	switch {
	case subscribers >= 10:
		return 4 * time.Minute
	case subscribers >= 3:
		return 8 * time.Minute
	default:
		return 15 * time.Minute
	}
}

// isStockHours returns true during US premarket + market + aftermarket (weekdays).
// Premarket:   4:00 AM - 9:30 AM ET  =  8:00 - 13:30 UTC
// Market:      9:30 AM - 4:00 PM ET  = 13:30 - 20:00 UTC
// Aftermarket: 4:00 PM - 8:00 PM ET  = 20:00 - 00:00 UTC
// Combined:    8:00 - 00:00 UTC on weekdays
func isStockHours() bool {
	return isStockHoursAt(time.Now().UTC())
}

func isStockHoursAt(now time.Time) bool {
	weekday := now.Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return false
	}
	// 8:00 UTC (4am ET) to 00:00 UTC (8pm ET)
	hour := now.Hour()
	return hour >= 8
}

func isCrypto(symbol string) bool {
	return strings.Contains(symbol, ":")
}

const maxPerTick = 50

func RunFetchCycle(ctx context.Context, fsClient *firestore.Client, gcsBucket *storage.BucketHandle, finnhub *FinnhubClient) error {
	// Read all watched symbols
	docs, err := fsClient.Collection("watched_symbols").
		Documents(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("read watched_symbols: %w", err)
	}

	if len(docs) == 0 {
		log.Println("No watched symbols with subscribers, skipping")
		return nil
	}

	stockHours := isStockHours()
	now := time.Now()

	// Compute priorities
	var candidates []symbolWithPriority
	for _, doc := range docs {
		var ws WatchedSymbol
		if err := doc.DataTo(&ws); err != nil {
			log.Printf("Skip bad doc %s: %v", doc.Ref.ID, err)
			continue
		}

		// Skip symbols with no recent interest (entire 7-day window is 0)
		peak := ws.subscriberPeak()
		if peak <= 0 {
			continue
		}

		// Skip stocks outside trading hours (pre+market+after); crypto is 24/7
		if !stockHours && !isCrypto(ws.Symbol) {
			continue
		}

		// Tier based on peak (7-day max), so one quiet day doesn't downgrade
		target := targetFreshness(peak)
		staleness := now.Sub(time.UnixMilli(ws.LastFetchedAt))
		priority := float64(staleness) / float64(target)

		candidates = append(candidates, symbolWithPriority{
			WatchedSymbol: ws,
			priority:      priority,
		})
	}

	// Sort by priority descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].priority > candidates[j].priority
	})

	// Take top N
	n := maxPerTick
	if len(candidates) < n {
		n = len(candidates)
	}
	batch := candidates[:n]

	log.Printf("Fetch cycle: %d candidates, fetching top %d (stock_hours=%v)", len(candidates), n, stockHours)

	fetched := 0
	for _, c := range batch {
		quote, err := finnhub.GetQuote(c.Symbol)
		if err != nil {
			log.Printf("Failed to fetch %s: %v", c.Symbol, err)
			continue
		}

		// Update ATH
		ath := c.ATH
		athDate := c.ATHDate
		if quote.Current > ath {
			ath = quote.Current
			athDate = time.Now().UTC().Format("2006-01-02")
		}

		// Compute off-ATH percentage
		offATHPct := 0.0
		if ath > 0 {
			offATHPct = ((quote.Current - ath) / ath) * 100
		}

		nowMs := time.Now().UnixMilli()

		// Write price file to GCS
		pf := &PriceFile{
			Symbol:        c.Symbol,
			Price:         quote.Current,
			High:          quote.High,
			Low:           quote.Low,
			Open:          quote.Open,
			PreviousClose: quote.PreviousClose,
			ATH:           ath,
			ATHDate:       athDate,
			OffATHPct:     offATHPct,
			FetchedAt:     nowMs,
		}

		if err := WritePriceFile(ctx, gcsBucket, pf); err != nil {
			log.Printf("Failed to write GCS for %s: %v", c.Symbol, err)
			continue
		}

		// Update Firestore
		_, err = fsClient.Collection("watched_symbols").Doc(c.Symbol).Set(ctx, map[string]interface{}{
			"ath":             ath,
			"ath_date":        athDate,
			"last_fetched_at": nowMs,
		}, firestore.MergeAll)
		if err != nil {
			log.Printf("Failed to update Firestore for %s: %v", c.Symbol, err)
			continue
		}

		fetched++
		log.Printf("  %s: $%.2f (ATH $%.2f, %.1f%% off)", c.Symbol, quote.Current, ath, offATHPct)
	}

	log.Printf("Fetch cycle complete: %d/%d symbols updated", fetched, n)
	return nil
}
