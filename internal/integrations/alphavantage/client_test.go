package alphavantage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/igor-kupczynski/alpha-monday/internal/integrations/retry"
)

func TestFetchPreviousCloseRetriesOnServerError(t *testing.T) {
	server, calls := alphaTestServer([]alphaResponse{
		{status: http.StatusInternalServerError, body: `{"error":"oops"}`},
		{status: http.StatusBadGateway, body: `{"error":"oops"}`},
		{status: http.StatusOK, body: alphaQuoteResponse("SPY", "123.45", "2026-01-30")},
	})
	defer server.Close()

	client := NewClient("test-key",
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
		WithRetryConfig(retry.Config{MaxAttempts: 3, BaseDelay: 0, MaxDelay: 0, Jitter: 0}),
	)

	quote, err := client.FetchPreviousClose(context.Background(), "SPY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if quote.PreviousClose != "123.45" {
		t.Fatalf("expected previous close, got %q", quote.PreviousClose)
	}
	if calls.Load() != 3 {
		t.Fatalf("expected 3 attempts, got %d", calls.Load())
	}
}

func TestFetchPreviousCloseNoRetryOnBadRequest(t *testing.T) {
	server, calls := alphaTestServer([]alphaResponse{
		{status: http.StatusBadRequest, body: `{"error":"bad request"}`},
		{status: http.StatusOK, body: alphaQuoteResponse("SPY", "123.45", "2026-01-30")},
	})
	defer server.Close()

	client := NewClient("test-key",
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
		WithRetryConfig(retry.Config{MaxAttempts: 3, BaseDelay: 0, MaxDelay: 0, Jitter: 0}),
	)

	_, err := client.FetchPreviousClose(context.Background(), "SPY")
	if err == nil {
		t.Fatalf("expected error for bad request")
	}
	if calls.Load() != 1 {
		t.Fatalf("expected 1 attempt, got %d", calls.Load())
	}
}

type alphaResponse struct {
	status int
	body   string
}

func alphaTestServer(responses []alphaResponse) (*httptest.Server, *atomic.Int32) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idx := int(calls.Add(1)) - 1
		if idx >= len(responses) {
			idx = len(responses) - 1
		}
		resp := responses[idx]
		if resp.status == 0 {
			resp.status = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.status)
		_, _ = w.Write([]byte(resp.body))
	}))
	return server, &calls
}

func alphaQuoteResponse(symbol, prevClose, tradingDay string) string {
	payload := map[string]map[string]string{
		"Global Quote": {
			"01. symbol":             symbol,
			"07. latest trading day": tradingDay,
			"08. previous close":     prevClose,
		},
	}
	data, _ := json.Marshal(payload)
	return string(data)
}
