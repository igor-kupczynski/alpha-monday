package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/igor-kupczynski/alpha-monday/internal/integrations/retry"
)

func TestGeneratePicksInvalidJSONRetries(t *testing.T) {
	server, calls := openAITestServer([]string{
		wrapChatResponse("not json"),
		wrapChatResponse("still not json"),
	})
	defer server.Close()

	client := NewClient("test-key",
		WithEndpoint(server.URL),
		WithHTTPClient(server.Client()),
		WithMaxAttempts(2),
	)

	_, err := client.GeneratePicks(context.Background())
	if err == nil {
		t.Fatalf("expected error for invalid json")
	}
	if calls.Load() != 2 {
		t.Fatalf("expected 2 attempts, got %d", calls.Load())
	}
}

func TestGeneratePicksWrongCountRetries(t *testing.T) {
	content, err := json.Marshal([]Pick{
		{Ticker: "AAPL", Action: "BUY", Reasoning: "ok"},
		{Ticker: "MSFT", Action: "SELL", Reasoning: "ok"},
	})
	if err != nil {
		t.Fatalf("marshal picks: %v", err)
	}

	server, calls := openAITestServer([]string{
		wrapChatResponse(string(content)),
		wrapChatResponse(string(content)),
	})
	defer server.Close()

	client := NewClient("test-key",
		WithEndpoint(server.URL),
		WithHTTPClient(server.Client()),
		WithMaxAttempts(2),
	)

	_, err = client.GeneratePicks(context.Background())
	if err == nil {
		t.Fatalf("expected error for wrong count")
	}
	if calls.Load() != 2 {
		t.Fatalf("expected 2 attempts, got %d", calls.Load())
	}
}

func TestGeneratePicksDuplicateTickersRetries(t *testing.T) {
	content, err := json.Marshal([]Pick{
		{Ticker: "AAPL", Action: "BUY", Reasoning: "ok"},
		{Ticker: "AAPL", Action: "SELL", Reasoning: "dup"},
		{Ticker: "MSFT", Action: "BUY", Reasoning: "ok"},
	})
	if err != nil {
		t.Fatalf("marshal picks: %v", err)
	}

	server, calls := openAITestServer([]string{
		wrapChatResponse(string(content)),
		wrapChatResponse(string(content)),
	})
	defer server.Close()

	client := NewClient("test-key",
		WithEndpoint(server.URL),
		WithHTTPClient(server.Client()),
		WithMaxAttempts(2),
	)

	_, err = client.GeneratePicks(context.Background())
	if err == nil {
		t.Fatalf("expected error for duplicate tickers")
	}
	if calls.Load() != 2 {
		t.Fatalf("expected 2 attempts, got %d", calls.Load())
	}
}

func TestGeneratePicksBadActionRetries(t *testing.T) {
	content, err := json.Marshal([]Pick{
		{Ticker: "AAPL", Action: "BUY", Reasoning: "ok"},
		{Ticker: "MSFT", Action: "HOLD", Reasoning: "bad"},
		{Ticker: "NVDA", Action: "SELL", Reasoning: "ok"},
	})
	if err != nil {
		t.Fatalf("marshal picks: %v", err)
	}

	server, calls := openAITestServer([]string{
		wrapChatResponse(string(content)),
		wrapChatResponse(string(content)),
	})
	defer server.Close()

	client := NewClient("test-key",
		WithEndpoint(server.URL),
		WithHTTPClient(server.Client()),
		WithMaxAttempts(2),
	)

	_, err = client.GeneratePicks(context.Background())
	if err == nil {
		t.Fatalf("expected error for bad action")
	}
	if calls.Load() != 2 {
		t.Fatalf("expected 2 attempts, got %d", calls.Load())
	}
}

func TestGeneratePicksSuccess(t *testing.T) {
	content, err := json.Marshal([]Pick{
		{Ticker: "AAPL", Action: "BUY", Reasoning: "ok"},
		{Ticker: "MSFT", Action: "SELL", Reasoning: "ok"},
		{Ticker: "NVDA", Action: "BUY", Reasoning: "ok"},
	})
	if err != nil {
		t.Fatalf("marshal picks: %v", err)
	}

	server, calls := openAITestServer([]string{
		wrapChatResponse(string(content)),
	})
	defer server.Close()

	client := NewClient("test-key",
		WithEndpoint(server.URL),
		WithHTTPClient(server.Client()),
		WithMaxAttempts(2),
	)

	picks, err := client.GeneratePicks(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(picks) != 3 {
		t.Fatalf("expected 3 picks, got %d", len(picks))
	}
	if calls.Load() != 1 {
		t.Fatalf("expected 1 attempt, got %d", calls.Load())
	}
}

func TestGeneratePicksRetriesOnTransientStatus(t *testing.T) {
	content, err := json.Marshal([]Pick{
		{Ticker: "AAPL", Action: "BUY", Reasoning: "ok"},
		{Ticker: "MSFT", Action: "SELL", Reasoning: "ok"},
		{Ticker: "NVDA", Action: "BUY", Reasoning: "ok"},
	})
	if err != nil {
		t.Fatalf("marshal picks: %v", err)
	}

	server, calls := openAITestServerWithResponses([]openAIResponse{
		{status: http.StatusTooManyRequests, body: `{"error":"rate limited"}`},
		{status: http.StatusInternalServerError, body: `{"error":"server error"}`},
		{status: http.StatusOK, body: wrapChatResponse(string(content))},
	})
	defer server.Close()

	client := NewClient("test-key",
		WithEndpoint(server.URL),
		WithHTTPClient(server.Client()),
		WithMaxAttempts(1),
		WithRetryConfig(retry.Config{MaxAttempts: 3, BaseDelay: 0, MaxDelay: 0, Jitter: 0}),
	)

	picks, err := client.GeneratePicks(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(picks) != 3 {
		t.Fatalf("expected 3 picks, got %d", len(picks))
	}
	if calls.Load() != 3 {
		t.Fatalf("expected 3 attempts, got %d", calls.Load())
	}
}

func openAITestServer(responses []string) (*httptest.Server, *atomic.Int32) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idx := int(calls.Add(1)) - 1
		if idx >= len(responses) {
			idx = len(responses) - 1
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(responses[idx]))
	}))
	return server, &calls
}

type openAIResponse struct {
	status int
	body   string
}

func openAITestServerWithResponses(responses []openAIResponse) (*httptest.Server, *atomic.Int32) {
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

func wrapChatResponse(content string) string {
	resp := map[string]interface{}{
		"choices": []map[string]interface{}{
			{"message": map[string]interface{}{"content": content}},
		},
	}
	data, _ := json.Marshal(resp)
	return string(data)
}
