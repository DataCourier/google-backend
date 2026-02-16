package main

import (
	"testing"
)

func TestExtractBaseSymbol(t *testing.T) {
	cases := []struct {
		symbol string
		expect string
	}{
		{"BINANCE:BTCUSDT", "BTC"},
		{"BINANCE:ETHUSDT", "ETH"},
		{"BINANCE:SOLUSDT", "SOL"},
		{"BINANCE:DOGEUSDT", "DOGE"},
		{"BINANCE:BNBUSDT", "BNB"},
		{"COINBASE:BTC-USD", "BTC"},
		{"AAPL", ""},             // not crypto
	}
	for _, c := range cases {
		got := extractBaseSymbol(c.symbol)
		if got != c.expect {
			t.Errorf("extractBaseSymbol(%q) = %q, want %q", c.symbol, got, c.expect)
		}
	}
}

func TestExtractBaseSymbol_USD(t *testing.T) {
	// COINBASE uses USD suffix
	got := extractBaseSymbol("COINBASE:BTCUSD")
	if got != "BTC" {
		t.Errorf("expected BTC, got %q", got)
	}
}

func TestCoinGeckoIDMapping(t *testing.T) {
	// Verify key mappings exist
	required := []string{"BTC", "ETH", "SOL", "BNB", "XRP", "ADA", "DOGE"}
	for _, sym := range required {
		if _, ok := coinGeckoIDs[sym]; !ok {
			t.Errorf("missing CoinGecko mapping for %s", sym)
		}
	}
}
