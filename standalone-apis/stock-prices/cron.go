package main

import (
	"context"
	"log"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

// cronMu prevents overlapping cron runs. If a previous tick is still
// running, the next one skips entirely to preserve API quota.
var cronMu sync.Mutex

// CronRouter is called every 5 minutes by Cloud Scheduler.
// It routes to different jobs based on the current time (UTC).
//
// Schedule (all times UTC):
//   Every 5 min:   RunFetchCycle (price quotes for top 50 symbols)
//   13:00:         RunUniverseUpdate (full symbol list refresh)
//   13:05-13:55:   RunFundamentalsCycle (earnings + metrics, uses "already checked today" guard)
//   00:00:         RotateSubscriberCounts (push today→history, reset today to 0)
//
// With 5-min intervals we get 50 quotes × 12/hour = 600 fetches/hour.
// Top 200 tickers refresh every ~20 min worst case, ~10 min avg.
func CronRouter(ctx context.Context, fsClient *firestore.Client, gcsBucket *storage.BucketHandle, fh *FinnhubClient) map[string]error {
	if !cronMu.TryLock() {
		log.Println("CRON: previous tick still running, skipping")
		return map[string]error{"skipped": nil}
	}
	defer cronMu.Unlock()

	now := time.Now().UTC()
	hour := now.Hour()
	minute := now.Minute()

	results := map[string]error{}

	// Always: fetch prices
	results["fetch"] = RunFetchCycle(ctx, fsClient, gcsBucket, fh)

	// 13:00 UTC — universe update (30 min before US market open)
	if hour == 13 && minute < 5 {
		log.Println("CRON: daily universe update")
		results["universe"] = RunUniverseUpdate(ctx, gcsBucket, fh)
	}

	// 13:05-13:55 UTC — fundamentals (after universe, before market open)
	// RunFundamentalsCycle has its own "already checked today" guard
	if hour == 13 && minute >= 5 {
		log.Println("CRON: daily fundamentals update")
		results["fundamentals"] = RunFundamentalsCycle(ctx, fsClient, gcsBucket, fh)
	}

	// Monday 14:00 UTC — weekly ATH refresh (after fundamentals, uses 52wk high + CoinGecko)
	if now.Weekday() == time.Monday && hour == 14 && minute < 5 {
		log.Println("CRON: weekly ATH refresh")
		results["ath"] = RunATHRefresh(ctx, fsClient, gcsBucket, fh)
	}

	// 13:15 UTC — daily health check (after universe + fundamentals kick off)
	if hour == 13 && minute >= 15 && minute < 20 {
		log.Println("CRON: daily health check")
		results["health"] = RunHealthCheck(ctx, fsClient, telegramBotToken, telegramChatID)
	}

	// 00:00 UTC — midnight subscriber count reset
	if hour == 0 && minute < 5 {
		log.Println("CRON: midnight subscriber rotation")
		results["reset"] = RotateSubscriberCounts(ctx, fsClient)
	}

	return results
}

// RotateSubscriberCounts pushes today's count into the 7-day history and resets today to 0.
// History: [0]=yesterday, [1]=2 days ago, ... up to 6 entries (7 days total including today).
func RotateSubscriberCounts(ctx context.Context, fsClient *firestore.Client) error {
	iter := fsClient.Collection("watched_symbols").Documents(ctx)
	batch := fsClient.Batch()
	count := 0

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}

		var ws WatchedSymbol
		if err := doc.DataTo(&ws); err != nil {
			continue
		}

		// Prepend today's count to history, cap at 6 entries (+ today = 7 days)
		history := append([]int{ws.SubscribersToday}, ws.SubscribersHistory...)
		if len(history) > 6 {
			history = history[:6]
		}

		batch.Set(doc.Ref, map[string]interface{}{
			"subscribers_today":   0,
			"subscribers_history": history,
		}, firestore.MergeAll)
		count++

		// Firestore batches max 500 writes
		if count%500 == 0 {
			if _, err := batch.Commit(ctx); err != nil {
				return err
			}
			batch = fsClient.Batch()
		}
	}

	if count%500 != 0 {
		if _, err := batch.Commit(ctx); err != nil {
			return err
		}
	}

	log.Printf("Rotated subscriber counts for %d symbols", count)
	return nil
}
