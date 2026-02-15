package main

import (
	"testing"
	"time"
)

// --- subscriberPeak ---

func TestSubscriberPeak_TodayOnly(t *testing.T) {
	ws := WatchedSymbol{SubscribersToday: 42}
	if got := ws.subscriberPeak(); got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
}

func TestSubscriberPeak_HistoryHigher(t *testing.T) {
	ws := WatchedSymbol{
		SubscribersToday:   10,
		SubscribersHistory: []int{200, 150, 100},
	}
	if got := ws.subscriberPeak(); got != 200 {
		t.Errorf("expected 200, got %d", got)
	}
}

func TestSubscriberPeak_TodayHighest(t *testing.T) {
	ws := WatchedSymbol{
		SubscribersToday:   500,
		SubscribersHistory: []int{200, 150, 100},
	}
	if got := ws.subscriberPeak(); got != 500 {
		t.Errorf("expected 500, got %d", got)
	}
}

func TestSubscriberPeak_AllZero(t *testing.T) {
	ws := WatchedSymbol{
		SubscribersToday:   0,
		SubscribersHistory: []int{0, 0, 0, 0, 0, 0},
	}
	if got := ws.subscriberPeak(); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestSubscriberPeak_OneQuietDay(t *testing.T) {
	// Today is quiet but last week was hot — should stay in fast lane
	ws := WatchedSymbol{
		SubscribersToday:   0,
		SubscribersHistory: []int{200, 190, 180, 170, 160, 150},
	}
	if got := ws.subscriberPeak(); got != 200 {
		t.Errorf("expected 200 (peak from history), got %d", got)
	}
}

func TestSubscriberPeak_EmptyHistory(t *testing.T) {
	ws := WatchedSymbol{SubscribersToday: 5}
	if got := ws.subscriberPeak(); got != 5 {
		t.Errorf("expected 5, got %d", got)
	}
}

func TestSubscriberPeak_DecayingInterest(t *testing.T) {
	// Interest fading over the week
	ws := WatchedSymbol{
		SubscribersToday:   2,
		SubscribersHistory: []int{5, 10, 20, 50, 100, 200},
	}
	// Peak is still 200 from 6 days ago
	if got := ws.subscriberPeak(); got != 200 {
		t.Errorf("expected 200, got %d", got)
	}
}

// --- targetFreshness ---

func TestTargetFreshness_LowSubscribers(t *testing.T) {
	cases := []struct {
		subs     int
		expected time.Duration
	}{
		{1, 15 * time.Minute},
		{2, 15 * time.Minute},
	}
	for _, c := range cases {
		if got := targetFreshness(c.subs); got != c.expected {
			t.Errorf("subs=%d: expected %v, got %v", c.subs, c.expected, got)
		}
	}
}

func TestTargetFreshness_MediumSubscribers(t *testing.T) {
	cases := []struct {
		subs     int
		expected time.Duration
	}{
		{3, 8 * time.Minute},
		{5, 8 * time.Minute},
		{9, 8 * time.Minute},
	}
	for _, c := range cases {
		if got := targetFreshness(c.subs); got != c.expected {
			t.Errorf("subs=%d: expected %v, got %v", c.subs, c.expected, got)
		}
	}
}

func TestTargetFreshness_HighSubscribers(t *testing.T) {
	cases := []struct {
		subs     int
		expected time.Duration
	}{
		{10, 4 * time.Minute},
		{50, 4 * time.Minute},
		{1000, 4 * time.Minute},
	}
	for _, c := range cases {
		if got := targetFreshness(c.subs); got != c.expected {
			t.Errorf("subs=%d: expected %v, got %v", c.subs, c.expected, got)
		}
	}
}

// --- isCrypto ---

func TestIsCrypto(t *testing.T) {
	cases := []struct {
		symbol string
		expect bool
	}{
		{"AAPL", false},
		{"MSFT", false},
		{"BINANCE:BTCUSDT", true},
		{"BINANCE:ETHUSDT", true},
		{"COINBASE:BTC-USD", true},
	}
	for _, c := range cases {
		if got := isCrypto(c.symbol); got != c.expect {
			t.Errorf("isCrypto(%q) = %v, want %v", c.symbol, got, c.expect)
		}
	}
}

// --- isMarketOpen ---

// Stock hours: weekdays 8:00-00:00 UTC (4am-8pm ET = premarket+market+aftermarket)

func TestStockHours_Weekday_DuringMarket(t *testing.T) {
	now := time.Date(2026, 2, 18, 15, 0, 0, 0, time.UTC) // Wed 15:00
	if !isStockHoursAt(now) {
		t.Error("expected stock hours at Wed 15:00 UTC")
	}
}

func TestStockHours_Weekday_Premarket(t *testing.T) {
	now := time.Date(2026, 2, 18, 9, 0, 0, 0, time.UTC) // Wed 9:00 = 4am ET premarket
	if !isStockHoursAt(now) {
		t.Error("expected stock hours during premarket at Wed 9:00 UTC")
	}
}

func TestStockHours_Weekday_Aftermarket(t *testing.T) {
	now := time.Date(2026, 2, 18, 23, 0, 0, 0, time.UTC) // Wed 23:00 = 6pm ET aftermarket
	if !isStockHoursAt(now) {
		t.Error("expected stock hours during aftermarket at Wed 23:00 UTC")
	}
}

func TestStockHours_Weekday_AtOpen(t *testing.T) {
	now := time.Date(2026, 2, 18, 8, 0, 0, 0, time.UTC) // Wed 8:00 = 3am ET (start)
	if !isStockHoursAt(now) {
		t.Error("expected stock hours at Wed 8:00 UTC")
	}
}

func TestStockHours_Weekday_Overnight(t *testing.T) {
	now := time.Date(2026, 2, 18, 5, 0, 0, 0, time.UTC) // Wed 5:00 = midnight ET
	if isStockHoursAt(now) {
		t.Error("expected no stock hours at Wed 5:00 UTC (overnight)")
	}
}

func TestStockHours_Saturday(t *testing.T) {
	now := time.Date(2026, 2, 14, 15, 0, 0, 0, time.UTC)
	if isStockHoursAt(now) {
		t.Error("expected no stock hours on Saturday")
	}
}

func TestStockHours_Sunday(t *testing.T) {
	now := time.Date(2026, 2, 15, 15, 0, 0, 0, time.UTC)
	if isStockHoursAt(now) {
		t.Error("expected no stock hours on Sunday")
	}
}

// --- Priority sorting ---

func TestPrioritySorting_StaleHighSubsFirst(t *testing.T) {
	// A hot ticker that's slightly overdue should beat a cold ticker that's very overdue
	hot := WatchedSymbol{
		Symbol:           "AAPL",
		SubscribersToday: 100,
		LastFetchedAt:    time.Now().Add(-5 * time.Minute).UnixMilli(), // 5 min stale, target 4 min → ratio 1.25
	}
	cold := WatchedSymbol{
		Symbol:           "OBSCURE",
		SubscribersToday: 1,
		LastFetchedAt:    time.Now().Add(-14 * time.Minute).UnixMilli(), // 14 min stale, target 15 min → ratio 0.93
	}

	hotTarget := targetFreshness(hot.subscriberPeak())
	coldTarget := targetFreshness(cold.subscriberPeak())

	hotStaleness := time.Since(time.UnixMilli(hot.LastFetchedAt))
	coldStaleness := time.Since(time.UnixMilli(cold.LastFetchedAt))

	hotPriority := float64(hotStaleness) / float64(hotTarget)
	coldPriority := float64(coldStaleness) / float64(coldTarget)

	if hotPriority <= coldPriority {
		t.Errorf("hot ticker (%.2f) should have higher priority than cold (%.2f)", hotPriority, coldPriority)
	}
}

func TestPrioritySorting_BothOverdue_MostOverdueFirst(t *testing.T) {
	// Two tickers in the same tier, both overdue — more stale one should win
	a := WatchedSymbol{
		Symbol:           "AAPL",
		SubscribersToday: 20,
		LastFetchedAt:    time.Now().Add(-10 * time.Minute).UnixMilli(), // 10/4 = 2.5
	}
	b := WatchedSymbol{
		Symbol:           "MSFT",
		SubscribersToday: 20,
		LastFetchedAt:    time.Now().Add(-6 * time.Minute).UnixMilli(), // 6/4 = 1.5
	}

	aTarget := targetFreshness(a.subscriberPeak())
	bTarget := targetFreshness(b.subscriberPeak())

	aPriority := float64(time.Since(time.UnixMilli(a.LastFetchedAt))) / float64(aTarget)
	bPriority := float64(time.Since(time.UnixMilli(b.LastFetchedAt))) / float64(bTarget)

	if aPriority <= bPriority {
		t.Errorf("more stale ticker (%.2f) should have higher priority than less stale (%.2f)", aPriority, bPriority)
	}
}

// --- Tier stability with peak ---

func TestTierStability_QuietDayDoesntDowngrade(t *testing.T) {
	// Had 200 subs 3 days ago, today is 0
	ws := WatchedSymbol{
		SubscribersToday:   0,
		SubscribersHistory: []int{50, 100, 200},
	}

	peak := ws.subscriberPeak()
	tier := targetFreshness(peak)

	if tier != 4*time.Minute {
		t.Errorf("expected 4min tier (peak=200), got %v (peak=%d)", tier, peak)
	}
}

func TestTierStability_FullWeekZeroDropsOff(t *testing.T) {
	// Entire week is zero — should not be fetched
	ws := WatchedSymbol{
		SubscribersToday:   0,
		SubscribersHistory: []int{0, 0, 0, 0, 0, 0},
	}

	if ws.subscriberPeak() != 0 {
		t.Error("expected peak 0 for fully inactive symbol")
	}
}

func TestTierStability_GradualDecay(t *testing.T) {
	// Decaying from 200 over a week — peak still holds at 200
	ws := WatchedSymbol{
		SubscribersToday:   2,
		SubscribersHistory: []int{5, 10, 50, 100, 150, 200},
	}

	if ws.subscriberPeak() != 200 {
		t.Errorf("expected peak 200, got %d", ws.subscriberPeak())
	}

	// After the 200 falls off (next rotation), peak drops to 150
	ws2 := WatchedSymbol{
		SubscribersToday:   1,
		SubscribersHistory: []int{2, 5, 10, 50, 100, 150},
	}

	if ws2.subscriberPeak() != 150 {
		t.Errorf("expected peak 150 after old data falls off, got %d", ws2.subscriberPeak())
	}
}
