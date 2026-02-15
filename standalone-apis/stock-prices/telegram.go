package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

func SendTelegram(botToken, chatID, message string) error {
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	resp, err := http.PostForm(endpoint, url.Values{
		"chat_id": {chatID},
		"text":    {message},
	})
	if err != nil {
		return fmt.Errorf("telegram send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram returned %d", resp.StatusCode)
	}
	return nil
}

func RunHealthCheck(ctx context.Context, fsClient *firestore.Client, botToken, chatID string) error {
	if botToken == "" || chatID == "" {
		log.Println("HEALTH: skipping, no Telegram credentials")
		return nil
	}

	iter := fsClient.Collection("watched_symbols").Documents(ctx)

	var total, active, stale int
	type tickerSubs struct {
		symbol string
		peak   int
	}
	var top []tickerSubs
	now := time.Now()

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("read watched_symbols: %w", err)
		}

		var ws WatchedSymbol
		if err := doc.DataTo(&ws); err != nil {
			continue
		}

		peak := ws.subscriberPeak()
		if peak <= 0 {
			continue
		}

		total++
		top = append(top, tickerSubs{ws.Symbol, peak})

		staleness := now.Sub(time.UnixMilli(ws.LastFetchedAt))
		if staleness < 10*time.Minute {
			active++
		} else if staleness > 20*time.Minute {
			stale++
		}
	}

	sort.Slice(top, func(i, j int) bool { return top[i].peak > top[j].peak })
	if len(top) > 5 {
		top = top[:5]
	}

	var topStr []string
	for _, t := range top {
		topStr = append(topStr, fmt.Sprintf("%s(%d)", t.symbol, t.peak))
	}

	msg := fmt.Sprintf("📊 Stock Widget Health\n━━━━━━━━━━━━━━━━━━\nWatched: %d symbols\nActive (fetched <10m): %d\nStale (>20m): %d", total, active, stale)
	if stale > 0 {
		msg += " ⚠️"
	}
	if len(topStr) > 0 {
		msg += fmt.Sprintf("\nTop: %s", strings.Join(topStr, " "))
	}

	log.Printf("HEALTH: sending Telegram report")
	return SendTelegram(botToken, chatID, msg)
}
