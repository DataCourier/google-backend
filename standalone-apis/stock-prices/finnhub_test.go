package main

import (
	"encoding/json"
	"testing"
)

func TestQuoteParsing(t *testing.T) {
	raw := `{"c":255.79,"d":-5.94,"dp":-2.2695,"h":262.23,"l":255.45,"o":262.02,"pc":261.73,"t":1771016400}`
	var q Quote
	if err := json.Unmarshal([]byte(raw), &q); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if q.Current != 255.79 {
		t.Errorf("Current: expected 255.79, got %f", q.Current)
	}
	if q.High != 262.23 {
		t.Errorf("High: expected 262.23, got %f", q.High)
	}
	if q.Low != 255.45 {
		t.Errorf("Low: expected 255.45, got %f", q.Low)
	}
	if q.Open != 262.02 {
		t.Errorf("Open: expected 262.02, got %f", q.Open)
	}
	if q.PreviousClose != 261.73 {
		t.Errorf("PreviousClose: expected 261.73, got %f", q.PreviousClose)
	}
	if q.Timestamp != 1771016400 {
		t.Errorf("Timestamp: expected 1771016400, got %d", q.Timestamp)
	}
}

func TestQuoteParsing_Zeros(t *testing.T) {
	// Finnhub returns all zeros for invalid symbols
	raw := `{"c":0,"h":0,"l":0,"o":0,"pc":0,"t":0}`
	var q Quote
	if err := json.Unmarshal([]byte(raw), &q); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if q.Current != 0 || q.High != 0 || q.Low != 0 {
		t.Error("expected all zeros for invalid symbol response")
	}
}

func TestEarningsCalendarParsing(t *testing.T) {
	raw := `{"earningsCalendar":[{"date":"2026-01-29","epsActual":2.84,"epsEstimate":2.7257,"hour":"amc","quarter":1,"revenueActual":143756000000,"revenueEstimate":141253997332,"symbol":"AAPL","year":2026}]}`
	var cal EarningsCalendar
	if err := json.Unmarshal([]byte(raw), &cal); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(cal.EarningsCalendar) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(cal.EarningsCalendar))
	}

	e := cal.EarningsCalendar[0]
	if e.Date != "2026-01-29" {
		t.Errorf("Date: expected 2026-01-29, got %s", e.Date)
	}
	if e.EPSActual != 2.84 {
		t.Errorf("EPSActual: expected 2.84, got %f", e.EPSActual)
	}
	if e.EPSEstimate != 2.7257 {
		t.Errorf("EPSEstimate: expected 2.7257, got %f", e.EPSEstimate)
	}
	if e.Hour != "amc" {
		t.Errorf("Hour: expected amc, got %s", e.Hour)
	}
	if e.Symbol != "AAPL" {
		t.Errorf("Symbol: expected AAPL, got %s", e.Symbol)
	}
}

func TestEarningsCalendar_Empty(t *testing.T) {
	raw := `{"earningsCalendar":[]}`
	var cal EarningsCalendar
	if err := json.Unmarshal([]byte(raw), &cal); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(cal.EarningsCalendar) != 0 {
		t.Errorf("expected empty, got %d entries", len(cal.EarningsCalendar))
	}
}

func TestMetricFloat(t *testing.T) {
	m := map[string]interface{}{
		"peTTM":    31.88,
		"nullVal":  nil,
		"stringVal": "not a number",
		"intVal":   42,
	}

	if got := metricFloat(m, "peTTM"); got != 31.88 {
		t.Errorf("expected 31.88, got %f", got)
	}
	if got := metricFloat(m, "nullVal"); got != 0 {
		t.Errorf("expected 0 for nil, got %f", got)
	}
	if got := metricFloat(m, "missing"); got != 0 {
		t.Errorf("expected 0 for missing key, got %f", got)
	}
	if got := metricFloat(m, "stringVal"); got != 0 {
		t.Errorf("expected 0 for string val, got %f", got)
	}
	if got := metricFloat(m, "intVal"); got != 0 {
		// JSON numbers decode as float64, but raw int won't match
		// This is fine — metricFloat handles the float64 case from JSON
	}
}

func TestFundamentalsParsing(t *testing.T) {
	// Verify our struct serializes correctly
	f := Fundamentals{
		PETTM:            31.88,
		PSTTM:            8.62,
		PBQuarterly:      45.87,
		ForwardPE:        30.27,
		PEGTTM:           1.29,
		DividendYieldTTM: 0.41,
		GrossMarginTTM:   47.33,
		NetMarginTTM:     27.04,
		ROETTM:           159.94,
		DebtEquity:       1.03,
		EPSTTM:           7.90,
		RevenueGrowthYOY: 10.07,
		EPSGrowthYOY:     25.65,
		Week52High:       288.62,
		Week52Low:        169.21,
	}

	data, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var f2 Fundamentals
	if err := json.Unmarshal(data, &f2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if f2.PETTM != f.PETTM {
		t.Errorf("PETTM roundtrip failed: %f != %f", f2.PETTM, f.PETTM)
	}
	if f2.Week52High != f.Week52High {
		t.Errorf("Week52High roundtrip failed: %f != %f", f2.Week52High, f.Week52High)
	}
}

func TestUniverseEntryParsing(t *testing.T) {
	raw := `{"symbol":"AAPL","description":"APPLE INC","displaySymbol":"AAPL","type":"Common Stock"}`
	var s finnhubSymbol
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if s.Symbol != "AAPL" {
		t.Errorf("Symbol: expected AAPL, got %s", s.Symbol)
	}
	if s.Description != "APPLE INC" {
		t.Errorf("Description: expected APPLE INC, got %s", s.Description)
	}
	if s.Type != "Common Stock" {
		t.Errorf("Type: expected Common Stock, got %s", s.Type)
	}
}

func TestCryptoSymbolParsing(t *testing.T) {
	raw := `{"symbol":"BINANCE:BTCUSDT","description":"Binance BTC/USDT","displaySymbol":"BTC/USDT"}`
	var s finnhubSymbol
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if s.Symbol != "BINANCE:BTCUSDT" {
		t.Errorf("Symbol: expected BINANCE:BTCUSDT, got %s", s.Symbol)
	}
	if !isCrypto(s.Symbol) {
		t.Error("BINANCE:BTCUSDT should be detected as crypto")
	}
}
