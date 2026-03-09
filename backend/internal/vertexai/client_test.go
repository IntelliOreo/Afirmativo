package vertexai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestEnsureCachedContentCreatesAndReusesCache(t *testing.T) {
	var (
		countTokensCalls int32
		createCalls      int32
	)

	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(r.URL.Path, ":countTokens"):
			atomic.AddInt32(&countTokensCalls, 1)
			return jsonResponse(http.StatusOK, map[string]any{"totalTokens": 4096}), nil
		case strings.HasSuffix(r.URL.Path, "/cachedContents"):
			atomic.AddInt32(&createCalls, 1)
			return jsonResponse(http.StatusOK, map[string]any{
				"name":       "projects/test/locations/global/cachedContents/interview",
				"expireTime": time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339),
			}), nil
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
			return nil, nil
		}
	})}

	client, err := NewClient(ClientConfig{
		BaseURL:              "https://vertex.test/v1/",
		ProjectID:            "test",
		Location:             "global",
		APIKey:               "vertex-key",
		AuthMode:             AuthModeAPIKey,
		HTTPClient:           httpClient,
		TimeoutSeconds:       5,
		ExplicitCacheEnabled: true,
		ContextCacheTTL:      300 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	spec := CacheSpec{
		Key:         "interview:prompt",
		Model:       "gemini-3.1-flash-lite-preview",
		DisplayName: "interview",
		SystemInstruction: &Content{
			Parts: []Part{{Text: strings.Repeat("x", 3000)}},
		},
	}

	first, err := client.EnsureCachedContent(context.Background(), spec)
	if err != nil {
		t.Fatalf("EnsureCachedContent() error = %v", err)
	}
	second, err := client.EnsureCachedContent(context.Background(), spec)
	if err != nil {
		t.Fatalf("EnsureCachedContent() second error = %v", err)
	}

	if first.Mode != "explicit_cache" || second.Mode != "explicit_cache" {
		t.Fatalf("cache modes = [%s %s], want explicit_cache", first.Mode, second.Mode)
	}
	if atomic.LoadInt32(&countTokensCalls) != 1 {
		t.Fatalf("countTokensCalls = %d, want 1", countTokensCalls)
	}
	if atomic.LoadInt32(&createCalls) != 1 {
		t.Fatalf("createCalls = %d, want 1", createCalls)
	}
}

func TestEnsureCachedContentSkipsExplicitCacheBelowThreshold(t *testing.T) {
	var countTokensCalls int32
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, ":countTokens") {
			atomic.AddInt32(&countTokensCalls, 1)
			return jsonResponse(http.StatusOK, map[string]any{"totalTokens": 1024}), nil
		}
		t.Fatalf("unexpected path %s", r.URL.Path)
		return nil, nil
	})}

	client, err := NewClient(ClientConfig{
		BaseURL:              "https://vertex.test/v1/",
		ProjectID:            "test",
		Location:             "global",
		APIKey:               "vertex-key",
		AuthMode:             AuthModeAPIKey,
		HTTPClient:           httpClient,
		TimeoutSeconds:       5,
		ExplicitCacheEnabled: true,
		ContextCacheTTL:      300 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	spec := CacheSpec{
		Key:         "report:prompt",
		Model:       "gemini-3.1-flash-lite-preview",
		DisplayName: "report",
		SystemInstruction: &Content{
			Parts: []Part{{Text: "too small"}},
		},
	}

	first, err := client.EnsureCachedContent(context.Background(), spec)
	if err != nil {
		t.Fatalf("EnsureCachedContent() error = %v", err)
	}
	second, err := client.EnsureCachedContent(context.Background(), spec)
	if err != nil {
		t.Fatalf("EnsureCachedContent() second error = %v", err)
	}

	if first.Mode != "implicit_only_below_threshold" || second.Mode != "implicit_only_below_threshold" {
		t.Fatalf("cache modes = [%s %s], want implicit_only_below_threshold", first.Mode, second.Mode)
	}
	if atomic.LoadInt32(&countTokensCalls) != 1 {
		t.Fatalf("countTokensCalls = %d, want 1", countTokensCalls)
	}
}

func TestGenerateContentUsesAPIKeyHeader(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("x-goog-api-key"); got != "vertex-key" {
			t.Fatalf("x-goog-api-key = %q, want vertex-key", got)
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"parts": []map[string]any{{"text": `{"next_question":"Hola","evaluation":null}`}},
					},
					"finishReason": "STOP",
				},
			},
		}), nil
	})}

	client, err := NewClient(ClientConfig{
		BaseURL:              "https://vertex.test/v1/",
		ProjectID:            "test",
		Location:             "global",
		APIKey:               "vertex-key",
		AuthMode:             AuthModeAPIKey,
		HTTPClient:           httpClient,
		TimeoutSeconds:       5,
		ExplicitCacheEnabled: false,
		ContextCacheTTL:      300 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	resp, err := client.GenerateContent(context.Background(), "gemini-3.1-flash-lite-preview", GenerateContentRequest{
		Contents: []Content{NewTextContent("user", "hello")},
	})
	if err != nil {
		t.Fatalf("GenerateContent() error = %v", err)
	}
	if len(resp.Candidates) != 1 {
		t.Fatalf("len(resp.Candidates) = %d, want 1", len(resp.Candidates))
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(statusCode int, body any) *http.Response {
	raw, _ := json.Marshal(body)
	return &http.Response{
		StatusCode: statusCode,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(raw))),
	}
}
