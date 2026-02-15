package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/storage"
)

// RunFundamentalsCycle refreshes earnings + fundamentals for watched symbols.
// Called once per day. Uses ~2 API calls per symbol (earnings + metrics).
// Budget: 200 symbols × 2 calls = 400 calls, well within daily limits.
func RunFundamentalsCycle(ctx context.Context, fsClient *firestore.Client, gcsBucket *storage.BucketHandle, fh *FinnhubClient) error {
	docs, err := fsClient.Collection("watched_symbols").Documents(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("read watched_symbols: %w", err)
	}

	now := time.Now()
	updated := 0

	for _, doc := range docs {
		var ws WatchedSymbol
		if err := doc.DataTo(&ws); err != nil {
			log.Printf("Skip bad doc %s: %v", doc.Ref.ID, err)
			continue
		}

		// Skip symbols with no recent subscribers
		if ws.SubscribersToday <= 0 {
			continue
		}

		// Skip if already updated today
		if ws.FundamentalsCheckedAt > 0 {
			lastCheck := time.UnixMilli(ws.FundamentalsCheckedAt)
			if now.Sub(lastCheck) < 20*time.Hour {
				continue
			}
		}

		// Skip crypto for fundamentals (no earnings/PE for BTC)
		if isCrypto(ws.Symbol) {
			continue
		}

		// Fetch earnings
		var earningsData *EarningsData
		cal, err := fh.GetEarnings(ws.Symbol)
		if err != nil {
			log.Printf("Failed to fetch earnings for %s: %v", ws.Symbol, err)
		} else if len(cal.EarningsCalendar) > 0 {
			entry := cal.EarningsCalendar[0]
			earningsDate, _ := time.Parse("2006-01-02", entry.Date)
			daysAway := int(math.Ceil(earningsDate.Sub(now).Hours() / 24))

			surprisePct := 0.0
			if entry.EPSEstimate != 0 {
				surprisePct = ((entry.EPSActual - entry.EPSEstimate) / math.Abs(entry.EPSEstimate)) * 100
			}

			earningsData = &EarningsData{
				Date:               entry.Date,
				DaysAway:           daysAway,
				LastEPSActual:      entry.EPSActual,
				LastEPSEstimate:    entry.EPSEstimate,
				LastEPSSurprisePct: surprisePct,
			}
		}

		// Fetch fundamentals
		fundamentals, err := fh.GetMetrics(ws.Symbol)
		if err != nil {
			log.Printf("Failed to fetch metrics for %s: %v", ws.Symbol, err)
			continue
		}

		// Seed ATH from 52-week high if we don't have one yet
		ath := ws.ATH
		athDate := ws.ATHDate
		if ath == 0 && fundamentals.Week52High > 0 {
			ath = fundamentals.Week52High
			athDate = "seeded"
			log.Printf("  %s: seeded ATH from 52-week high: $%.2f", ws.Symbol, ath)
		}

		// Read existing price file to merge fundamentals into it
		pf, err := ReadPriceFile(ctx, gcsBucket, ws.Symbol)
		if err != nil {
			// No price file yet — create a minimal one
			pf = &PriceFile{
				Symbol: ws.Symbol,
				ATH:    ath,
				ATHDate: athDate,
			}
		}

		// Update ATH in price file too
		if ath > pf.ATH {
			pf.ATH = ath
			pf.ATHDate = athDate
			if pf.Price > 0 {
				pf.OffATHPct = ((pf.Price - ath) / ath) * 100
			}
		}

		pf.Earnings = earningsData
		pf.Fundamentals = fundamentals
		pf.FundamentalsUpdatedAt = now.UnixMilli()

		if err := WritePriceFile(ctx, gcsBucket, pf); err != nil {
			log.Printf("Failed to write GCS for %s: %v", ws.Symbol, err)
			continue
		}

		// Update Firestore
		updates := map[string]interface{}{
			"fundamentals_checked_at": now.UnixMilli(),
		}
		if ath > ws.ATH {
			updates["ath"] = ath
			updates["ath_date"] = athDate
		}
		_, err = fsClient.Collection("watched_symbols").Doc(ws.Symbol).Set(ctx, updates, firestore.MergeAll)
		if err != nil {
			log.Printf("Failed to update Firestore for %s: %v", ws.Symbol, err)
			continue
		}

		updated++
		log.Printf("  %s: PE=%.1f PS=%.1f EPS=$%.2f earnings=%s",
			ws.Symbol, fundamentals.PETTM, fundamentals.PSTTM, fundamentals.EPSTTM,
			earningsData.Date)
	}

	log.Printf("Fundamentals cycle complete: %d symbols updated", updated)
	return nil
}
