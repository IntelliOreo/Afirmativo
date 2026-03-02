// HTTP client that calls the Claude API to generate the final assessment report.
// Uses the same Claude Messages API as the interview AI client but with a
// different prompt and output schema tailored for report generation.
package report

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/afirmativo/backend/internal/shared"
)

// AIClient generates the report assessment via the configured AI provider.
type AIClient interface {
	GenerateReport(ctx context.Context, areaSummaries []AreaSummary, openFloorTranscript string) (*ReportAIResponse, error)
}

// ReportAIClientConfig holds config for the report AI client.
type ReportAIClientConfig struct {
	BaseURL        string // "https://api.anthropic.com" or mock URL
	APIKey         string
	Model          string
	MaxTokens      int
	TimeoutSeconds int    // HTTP timeout for AI API calls (e.g. 30)
	ReportPrompt   string // system prompt for report generation
}

// HTTPReportAIClient implements AIClient by calling the Claude Messages API.
type HTTPReportAIClient struct {
	baseURL        string
	apiKey         string
	model          string
	maxTokens      int
	timeoutSeconds int
	reportPrompt   string
	outputSchema   map[string]interface{}
	client         *http.Client
}

// NewHTTPReportAIClient creates a report AI client.
func NewHTTPReportAIClient(cfg ReportAIClientConfig) *HTTPReportAIClient {
	return &HTTPReportAIClient{
		baseURL:        cfg.BaseURL,
		apiKey:         cfg.APIKey,
		model:          cfg.Model,
		maxTokens:      cfg.MaxTokens,
		timeoutSeconds: cfg.TimeoutSeconds,
		reportPrompt:   cfg.ReportPrompt,
		outputSchema:   buildReportOutputSchema(),
		client:         &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second},
	}
}

// GenerateReport calls the Claude API with the area summaries and open floor transcript.
func (c *HTTPReportAIClient) GenerateReport(ctx context.Context, areaSummaries []AreaSummary, openFloorTranscript string) (*ReportAIResponse, error) {
	userContent := buildReportUserMessage(areaSummaries, openFloorTranscript)

	requestBody := map[string]interface{}{
		"model":      c.model,
		"max_tokens": c.maxTokens,
		"system":     c.reportPrompt,
		"messages": []map[string]interface{}{
			{"role": "user", "content": userContent},
		},
	}

	if c.outputSchema != nil {
		requestBody["output_config"] = c.outputSchema
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := c.baseURL + "/v1/messages"
	slog.Debug("calling AI API for report", "url", url, "model", c.model)
	if messages, ok := requestBody["messages"].([]map[string]interface{}); ok {
		shared.DebugChatMessages("report AI request messages", messages)
	}
	shared.DebugJSON("report AI request body", requestBody)

	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(c.timeoutSeconds)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	}

	start := time.Now()
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("AI API call failed: %w", err)
	}
	defer resp.Body.Close()

	slog.Debug("report AI API responded", "status", resp.StatusCode, "duration", time.Since(start))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AI API returned status %d", resp.StatusCode)
	}

	// Reuse the same Claude API envelope as the interview package.
	var apiResp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode API response: %w", err)
	}

	if apiResp.StopReason == "max_tokens" {
		return nil, fmt.Errorf("response truncated: stop_reason=max_tokens")
	}
	if len(apiResp.Content) == 0 {
		return nil, fmt.Errorf("empty content in API response")
	}

	jsonStr := apiResp.Content[0].Text
	shared.DebugJSONText("report AI raw response", jsonStr)

	var result ReportAIResponse
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("parse report AI response: %w", err)
	}

	slog.Debug("report AI response parsed",
		"strengths_count", len(result.Strengths),
		"weaknesses_count", len(result.Weaknesses),
		"recommendation_len", len(result.Recommendation),
	)

	return &result, nil
}

// buildReportUserMessage constructs the user prompt with area summaries and open floor text.
func buildReportUserMessage(summaries []AreaSummary, openFloorTranscript string) string {
	summariesJSON, _ := json.MarshalIndent(summaries, "", "  ")

	return fmt.Sprintf(`AREA EVALUATIONS:
%s

OPEN FLOOR TRANSCRIPT (applicant's own words, in Spanish):
%s

Please generate the assessment report based on the area evaluations above. If the open floor transcript addresses gaps in any insufficient areas, note that in your assessment.`,
		string(summariesJSON),
		openFloorTranscript,
	)
}

// buildReportOutputSchema creates the JSON schema for the report AI output.
func buildReportOutputSchema() map[string]interface{} {
	return map[string]interface{}{
		"format": map[string]interface{}{
			"type": "json_schema",
			"schema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"content_en": map[string]interface{}{
						"type":        "string",
						"description": "Full assessment report in English. Cover each area's result, overall case strength, and specific recommendations.",
					},
					"content_es": map[string]interface{}{
						"type":        "string",
						"description": "Full assessment report in Spanish. Same content as content_en but translated to Spanish.",
					},
					"strengths": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Array of strength bullet points (in English). Each should be a specific, actionable observation.",
					},
					"weaknesses": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Array of weakness/improvement bullet points (in English). Each should identify a gap and suggest how to address it.",
					},
					"recommendation": map[string]interface{}{
						"type":        "string",
						"description": "Overall recommendation (in English). Should state whether the case appears strong, needs work, or has significant gaps.",
					},
				},
				"required":             []string{"content_en", "content_es", "strengths", "weaknesses", "recommendation"},
				"additionalProperties": false,
			},
		},
	}
}
