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

	var mu sync.Mutex
	var order []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")
		mu.Lock()
		order = append(order, symbol)
		mu.Unlock()

		payload := map[string]map[string]string{
			"Global Quote": {
				"08. previous close": prices[symbol],
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
	if snapshot["SPY"] != prices["SPY"] {
		t.Fatalf("expected SPY price %s, got %s", prices["SPY"], snapshot["SPY"])
	}
	if snapshot["AAPL"] != prices["AAPL"] {
		t.Fatalf("expected AAPL price %s, got %s", prices["AAPL"], snapshot["AAPL"])
	}
	if snapshot["MSFT"] != prices["MSFT"] {
		t.Fatalf("expected MSFT price %s, got %s", prices["MSFT"], snapshot["MSFT"])
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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")
		payload := map[string]map[string]string{
			"Global Quote": {
				"08. previous close": prices[symbol],
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
