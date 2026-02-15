package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPriceFileJSON(t *testing.T) {
	pf := PriceFile{
		Symbol:        "AAPL",
		Price:         255.79,
		High:          262.23,
		Low:           255.45,
		Open:          262.02,
		PreviousClose: 261.73,
		ATH:           288.62,
		ATHDate:       "2025-12-03",
		OffATHPct:     -11.38,
		FetchedAt:     1771016400000,
	}

	data, err := json.Marshal(pf)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Verify snake_case field names
	s := string(data)
	if !strings.Contains(s, `"previous_close"`) {
		t.Error("expected snake_case previous_close in JSON")
	}
	if !strings.Contains(s, `"ath_date"`) {
		t.Error("expected snake_case ath_date in JSON")
	}
	if !strings.Contains(s, `"off_ath_pct"`) {
		t.Error("expected snake_case off_ath_pct in JSON")
	}
	if !strings.Contains(s, `"fetched_at"`) {
		t.Error("expected snake_case fetched_at in JSON")
	}

	// Earnings and fundamentals should be omitted when nil
	if strings.Contains(s, `"earnings"`) {
		t.Error("earnings should be omitted when nil")
	}
	if strings.Contains(s, `"fundamentals"`) {
		t.Error("fundamentals should be omitted when nil")
	}
}

func TestPriceFileJSON_WithFundamentals(t *testing.T) {
	pf := PriceFile{
		Symbol: "AAPL",
		Price:  255.79,
		Earnings: &EarningsData{
			Date:               "2026-01-29",
			DaysAway:           -17,
			LastEPSActual:      2.84,
			LastEPSEstimate:    2.73,
			LastEPSSurprisePct: 4.03,
		},
		Fundamentals: &Fundamentals{
			PETTM:       31.88,
			PSTTM:       8.62,
			Week52High:  288.62,
			Week52Low:   169.21,
		},
		FundamentalsUpdatedAt: 1704067200000,
	}

	data, err := json.Marshal(pf)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	s := string(data)
	if !strings.Contains(s, `"earnings"`) {
		t.Error("earnings should be present when set")
	}
	if !strings.Contains(s, `"fundamentals"`) {
		t.Error("fundamentals should be present when set")
	}
	if !strings.Contains(s, `"days_away"`) {
		t.Error("expected snake_case days_away")
	}
	if !strings.Contains(s, `"pe_ttm"`) {
		t.Error("expected snake_case pe_ttm")
	}
	if !strings.Contains(s, `"week52_high"`) {
		t.Error("expected snake_case week52_high")
	}
	if !strings.Contains(s, `"fundamentals_updated_at"`) {
		t.Error("expected fundamentals_updated_at when set")
	}
}

func TestPriceFileJSON_Roundtrip(t *testing.T) {
	original := PriceFile{
		Symbol:    "BINANCE:BTCUSDT",
		Price:     69000.50,
		ATH:       69500.00,
		ATHDate:   "2026-02-10",
		OffATHPct: -0.72,
		FetchedAt: 1771016400000,
		Fundamentals: &Fundamentals{
			PETTM:      0, // crypto has no PE
			Week52High: 73000,
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded PriceFile
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Symbol != original.Symbol {
		t.Errorf("Symbol: %s != %s", decoded.Symbol, original.Symbol)
	}
	if decoded.Price != original.Price {
		t.Errorf("Price: %f != %f", decoded.Price, original.Price)
	}
	if decoded.ATH != original.ATH {
		t.Errorf("ATH: %f != %f", decoded.ATH, original.ATH)
	}
	if decoded.Fundamentals.Week52High != original.Fundamentals.Week52High {
		t.Errorf("Week52High: %f != %f", decoded.Fundamentals.Week52High, original.Fundamentals.Week52High)
	}
}

func TestSymbolToFilename(t *testing.T) {
	cases := []struct {
		symbol   string
		expected string
	}{
		{"AAPL", "AAPL"},
		{"BINANCE:BTCUSDT", "BINANCE_BTCUSDT"},
		{"COINBASE:BTC-USD", "COINBASE_BTC-USD"},
	}
	for _, c := range cases {
		got := strings.ReplaceAll(c.symbol, ":", "_")
		if got != c.expected {
			t.Errorf("symbol %q → %q, want %q", c.symbol, got, c.expected)
		}
	}
}
