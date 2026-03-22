package vertexai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const defaultBaseURL = "https://aiplatform.googleapis.com/v1"

const (
	AuthModeAPIKey = "api_key"
	AuthModeADC    = "adc"
)

// ClientConfig configures the shared Vertex AI transport and cache manager.
type ClientConfig struct {
	BaseURL              string
	ProjectID            string
	Location             string
	APIKey               string
	AuthMode             string
	HTTPClient           *http.Client
	TimeoutSeconds       int
	ExplicitCacheEnabled bool
	ContextCacheTTL      time.Duration
}

// Part is a text part in the Vertex content schema.
type Part struct {
	Text string `json:"text,omitempty"`
}

// Content is a Vertex content block.
type Content struct {
	Role  string `json:"role,omitempty"`
	Parts []Part `json:"parts,omitempty"`
}

// GenerationConfig configures structured output generation.
type GenerationConfig struct {
	MaxOutputTokens  int            `json:"maxOutputTokens,omitempty"`
	ResponseMIMEType string         `json:"responseMimeType,omitempty"`
	ResponseSchema   map[string]any `json:"responseSchema,omitempty"`
}

// GenerateContentRequest is the request body for generateContent.
type GenerateContentRequest struct {
	SystemInstruction *Content         `json:"systemInstruction,omitempty"`
	Contents          []Content        `json:"contents,omitempty"`
	GenerationConfig  GenerationConfig `json:"generationConfig,omitempty"`
	CachedContent     string           `json:"cachedContent,omitempty"`
}

// CountTokensRequest mirrors the reusable request prefix for countTokens.
type CountTokensRequest struct {
	SystemInstruction *Content  `json:"systemInstruction,omitempty"`
	Contents          []Content `json:"contents,omitempty"`
}

// CreateCachedContentRequest creates a reusable explicit cache entry.
type CreateCachedContentRequest struct {
	DisplayName       string    `json:"displayName,omitempty"`
	Model             string    `json:"model,omitempty"`
	SystemInstruction *Content  `json:"systemInstruction,omitempty"`
	Contents          []Content `json:"contents,omitempty"`
	TTL               string    `json:"ttl,omitempty"`
}

