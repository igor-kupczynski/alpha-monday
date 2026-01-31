package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"

	"github.com/igor-kupczynski/alpha-monday/internal/integrations/retry"
)

const (
	defaultEndpoint    = "https://api.openai.com/v1/chat/completions"
	defaultModel       = "gpt-4o-mini"
	defaultTemperature = 0.2
	defaultMaxAttempts = 2
)

var (
	ErrInvalidOutput = errors.New("invalid picks output")
	tickerPattern    = regexp.MustCompile(`^[A-Z]{1,5}$`)
)

type Client struct {
	apiKey      string
	model       string
	endpoint    string
	temperature float64
	maxAttempts int
	httpClient  *http.Client
	retryConfig retry.Config
}

type Option func(*Client)

func WithEndpoint(endpoint string) Option {
	return func(c *Client) {
		if strings.TrimSpace(endpoint) != "" {
			c.endpoint = strings.TrimSpace(endpoint)
		}
	}
}

func WithModel(model string) Option {
	return func(c *Client) {
		if strings.TrimSpace(model) != "" {
			c.model = strings.TrimSpace(model)
		}
	}
}

func WithTemperature(temp float64) Option {
	return func(c *Client) {
		if temp >= 0 {
			c.temperature = temp
		}
	}
}

func WithMaxAttempts(attempts int) Option {
	return func(c *Client) {
		if attempts > 0 {
			c.maxAttempts = attempts
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
		model:       defaultModel,
		endpoint:    defaultEndpoint,
		temperature: defaultTemperature,
		maxAttempts: defaultMaxAttempts,
		httpClient:  http.DefaultClient,
		retryConfig: retry.DefaultConfig(),
	}

	for _, opt := range opts {
		opt(client)
	}

	if client.maxAttempts < 1 {
		client.maxAttempts = 1
	}

	return client
}

type Pick struct {
	Ticker    string `json:"ticker"`
	Action    string `json:"action"`
	Reasoning string `json:"reasoning"`
}

func (c *Client) GeneratePicks(ctx context.Context) ([]Pick, error) {
	if strings.TrimSpace(c.apiKey) == "" {
		return nil, fmt.Errorf("openai api key is required")
	}

	var lastErr error
	for attempt := 1; attempt <= c.maxAttempts; attempt++ {
		content, err := c.request(ctx)
		if err != nil {
			return nil, err
		}
		picks, err := parseAndValidate(content)
		if err == nil {
			return picks, nil
		}
		lastErr = err
	}

	if lastErr == nil {
		lastErr = ErrInvalidOutput
	}
	return nil, fmt.Errorf("openai output invalid after %d attempts: %w", c.maxAttempts, lastErr)
}

type chatRequest struct {
	Model       string    `json:"model"`
	Temperature float64   `json:"temperature,omitempty"`
	Messages    []message `json:"messages"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (c *Client) request(ctx context.Context) (string, error) {
	var content string
	err := retry.Do(ctx, c.retryConfig, isRetryableError, func() error {
		result, err := c.requestOnce(ctx)
		if err != nil {
			return err
		}
		content = result
		return nil
	})
	if err != nil {
		return "", err
	}
	return content, nil
}

func (c *Client) requestOnce(ctx context.Context) (string, error) {
	reqBody := chatRequest{
		Model:       c.model,
		Temperature: c.temperature,
		Messages: []message{
			{
				Role: "system",
				Content: "You are a stock analyst. Return exactly 3 unique S&P 500 tickers with BUY/SELL and reasoning. " +
					"Output only a JSON array of objects with fields ticker, action, reasoning. No extra text.",
			},
			{
				Role:    "user",
				Content: "Provide 3 unique S&P 500 picks in strict JSON array format.",
			},
		},
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", httpStatusError{
			status: resp.StatusCode,
			msg:    fmt.Sprintf("openai request failed: status %s: %s", resp.Status, strings.TrimSpace(string(body))),
		}
	}

	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("openai response missing choices")
	}
	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("openai response missing content")
	}
	return content, nil
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
	if errors.As(err, &netErr) {
		return true
	}
	return false
}

func parseAndValidate(content string) ([]Pick, error) {
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()

	var picks []Pick
	if err := decoder.Decode(&picks); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidOutput, err)
	}
	if err := ensureEOF(decoder); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidOutput, err)
	}

	if err := validatePicks(picks); err != nil {
		return nil, err
	}
	return picks, nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra interface{}
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	}
	return fmt.Errorf("extra json content detected")
}

func validatePicks(picks []Pick) error {
	if len(picks) != 3 {
		return fmt.Errorf("%w: expected 3 picks, got %d", ErrInvalidOutput, len(picks))
	}
	seen := map[string]bool{}
	for _, pick := range picks {
		ticker := strings.TrimSpace(pick.Ticker)
		if !tickerPattern.MatchString(ticker) {
			return fmt.Errorf("%w: invalid ticker %q", ErrInvalidOutput, pick.Ticker)
		}
		if seen[ticker] {
			return fmt.Errorf("%w: duplicate ticker %q", ErrInvalidOutput, ticker)
		}
		seen[ticker] = true
		if pick.Action != "BUY" && pick.Action != "SELL" {
			return fmt.Errorf("%w: invalid action %q", ErrInvalidOutput, pick.Action)
		}
		if strings.TrimSpace(pick.Reasoning) == "" {
			return fmt.Errorf("%w: missing reasoning for %s", ErrInvalidOutput, ticker)
		}
	}
	return nil
}
