package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	tokenGrantPath       = "/v1/auth/grant"
	tokenGrantLegacyPath = "/v1/auth/tokens/grant"
	defaultTimeoutSeconds = 30
)

// APIError wraps non-2xx provider responses.
type APIError struct {
	StatusCode int
}

func (e *APIError) Error() string {
	return fmt.Sprintf("voice provider status %d", e.StatusCode)
}

// ClientConfig configures the Voice AI HTTP client.
type ClientConfig struct {
	BaseURL        string
	APIKey         string
	Model          string
	Provider       string
	TimeoutSeconds int
}

// TokenGrantResponse is normalized from the voice provider token grant response.
type TokenGrantResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// Client calls a voice-provider token grant API.
type Client struct {
	baseURL  string
	apiKey   string
	model    string
	provider string
	client   *http.Client
}

// NewClient constructs a Voice AI client.
func NewClient(cfg ClientConfig) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("VOICE_AI_BASE_URL is required")
	}
	timeoutSeconds := cfg.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultTimeoutSeconds
	}

	return &Client{
		baseURL:  baseURL,
		apiKey:   strings.TrimSpace(cfg.APIKey),
		model:    strings.TrimSpace(cfg.Model),
		provider: strings.TrimSpace(cfg.Provider),
		client: &http.Client{
			Timeout: time.Duration(timeoutSeconds) * time.Second,
		},
	}, nil
}

// Provider returns configured provider label.
func (c *Client) Provider() string {
	return c.provider
}

// Model returns configured model label.
func (c *Client) Model() string {
	return c.model
}

// MintToken mints a short-lived provider token.
func (c *Client) MintToken(ctx context.Context, ttlSeconds int) (*TokenGrantResponse, error) {
	if ttlSeconds <= 0 {
		return nil, fmt.Errorf("ttl_seconds must be > 0")
	}
	if c.apiKey == "" {
		return nil, fmt.Errorf("VOICE_AI_API_KEY is required")
	}

	payload, err := json.Marshal(map[string]int{
		"ttl_seconds": ttlSeconds,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal token grant payload: %w", err)
	}
	slog.Debug("voice provider token grant payload",
		"provider", c.provider,
		"base_url", c.baseURL,
		"ttl_seconds", ttlSeconds,
	)

	// Try current path first, then compatibility alias used by the local mock binary.
	candidates := []string{tokenGrantPath, tokenGrantLegacyPath}
	var lastErr error
	for _, p := range candidates {
		slog.Debug("voice provider token grant attempt",
			"provider", c.provider,
			"path", p,
		)
		resp, callErr := c.mintTokenPath(ctx, p, payload)
		if callErr == nil {
			slog.Debug("voice provider token grant attempt succeeded",
				"provider", c.provider,
				"path", p,
				"expires_in", resp.ExpiresIn,
				"token_type", resp.TokenType,
			)
			return resp, nil
		}
		slog.Warn("voice provider token grant attempt failed",
			"provider", c.provider,
			"path", p,
			"error", callErr,
		)
		lastErr = callErr
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("token grant failed")
}

func (c *Client) mintTokenPath(ctx context.Context, path string, payload []byte) (*TokenGrantResponse, error) {
	start := time.Now()
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+path,
		bytes.NewReader(payload),
	)
	if err != nil {
		return nil, fmt.Errorf("build token grant request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Token "+c.apiKey)

	httpResp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("voice provider token grant request failed: %w", err)
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(httpResp.Body, 64*1024))
	if err != nil {
		return nil, fmt.Errorf("read token grant response: %w", err)
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		slog.Warn("voice provider token grant non-2xx response",
			"provider", c.provider,
			"path", path,
			"status", httpResp.StatusCode,
			"latency_ms", time.Since(start).Milliseconds(),
		)
		return nil, &APIError{StatusCode: httpResp.StatusCode}
	}

	var out TokenGrantResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode token grant response: %w", err)
	}
	if strings.TrimSpace(out.AccessToken) == "" {
		return nil, fmt.Errorf("token grant response missing access_token")
	}
	if strings.TrimSpace(out.TokenType) == "" {
		out.TokenType = "bearer"
	}
	slog.Debug("voice provider token grant response (redacted)",
		"provider", c.provider,
		"path", path,
		"status", httpResp.StatusCode,
		"latency_ms", time.Since(start).Milliseconds(),
		"expires_in", out.ExpiresIn,
		"token_type", out.TokenType,
		"access_token_set", strings.TrimSpace(out.AccessToken) != "",
	)

	return &out, nil
}