// GenerateContentResponse is the subset of Vertex response fields this repo needs.
type GenerateContentResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	ModelVersion   string `json:"modelVersion"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	UsageMetadata struct {
		PromptTokenCount        int `json:"promptTokenCount"`
		CandidatesTokenCount    int `json:"candidatesTokenCount"`
		TotalTokenCount         int `json:"totalTokenCount"`
		CachedContentTokenCount int `json:"cachedContentTokenCount"`
	} `json:"usageMetadata"`
}

// CountTokensResponse is the subset of countTokens response fields this repo needs.
type CountTokensResponse struct {
	TotalTokens int `json:"totalTokens"`
}

// CachedContent is the subset of cache metadata this repo needs.
type CachedContent struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Model       string `json:"model"`
	ExpireTime  string `json:"expireTime"`
}

// CacheSpec identifies one stable prefix that can be explicitly cached.
type CacheSpec struct {
	Key               string
	Model             string
	DisplayName       string
	SystemInstruction *Content
	Contents          []Content
}

// CacheOutcome describes whether explicit caching was used.
type CacheOutcome struct {
	Mode              string
	CachedContentName string
}

type cacheRecord struct {
	Name          string
	ExpireAt      time.Time
	Ineligible    bool
	LastCheckedAt time.Time
}

// Client wraps Vertex REST calls plus explicit cache metadata.
type Client struct {
	baseURL              string
	projectID            string
	location             string
	apiKey               string
	authMode             string
	httpClient           *http.Client
	tokenSource          tokenSource
	explicitCacheEnabled bool
	contextCacheTTL      time.Duration
	nowFn                func() time.Time

	cacheMu sync.RWMutex
	cache   map[string]cacheRecord
	group   singleflight.Group
}

// NewClient constructs a shared Vertex client.
func NewClient(cfg ClientConfig) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	location := strings.TrimSpace(cfg.Location)
	if location == "" {
		location = "global"
	}
	authMode := strings.TrimSpace(cfg.AuthMode)
	if authMode == "" {
		authMode = AuthModeAPIKey
	}
	timeoutSeconds := cfg.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}
	contextCacheTTL := cfg.ContextCacheTTL
	if contextCacheTTL <= 0 {
		contextCacheTTL = 300 * time.Second
	}

	client := &Client{
		baseURL:              baseURL,
		projectID:            strings.TrimSpace(cfg.ProjectID),
		location:             location,
		apiKey:               strings.TrimSpace(cfg.APIKey),
		authMode:             authMode,
		httpClient:           cfg.HTTPClient,
		explicitCacheEnabled: cfg.ExplicitCacheEnabled,
		contextCacheTTL:      contextCacheTTL,
		nowFn:                time.Now,
		cache:                make(map[string]cacheRecord),
	}
	if client.httpClient == nil {
		client.httpClient = &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second}
	}

	switch authMode {
	case AuthModeAPIKey:
		if client.apiKey == "" {
			return nil, fmt.Errorf("VERTEX_AI_API_KEY is required when VERTEX_AI_AUTH_MODE=api_key")
		}
	case AuthModeADC:
		tokenSource, err := newADCTokenSource(client.httpClient, client.nowFn)
		if err != nil {
			return nil, err
		}
		client.tokenSource = tokenSource
	default:
		return nil, fmt.Errorf("unsupported Vertex auth mode %q", authMode)
	}

	return client, nil
}

// NewTextContent builds a single-text content block.
func NewTextContent(role, text string) Content {
	content := Content{
		Parts: []Part{{Text: text}},
	}
	if strings.TrimSpace(role) != "" {
		content.Role = role
	}
	return content
}

// GenerateContent calls the Vertex generateContent endpoint.
func (c *Client) GenerateContent(ctx context.Context, model string, reqBody GenerateContentRequest) (*GenerateContentResponse, error) {
	path := c.modelPath(model) + ":generateContent"
	var respBody GenerateContentResponse
	if err := c.doJSON(ctx, http.MethodPost, path, nil, reqBody, &respBody); err != nil {
		return nil, err
	}
	return &respBody, nil
}

// CountTokens calls the Vertex countTokens endpoint.
func (c *Client) CountTokens(ctx context.Context, model string, reqBody CountTokensRequest) (*CountTokensResponse, error) {
	path := c.modelPath(model) + ":countTokens"
	var respBody CountTokensResponse
	if err := c.doJSON(ctx, http.MethodPost, path, nil, reqBody, &respBody); err != nil {
		return nil, err
	}
	return &respBody, nil
}

// GetCachedContent fetches one cached-content resource.
func (c *Client) GetCachedContent(ctx context.Context, name string) (*CachedContent, error) {
	var cached CachedContent
	if err := c.doJSON(ctx, http.MethodGet, name, nil, nil, &cached); err != nil {
		return nil, err
	}
	return &cached, nil
}

// CreateCachedContent creates a new explicit cache entry.
func (c *Client) CreateCachedContent(ctx context.Context, reqBody CreateCachedContentRequest) (*CachedContent, error) {
	var cached CachedContent
	path := fmt.Sprintf("projects/%s/locations/%s/cachedContents", url.PathEscape(c.projectID), url.PathEscape(c.location))
	if err := c.doJSON(ctx, http.MethodPost, path, nil, reqBody, &cached); err != nil {
		return nil, err
	}
	return &cached, nil
}

// UpdateCachedContentTTL extends an existing explicit cache entry.
func (c *Client) UpdateCachedContentTTL(ctx context.Context, name string, ttl time.Duration) (*CachedContent, error) {
	var cached CachedContent
	query := url.Values{}
	query.Set("updateMask", "ttl")
	reqBody := map[string]string{
		"name": name,
		"ttl":  durationString(ttl),
	}
	if err := c.doJSON(ctx, http.MethodPatch, name, query, reqBody, &cached); err != nil {
		return nil, err
	}
	return &cached, nil
}

// EnsureCachedContent returns an explicit cache when possible and falls back cleanly otherwise.
func (c *Client) EnsureCachedContent(ctx context.Context, spec CacheSpec) (*CacheOutcome, error) {
	outcome := &CacheOutcome{Mode: "implicit_only"}
	if !c.explicitCacheEnabled {
		return outcome, nil
	}

	if record, ok := c.lookupCache(spec.Key); ok {
		switch {
		case record.Ineligible:
			outcome.Mode = "implicit_only_below_threshold"
			return outcome, nil
		case record.Name != "" && record.ExpireAt.After(c.nowFn().Add(15*time.Second)):
			outcome.Mode = "explicit_cache"
			outcome.CachedContentName = record.Name
			return outcome, nil
		}
	}

	value, err, _ := c.group.Do(spec.Key, func() (any, error) {
		return c.ensureCachedContentSingleflight(ctx, spec)
	})
	if err != nil {
		return outcome, err
	}
	createdOutcome, _ := value.(*CacheOutcome)
	if createdOutcome == nil {
		return outcome, nil
	}
	return createdOutcome, nil
}

func (c *Client) ensureCachedContentSingleflight(ctx context.Context, spec CacheSpec) (*CacheOutcome, error) {
	if record, ok := c.lookupCache(spec.Key); ok {
		switch {
		case record.Ineligible:
			return &CacheOutcome{Mode: "implicit_only_below_threshold"}, nil
		case record.Name != "" && record.ExpireAt.After(c.nowFn().Add(15*time.Second)):
			return &CacheOutcome{Mode: "explicit_cache", CachedContentName: record.Name}, nil
		}
	}

	countResp, err := c.CountTokens(ctx, spec.Model, CountTokensRequest{
		SystemInstruction: spec.SystemInstruction,
		Contents:          spec.Contents,
	})
	if err != nil {
		return &CacheOutcome{Mode: "implicit_only_cache_probe_failed"}, err
	}
	if countResp.TotalTokens < 2048 {
		c.storeCache(spec.Key, cacheRecord{
			Ineligible:    true,
			LastCheckedAt: c.nowFn(),
		})
		return &CacheOutcome{Mode: "implicit_only_below_threshold"}, nil
	}

	if record, ok := c.lookupCache(spec.Key); ok && record.Name != "" {
		cached, err := c.UpdateCachedContentTTL(ctx, record.Name, c.contextCacheTTL)
		if err == nil {
			expireAt := parseExpireTime(cached.ExpireTime, c.nowFn().Add(c.contextCacheTTL))
			c.storeCache(spec.Key, cacheRecord{Name: cached.Name, ExpireAt: expireAt, LastCheckedAt: c.nowFn()})
			return &CacheOutcome{Mode: "explicit_cache", CachedContentName: cached.Name}, nil
		}
	}

	cached, err := c.CreateCachedContent(ctx, CreateCachedContentRequest{
		DisplayName:       spec.DisplayName,
		Model:             c.modelResource(spec.Model),
		SystemInstruction: spec.SystemInstruction,
		Contents:          spec.Contents,
		TTL:               durationString(c.contextCacheTTL),
	})
	if err != nil {
		return &CacheOutcome{Mode: "implicit_only_cache_create_failed"}, err
	}
	expireAt := parseExpireTime(cached.ExpireTime, c.nowFn().Add(c.contextCacheTTL))
	c.storeCache(spec.Key, cacheRecord{Name: cached.Name, ExpireAt: expireAt, LastCheckedAt: c.nowFn()})
	return &CacheOutcome{Mode: "explicit_cache", CachedContentName: cached.Name}, nil
}

func (c *Client) lookupCache(key string) (cacheRecord, bool) {
	c.cacheMu.RLock()
	defer c.cacheMu.RUnlock()
	record, ok := c.cache[key]
	return record, ok
}

func (c *Client) storeCache(key string, record cacheRecord) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	c.cache[key] = record
}

func (c *Client) modelPath(model string) string {
	return c.modelResource(model)
}

func (c *Client) modelResource(model string) string {
	return fmt.Sprintf("projects/%s/locations/%s/publishers/google/models/%s",
		url.PathEscape(c.projectID),
		url.PathEscape(c.location),
		url.PathEscape(strings.TrimSpace(model)),
	)
}

func (c *Client) doJSON(ctx context.Context, method, path string, query url.Values, reqBody any, out any) error {
	requestURL := c.resourceURL(path, query)

	var bodyReader io.Reader
	if reqBody != nil {
		raw, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshal Vertex request: %w", err)
		}
		bodyReader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, requestURL, bodyReader)
	if err != nil {
		return fmt.Errorf("build Vertex request: %w", err)
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if err := c.authorizeRequest(ctx, req); err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Vertex request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read Vertex response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseAPIError(resp.StatusCode, respBody)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode Vertex response: %w", err)
	}
	return nil
}

func (c *Client) authorizeRequest(ctx context.Context, req *http.Request) error {
	switch c.authMode {
	case AuthModeAPIKey:
		req.Header.Set("x-goog-api-key", c.apiKey)
		return nil
	case AuthModeADC:
		token, err := c.tokenSource.Token(ctx)
		if err != nil {
			return fmt.Errorf("resolve Vertex access token: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	default:
		return fmt.Errorf("unsupported Vertex auth mode %q", c.authMode)
	}
}

func (c *Client) resourceURL(path string, query url.Values) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	base := c.baseURL
	if !strings.HasPrefix(path, "/") && !strings.Contains(path, "/v1/") {
		base += "/"
	}
	requestURL := base + strings.TrimLeft(path, "/")
	if query == nil || len(query) == 0 {
		return requestURL
	}
	return requestURL + "?" + query.Encode()
}

func durationString(value time.Duration) string {
	seconds := value.Seconds()
	return fmt.Sprintf("%.0fs", seconds)
}

func parseExpireTime(raw string, fallback time.Time) time.Time {
	if raw == "" {
		return fallback
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return fallback
	}
	return parsed
}

type APIError struct {
	StatusCode int
	Status     string
	Message    string
	RawBody    string
}

func (e *APIError) Error() string {
	if strings.TrimSpace(e.Message) != "" {
		return fmt.Sprintf("Vertex API status %d: %s", e.StatusCode, e.Message)
	}
	if strings.TrimSpace(e.Status) != "" {
		return fmt.Sprintf("Vertex API status %d: %s", e.StatusCode, e.Status)
	}
	return fmt.Sprintf("Vertex API status %d", e.StatusCode)
}

func parseAPIError(statusCode int, body []byte) error {
	var envelope struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && (envelope.Error.Code != 0 || envelope.Error.Message != "" || envelope.Error.Status != "") {
		return &APIError{
			StatusCode: statusCode,
			Status:     envelope.Error.Status,
			Message:    envelope.Error.Message,
			RawBody:    strings.TrimSpace(string(body)),
		}
	}
	return &APIError{
		StatusCode: statusCode,
		RawBody:    strings.TrimSpace(string(body)),
	}
}
