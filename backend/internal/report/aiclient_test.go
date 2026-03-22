package report

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

func TestHTTPReportAIClientGenerateReport_ParsesUsageAndLogsIt(t *testing.T) {
	var requestCount int
	client := NewHTTPReportAIClient(ReportAIClientConfig{
		BaseURL:        "https://api.anthropic.com",
		Model:          "claude-3-haiku-20240307",
		MaxTokens:      1024,
		TimeoutSeconds: 5,
		ReportPrompt:   "report prompt",
	})
	client.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requestCount++
		body := io.NopCloser(strings.NewReader(`{
			"content": [
				{
					"type": "text",
					"text": "{\"content_en\":\"English summary\",\"content_es\":\"Resumen en espanol\",\"areas_of_clarity\":[\"clear point\"],\"areas_of_clarity_es\":[\"punto claro\"],\"areas_to_develop_further\":[\"develop point\"],\"areas_to_develop_further_es\":[\"desarrollar punto\"],\"recommendation\":\"Keep practicing.\",\"recommendation_es\":\"Siga practicando.\"}"
				}
			],
			"stop_reason": "end_turn",
			"usage": {
				"input_tokens": 1234,
				"output_tokens": 321,
				"cache_creation_input_tokens": 111,
				"cache_read_input_tokens": 222
			}
		}`))
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       body,
		}, nil
	})}

	var logBuf bytes.Buffer
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	defer slog.SetDefault(previousDefault)

	result, err := client.GenerateReport(context.Background(), []AreaSummary{
		{Slug: "protected_ground", Label: "Protected ground", Status: "complete", EvidenceSummary: "summary", Recommendation: "move_on"},
	}, "open floor")
	if err != nil {
		t.Fatalf("GenerateReport() error = %v", err)
	}
	if requestCount != 1 {
		t.Fatalf("requestCount = %d, want 1", requestCount)
	}
	if result == nil {
		t.Fatal("GenerateReport() = nil, want non-nil response")
	}
	if result.ContentEn != "English summary" {
		t.Fatalf("result.ContentEn = %q, want English summary", result.ContentEn)
	}
	if result.ContentEs != "Resumen en espanol" {
		t.Fatalf("result.ContentEs = %q, want Resumen en espanol", result.ContentEs)
	}
	if len(result.AreasOfClarityEs) != 1 || result.AreasOfClarityEs[0] != "punto claro" {
		t.Fatalf("result.AreasOfClarityEs = %#v, want [\"punto claro\"]", result.AreasOfClarityEs)
	}
	if len(result.AreasToDevelopFurtherEs) != 1 || result.AreasToDevelopFurtherEs[0] != "desarrollar punto" {
		t.Fatalf("result.AreasToDevelopFurtherEs = %#v, want [\"desarrollar punto\"]", result.AreasToDevelopFurtherEs)
	}
	if result.RecommendationEs != "Siga practicando." {
		t.Fatalf("result.RecommendationEs = %q, want Siga practicando.", result.RecommendationEs)
	}

	logOutput := logBuf.String()
	for _, want := range []string{
		`msg="Claude API usage"`,
		`phase=report`,
		`model=claude-3-haiku-20240307`,
		`input_tokens=1234`,
		`output_tokens=321`,
		`cache_creation_input_tokens=111`,
		`cache_read_input_tokens=222`,
	} {
		if !strings.Contains(logOutput, want) {
			t.Fatalf("log output missing %q:\n%s", want, logOutput)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
