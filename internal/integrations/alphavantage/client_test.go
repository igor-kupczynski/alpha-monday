package alphavantage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestSnapshotPreviousClosesSuccess(t *testing.T) {
	prices := map[string]string{
		"SPY":  "401.25",
		"AAPL": "178.10",
		"MSFT": "342.55",
	}
	tradingDays := map[string]string{
		"SPY":  "2026-01-03",
		"AAPL": "2026-01-03",
		"MSFT": "2026-01-03",
	}

	var mu sync.Mutex
	var order []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")
		mu.Lock()
		order = append(order, symbol)
		mu.Unlock()

		payload := map[string]map[string]string{
			"Global Quote": {
				"08. previous close":     prices[symbol],
				"07. latest trading day": tradingDays[symbol],
			},
		}
		data, _ := json.Marshal(payload)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
	defer server.Close()

	client := NewClient("test-key",
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
	)

	snapshot, err := client.SnapshotPreviousCloses(context.Background(), "SPY", []string{"AAPL", "MSFT"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snapshot["SPY"].PreviousClose != prices["SPY"] {
		t.Fatalf("expected SPY price %s, got %s", prices["SPY"], snapshot["SPY"].PreviousClose)
	}
	if snapshot["SPY"].TradingDay != tradingDays["SPY"] {
		t.Fatalf("expected SPY trading day %s, got %s", tradingDays["SPY"], snapshot["SPY"].TradingDay)
	}
	if snapshot["AAPL"].PreviousClose != prices["AAPL"] {
		t.Fatalf("expected AAPL price %s, got %s", prices["AAPL"], snapshot["AAPL"].PreviousClose)
	}
	if snapshot["AAPL"].TradingDay != tradingDays["AAPL"] {
		t.Fatalf("expected AAPL trading day %s, got %s", tradingDays["AAPL"], snapshot["AAPL"].TradingDay)
	}
	if snapshot["MSFT"].PreviousClose != prices["MSFT"] {
		t.Fatalf("expected MSFT price %s, got %s", prices["MSFT"], snapshot["MSFT"].PreviousClose)
	}
	if snapshot["MSFT"].TradingDay != tradingDays["MSFT"] {
		t.Fatalf("expected MSFT trading day %s, got %s", tradingDays["MSFT"], snapshot["MSFT"].TradingDay)
	}
	if len(order) == 0 || order[0] != "SPY" {
		t.Fatalf("expected SPY to be fetched first, got %v", order)
	}
}

func TestSnapshotPreviousClosesMissingPriceFails(t *testing.T) {
	prices := map[string]string{
		"SPY":  "401.25",
		"AAPL": "",
	}
	tradingDays := map[string]string{
		"SPY":  "2026-01-03",
		"AAPL": "2026-01-03",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")
		payload := map[string]map[string]string{
			"Global Quote": {
				"08. previous close":     prices[symbol],
				"07. latest trading day": tradingDays[symbol],
			},
		}
		data, _ := json.Marshal(payload)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
	defer server.Close()

	client := NewClient("test-key",
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
	)

	_, err := client.SnapshotPreviousCloses(context.Background(), "SPY", []string{"AAPL"})
	if err == nil {
		t.Fatalf("expected error for missing previous close")
	}
}
