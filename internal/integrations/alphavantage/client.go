package alphavantage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const defaultBaseURL = "https://www.alphavantage.co/query"

type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

type Option func(*Client)

func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		if strings.TrimSpace(baseURL) != "" {
			c.baseURL = strings.TrimSpace(baseURL)
		}
	}
}

func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		if client != nil {
			c.httpClient = client
		}
	}
}

func NewClient(apiKey string, opts ...Option) *Client {
	client := &Client{
		apiKey:     strings.TrimSpace(apiKey),
		baseURL:    defaultBaseURL,
		httpClient: http.DefaultClient,
	}

	for _, opt := range opts {
		opt(client)
	}

	return client
}

func (c *Client) SnapshotPreviousCloses(ctx context.Context, benchmark string, picks []string) (map[string]string, error) {
	benchmark = strings.TrimSpace(benchmark)
	if benchmark == "" {
		return nil, fmt.Errorf("benchmark symbol is required")
	}
	if strings.TrimSpace(c.apiKey) == "" {
		return nil, fmt.Errorf("alpha vantage api key is required")
	}

	result := map[string]string{}
	benchmarkPrice, err := c.previousClose(ctx, benchmark)
	if err != nil {
		return nil, err
	}
	result[benchmark] = benchmarkPrice

	for _, pick := range picks {
		ticker := strings.TrimSpace(pick)
		if ticker == "" {
			return nil, fmt.Errorf("pick ticker is required")
		}
		if _, seen := result[ticker]; seen {
			continue
		}
		price, err := c.previousClose(ctx, ticker)
		if err != nil {
			return nil, err
		}
		result[ticker] = price
	}

	return result, nil
}

type globalQuoteResponse struct {
	GlobalQuote map[string]string `json:"Global Quote"`
}

func (c *Client) previousClose(ctx context.Context, symbol string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}

	query := req.URL.Query()
	query.Set("function", "GLOBAL_QUOTE")
	query.Set("symbol", symbol)
	query.Set("apikey", c.apiKey)
	req.URL.RawQuery = query.Encode()

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("alpha vantage request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("alpha vantage request failed: status %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var parsed globalQuoteResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	price := strings.TrimSpace(parsed.GlobalQuote["08. previous close"])
	if price == "" {
		return "", fmt.Errorf("missing previous close for %s", symbol)
	}
	return price, nil
}
