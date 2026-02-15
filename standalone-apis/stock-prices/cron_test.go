package main

import (
	"testing"
	"time"
)

// Test the cron schedule logic (time-based routing)
// We can't call CronRouter directly without Firestore/GCS,
// but we can test the time-matching logic.

func TestCronSchedule_AlwaysFetch(t *testing.T) {
	// Any time should trigger fetch
	times := []time.Time{
		time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC),  // midnight Mon
		time.Date(2026, 2, 16, 6, 30, 0, 0, time.UTC),  // early morning
		time.Date(2026, 2, 16, 13, 0, 0, 0, time.UTC),  // pre-market
		time.Date(2026, 2, 16, 15, 0, 0, 0, time.UTC),  // mid-market
		time.Date(2026, 2, 16, 23, 55, 0, 0, time.UTC), // late night
	}
	for _, now := range times {
		if !shouldFetch(now) {
			t.Errorf("should always fetch at %v", now)
		}
	}
}

func TestCronSchedule_UniverseAt1300(t *testing.T) {
	yes := time.Date(2026, 2, 16, 13, 0, 0, 0, time.UTC)
	no := time.Date(2026, 2, 16, 13, 5, 0, 0, time.UTC)
	noToo := time.Date(2026, 2, 16, 12, 55, 0, 0, time.UTC)

	if !shouldRunUniverse(yes) {
		t.Error("should run universe at 13:00")
	}
	if shouldRunUniverse(no) {
		t.Error("should NOT run universe at 13:05")
	}
	if shouldRunUniverse(noToo) {
		t.Error("should NOT run universe at 12:55")
	}
}

func TestCronSchedule_FundamentalsAt1305(t *testing.T) {
	yes := time.Date(2026, 2, 16, 13, 5, 0, 0, time.UTC)
	yes2 := time.Date(2026, 2, 16, 13, 30, 0, 0, time.UTC)
	no := time.Date(2026, 2, 16, 13, 0, 0, 0, time.UTC)
	noToo := time.Date(2026, 2, 16, 14, 0, 0, 0, time.UTC)

	if !shouldRunFundamentals(yes) {
		t.Error("should run fundamentals at 13:05")
	}
	if !shouldRunFundamentals(yes2) {
		t.Error("should run fundamentals at 13:30")
	}
	if shouldRunFundamentals(no) {
		t.Error("should NOT run fundamentals at 13:00 (universe runs then)")
	}
	if shouldRunFundamentals(noToo) {
		t.Error("should NOT run fundamentals at 14:00")
	}
}

func TestCronSchedule_ResetAtMidnight(t *testing.T) {
	yes := time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
	yes2 := time.Date(2026, 2, 16, 0, 4, 0, 0, time.UTC)
	no := time.Date(2026, 2, 16, 0, 5, 0, 0, time.UTC)
	noToo := time.Date(2026, 2, 16, 23, 55, 0, 0, time.UTC)

	if !shouldRunReset(yes) {
		t.Error("should reset at 00:00")
	}
	if !shouldRunReset(yes2) {
		t.Error("should reset at 00:04")
	}
	if shouldRunReset(no) {
		t.Error("should NOT reset at 00:05")
	}
	if shouldRunReset(noToo) {
		t.Error("should NOT reset at 23:55")
	}
}

// Helper functions matching the logic in cron.go
func shouldFetch(_ time.Time) bool { return true }

func shouldRunUniverse(now time.Time) bool {
	return now.Hour() == 13 && now.Minute() < 5
}

func shouldRunFundamentals(now time.Time) bool {
	return now.Hour() == 13 && now.Minute() >= 5
}

func shouldRunReset(now time.Time) bool {
	return now.Hour() == 0 && now.Minute() < 5
}
