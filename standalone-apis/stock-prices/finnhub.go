package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type FinnhubClient struct {
	apiKey     string
	httpClient *http.Client
	ticker     *time.Ticker // rate limiter: 1 request per tick
}

type Quote struct {
	Current       float64 `json:"c"`
	High          float64 `json:"h"`
	Low           float64 `json:"l"`
	Open          float64 `json:"o"`
	PreviousClose float64 `json:"pc"`
	Timestamp     int64   `json:"t"`
}

func NewFinnhubClient(apiKey string) *FinnhubClient {
	return &FinnhubClient{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		// 50 requests per minute → 1 request per 1.2s
		ticker: time.NewTicker(1200 * time.Millisecond),
	}
}

func (c *FinnhubClient) GetQuote(symbol string) (*Quote, error) {
	// Wait for rate limiter
	<-c.ticker.C

	url := fmt.Sprintf("https://finnhub.io/api/v1/quote?symbol=%s&token=%s", symbol, c.apiKey)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("finnhub request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("finnhub rate limited")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("finnhub returned status %d", resp.StatusCode)
	}

	var q Quote
	if err := json.NewDecoder(resp.Body).Decode(&q); err != nil {
		return nil, fmt.Errorf("finnhub decode failed: %w", err)
	}

	// Finnhub returns all zeros for invalid symbols
	if q.Current == 0 && q.High == 0 && q.Low == 0 {
		return nil, fmt.Errorf("no data for symbol %s", symbol)
	}

	return &q, nil
}

// Earnings calendar response
type EarningsCalendar struct {
	EarningsCalendar []EarningsEntry `json:"earningsCalendar"`
}

type EarningsEntry struct {
	Date            string  `json:"date"`
	EPSActual       float64 `json:"epsActual"`
	EPSEstimate     float64 `json:"epsEstimate"`
	Hour            string  `json:"hour"` // "bmo" (before market open), "amc" (after market close)
	Quarter         int     `json:"quarter"`
	RevenueActual   int64   `json:"revenueActual"`
	RevenueEstimate int64   `json:"revenueEstimate"`
	Symbol          string  `json:"symbol"`
	Year            int     `json:"year"`
}

func (c *FinnhubClient) GetEarnings(symbol string) (*EarningsCalendar, error) {
	<-c.ticker.C

	url := fmt.Sprintf("https://finnhub.io/api/v1/calendar/earnings?symbol=%s&token=%s", symbol, c.apiKey)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("finnhub earnings request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("finnhub rate limited")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("finnhub returned status %d", resp.StatusCode)
	}

	var cal EarningsCalendar
	if err := json.NewDecoder(resp.Body).Decode(&cal); err != nil {
		return nil, fmt.Errorf("finnhub earnings decode failed: %w", err)
	}

	return &cal, nil
}

// Basic metrics response — we extract what we need from the huge response
type MetricsResponse struct {
	Metric map[string]interface{} `json:"metric"`
}

type Fundamentals struct {
	PETTM            float64 `json:"pe_ttm"`
	PSTTM            float64 `json:"ps_ttm"`
	PBQuarterly      float64 `json:"pb_quarterly"`
	ForwardPE        float64 `json:"forward_pe"`
	PEGTTM           float64 `json:"peg_ttm"`
	DividendYieldTTM float64 `json:"dividend_yield_ttm"`
	GrossMarginTTM   float64 `json:"gross_margin_ttm"`
	NetMarginTTM     float64 `json:"net_margin_ttm"`
	ROETTM           float64 `json:"roe_ttm"`
	DebtEquity       float64 `json:"debt_equity"`
	EPSTTM           float64 `json:"eps_ttm"`
	RevenueGrowthYOY float64 `json:"revenue_growth_yoy"`
	EPSGrowthYOY     float64 `json:"eps_growth_yoy"`
	Week52High       float64 `json:"week52_high"`
	Week52Low        float64 `json:"week52_low"`
}

func (c *FinnhubClient) GetMetrics(symbol string) (*Fundamentals, error) {
	<-c.ticker.C

	url := fmt.Sprintf("https://finnhub.io/api/v1/stock/metric?symbol=%s&metric=all&token=%s", symbol, c.apiKey)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("finnhub metrics request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("finnhub rate limited")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("finnhub returned status %d", resp.StatusCode)
	}

	var mr MetricsResponse
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil, fmt.Errorf("finnhub metrics decode failed: %w", err)
	}

	f := &Fundamentals{}
	f.PETTM = metricFloat(mr.Metric, "peTTM")
	f.PSTTM = metricFloat(mr.Metric, "psTTM")
	f.PBQuarterly = metricFloat(mr.Metric, "pbQuarterly")
	f.ForwardPE = metricFloat(mr.Metric, "forwardPE")
	f.PEGTTM = metricFloat(mr.Metric, "pegTTM")
	f.DividendYieldTTM = metricFloat(mr.Metric, "currentDividendYieldTTM")
	f.GrossMarginTTM = metricFloat(mr.Metric, "grossMarginTTM")
	f.NetMarginTTM = metricFloat(mr.Metric, "netProfitMarginTTM")
	f.ROETTM = metricFloat(mr.Metric, "roeTTM")
	f.DebtEquity = metricFloat(mr.Metric, "totalDebt/totalEquityQuarterly")
	f.EPSTTM = metricFloat(mr.Metric, "epsTTM")
	f.RevenueGrowthYOY = metricFloat(mr.Metric, "revenueGrowthTTMYoy")
	f.EPSGrowthYOY = metricFloat(mr.Metric, "epsGrowthTTMYoy")
	f.Week52High = metricFloat(mr.Metric, "52WeekHigh")
	f.Week52Low = metricFloat(mr.Metric, "52WeekLow")

	return f, nil
}

func metricFloat(m map[string]interface{}, key string) float64 {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	default:
		return 0
	}
}
