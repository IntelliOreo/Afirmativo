package report

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/afirmativo/backend/internal/vertexai"
)

func TestVertexReportAIClientGenerateReport_UsesExplicitCacheAndParsesResponse(t *testing.T) {
	var (
		generateBody map[string]any
		logBuf       bytes.Buffer
	)

	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(r.URL.Path, ":countTokens"):
			return jsonResponse(http.StatusOK, map[string]any{"totalTokens": 4096}), nil
		case strings.HasSuffix(r.URL.Path, "/cachedContents"):
			return jsonResponse(http.StatusOK, map[string]any{
				"name":       "projects/test/locations/global/cachedContents/report",
				"expireTime": time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339),
			}), nil
		case strings.HasSuffix(r.URL.Path, ":generateContent"):
			if err := json.NewDecoder(r.Body).Decode(&generateBody); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			return jsonResponse(http.StatusOK, map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"parts": []map[string]any{{"text": `{"content_en":"English summary","content_es":"Resumen en espanol","areas_of_clarity":["clear point"],"areas_of_clarity_es":["punto claro"],"areas_to_develop_further":["develop point"],"areas_to_develop_further_es":["desarrollar punto"],"recommendation":"Keep practicing.","recommendation_es":"Siga practicando."}`}},
						},
						"finishReason": "STOP",
					},
				},
				"modelVersion": "gemini-3.1-flash-lite-preview",
				"usageMetadata": map[string]any{
					"promptTokenCount":        1234,
					"candidatesTokenCount":    321,
					"totalTokenCount":         1555,
					"cachedContentTokenCount": 111,
				},
			}), nil
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
			return nil, nil
		}
	})}

	vertexClient, err := vertexai.NewClient(vertexai.ClientConfig{
		BaseURL:              "https://vertex.test/v1/",
		ProjectID:            "test",
		Location:             "global",
		APIKey:               "vertex-key",
		AuthMode:             vertexai.AuthModeAPIKey,
		HTTPClient:           httpClient,
		TimeoutSeconds:       5,
		ExplicitCacheEnabled: true,
		ContextCacheTTL:      300 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	client := NewVertexReportAIClient(VertexReportAIClientConfig{
		Model:        "gemini-3.1-flash-lite-preview",
		MaxTokens:    2048,
		ReportPrompt: strings.Repeat("report prompt ", 180),
		VertexClient: vertexClient,
	})

	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	defer slog.SetDefault(previousDefault)

	result, err := client.GenerateReport(context.Background(), []AreaSummary{
		{Slug: "protected_ground", Label: "Protected ground", Status: "complete", EvidenceSummary: "summary", Recommendation: "move_on"},
	}, "open floor")
	if err != nil {
		t.Fatalf("GenerateReport() error = %v", err)
	}
	if result.ContentEn != "English summary" {
		t.Fatalf("result.ContentEn = %q, want English summary", result.ContentEn)
	}
	if got := stringValue(generateBody["cachedContent"]); got == "" {
		t.Fatalf("cachedContent = %q, want non-empty", got)
	}
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, `msg="AI API usage"`) || !strings.Contains(logOutput, `phase=report`) || !strings.Contains(logOutput, `provider=vertex`) {
		t.Fatalf("log output missing Vertex usage entry:\n%s", logOutput)
	}
}

func TestVertexReportAIClientGenerateReport_FallsBackToSystemInstructionBelowThreshold(t *testing.T) {
	var generateBody map[string]any

	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(r.URL.Path, ":countTokens"):
			return jsonResponse(http.StatusOK, map[string]any{"totalTokens": 100}), nil
		case strings.HasSuffix(r.URL.Path, ":generateContent"):
			if err := json.NewDecoder(r.Body).Decode(&generateBody); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			return jsonResponse(http.StatusOK, map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"parts": []map[string]any{{"text": `{"content_en":"English summary","content_es":"Resumen en espanol","areas_of_clarity":["clear point"],"areas_of_clarity_es":["punto claro"],"areas_to_develop_further":["develop point"],"areas_to_develop_further_es":["desarrollar punto"],"recommendation":"Keep practicing.","recommendation_es":"Siga practicando."}`}},
						},
						"finishReason": "STOP",
					},
				},
			}), nil
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
			return nil, nil
		}
	})}

	vertexClient, err := vertexai.NewClient(vertexai.ClientConfig{
		BaseURL:              "https://vertex.test/v1/",
		ProjectID:            "test",
		Location:             "global",
		APIKey:               "vertex-key",
		AuthMode:             vertexai.AuthModeAPIKey,
		HTTPClient:           httpClient,
		TimeoutSeconds:       5,
		ExplicitCacheEnabled: true,
		ContextCacheTTL:      300 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	client := NewVertexReportAIClient(VertexReportAIClientConfig{
		Model:        "gemini-3.1-flash-lite-preview",
		MaxTokens:    2048,
		ReportPrompt: "tiny prompt",
		VertexClient: vertexClient,
	})

	if _, err := client.GenerateReport(context.Background(), nil, "open floor"); err != nil {
		t.Fatalf("GenerateReport() error = %v", err)
	}
	if _, ok := generateBody["cachedContent"]; ok {
		t.Fatalf("cachedContent present for below-threshold prompt; body = %#v", generateBody)
	}
	if _, ok := generateBody["systemInstruction"]; !ok {
		t.Fatalf("systemInstruction missing from fallback request")
	}
}

func jsonResponse(statusCode int, body any) *http.Response {
	raw, _ := json.Marshal(body)
	return &http.Response{
		StatusCode: statusCode,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(raw))),
	}
}

func stringValue(value any) string {
	got, _ := value.(string)
	return got
}
