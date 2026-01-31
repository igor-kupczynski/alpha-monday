package alphavantage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/igor-kupczynski/alpha-monday/internal/integrations/retry"
)

const defaultBaseURL = "https://www.alphavantage.co/query"

type Client struct {
	apiKey      string
	baseURL     string
	httpClient  *http.Client
	retryConfig retry.Config
}

type Quote struct {
	Symbol        string
	PreviousClose string
	TradingDay    string
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

func WithRetryConfig(config retry.Config) Option {
	return func(c *Client) {
		c.retryConfig = config
	}
}

func NewClient(apiKey string, opts ...Option) *Client {
	client := &Client{
		apiKey:      strings.TrimSpace(apiKey),
		baseURL:     defaultBaseURL,
		httpClient:  http.DefaultClient,
		retryConfig: retry.DefaultConfig(),
	}

	for _, opt := range opts {
		opt(client)
	}

	return client
}

func (c *Client) SnapshotPreviousCloses(ctx context.Context, benchmark string, picks []string) (map[string]Quote, error) {
	benchmark = strings.TrimSpace(benchmark)
	if benchmark == "" {
		return nil, fmt.Errorf("benchmark symbol is required")
	}
	if strings.TrimSpace(c.apiKey) == "" {
		return nil, fmt.Errorf("alpha vantage api key is required")
	}

	result := map[string]Quote{}
	benchmarkQuote, err := c.FetchPreviousClose(ctx, benchmark)
	if err != nil {
		return nil, err
	}
	if err := requireQuote(benchmarkQuote); err != nil {
		return nil, err
	}
	result[benchmark] = benchmarkQuote

	for _, pick := range picks {
		ticker := strings.TrimSpace(pick)
		if ticker == "" {
			return nil, fmt.Errorf("pick ticker is required")
		}
		if _, seen := result[ticker]; seen {
			continue
		}
		quote, err := c.FetchPreviousClose(ctx, ticker)
		if err != nil {
			return nil, err
		}
		if err := requireQuote(quote); err != nil {
			return nil, err
		}
		result[ticker] = quote
	}

	return result, nil
}

type globalQuoteResponse struct {
	GlobalQuote map[string]string `json:"Global Quote"`
}

func (c *Client) FetchPreviousClose(ctx context.Context, symbol string) (Quote, error) {
	var quote Quote
	err := retry.Do(ctx, c.retryConfig, isRetryableError, func() error {
		result, err := c.fetchPreviousCloseOnce(ctx, symbol)
		if err != nil {
			return err
		}
		quote = result
		return nil
	})
	if err != nil {
		return Quote{}, err
	}
	return quote, nil
}

func (c *Client) fetchPreviousCloseOnce(ctx context.Context, symbol string) (Quote, error) {
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return Quote{}, fmt.Errorf("symbol is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL, nil)
	if err != nil {
		return Quote{}, fmt.Errorf("build request: %w", err)
	}

	query := req.URL.Query()
	query.Set("function", "GLOBAL_QUOTE")
	query.Set("symbol", symbol)
	query.Set("apikey", c.apiKey)
	req.URL.RawQuery = query.Encode()

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Quote{}, fmt.Errorf("alpha vantage request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return Quote{}, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Quote{}, httpStatusError{
			status: resp.StatusCode,
			msg:    fmt.Sprintf("alpha vantage request failed: status %s: %s", resp.Status, strings.TrimSpace(string(body))),
		}
	}

	var parsed globalQuoteResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Quote{}, fmt.Errorf("decode response: %w", err)
	}

	return Quote{
		Symbol:        symbol,
		PreviousClose: strings.TrimSpace(parsed.GlobalQuote["08. previous close"]),
		TradingDay:    strings.TrimSpace(parsed.GlobalQuote["07. latest trading day"]),
	}, nil
}

type httpStatusError struct {
	status int
	msg    string
}

func (e httpStatusError) Error() string {
	return e.msg
}

func (e httpStatusError) StatusCode() int {
	return e.status
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var statusErr httpStatusError
	if errors.As(err, &statusErr) {
		return statusErr.status == http.StatusTooManyRequests || statusErr.status >= 500
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func requireQuote(quote Quote) error {
	if strings.TrimSpace(quote.PreviousClose) == "" {
		return fmt.Errorf("missing previous close for %s", quote.Symbol)
	}
	if strings.TrimSpace(quote.TradingDay) == "" {
		return fmt.Errorf("missing trading day for %s", quote.Symbol)
	}
	return nil
}
