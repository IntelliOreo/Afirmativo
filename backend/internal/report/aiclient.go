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
	"text/template"
	"time"

	"github.com/afirmativo/backend/internal/shared"
)

// ReportAIClient generates the report assessment via the configured AI provider.
type ReportAIClient interface {
	GenerateReport(ctx context.Context, areaSummaries []AreaSummary, openFloorTranscript string) (*ReportAIResponse, error)
}

// ReportAIClientConfig holds config for the report AI client.
type ReportAIClientConfig struct {
	BaseURL                 string // "https://api.anthropic.com" or mock URL
	APIKey                  string
	Model                   string
	MaxTokens               int
	TimeoutSeconds          int    // HTTP timeout for AI API calls (e.g. 30)
	ReportPrompt            string // system prompt for report generation
	AllowSensitiveDebugLogs bool
}

// HTTPReportAIClient implements ReportAIClient by calling the Claude Messages API.
type HTTPReportAIClient struct {
	baseURL                 string
	apiKey                  string
	model                   string
	maxTokens               int
	timeoutSeconds          int
	reportPrompt            string
	allowSensitiveDebugLogs bool
	outputSchema            map[string]interface{}
	client                  *http.Client
}

type claudeReportAPIResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
}

// NewHTTPReportAIClient creates a report AI client.
func NewHTTPReportAIClient(cfg ReportAIClientConfig) *HTTPReportAIClient {
	return &HTTPReportAIClient{
		baseURL:                 cfg.BaseURL,
		apiKey:                  cfg.APIKey,
		model:                   cfg.Model,
		maxTokens:               cfg.MaxTokens,
		timeoutSeconds:          cfg.TimeoutSeconds,
		reportPrompt:            cfg.ReportPrompt,
		allowSensitiveDebugLogs: cfg.AllowSensitiveDebugLogs,
		outputSchema:            buildReportOutputSchema(),
		client:                  &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second},
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
	slog.Debug("calling AI API for report",
		"url", url,
		"model", c.model,
		"sensitive_debug_logs_enabled", c.allowSensitiveDebugLogs,
	)
	if c.allowSensitiveDebugLogs {
		if messages, ok := requestBody["messages"].([]map[string]interface{}); ok {
			shared.DebugChatMessages("report AI request messages", messages)
		}
		shared.DebugJSON("report AI request body", requestBody)
	}

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

	var apiResp claudeReportAPIResponse
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
	if c.allowSensitiveDebugLogs {
		shared.DebugJSONText("report AI raw response", jsonStr)
	}

	var result ReportAIResponse
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("parse report AI response: %w", err)
	}
	if err := validateReportAIResponse(&result); err != nil {
		return nil, err
	}

	slog.Debug("report AI response parsed",
		"areas_of_clarity_count", len(result.AreasOfClarity),
		"areas_to_develop_further_count", len(result.AreasToDevelopFurther),
		"recommendation_len", len(result.Recommendation),
	)
	slog.Info("Claude API usage",
		"phase", "report",
		"model", c.model,
		"input_tokens", apiResp.Usage.InputTokens,
		"output_tokens", apiResp.Usage.OutputTokens,
		"cache_creation_input_tokens", apiResp.Usage.CacheCreationInputTokens,
		"cache_read_input_tokens", apiResp.Usage.CacheReadInputTokens,
	)

	return &result, nil
}

type reportUserPromptData struct {
	Summaries           []AreaSummary
	OpenFloorTranscript string
}

var reportUserMessageTemplate = template.Must(
	template.New("report_user_message").
		Funcs(template.FuncMap{
			"json": func(v interface{}) string {
				b, err := json.MarshalIndent(v, "", "  ")
				if err != nil {
					return "[]"
				}
				return string(b)
			},
		}).
		Parse(`AREA EVALUATIONS:
{{json .Summaries}}

OPEN FLOOR TRANSCRIPT (applicant's own words, in Spanish):
{{.OpenFloorTranscript}}

Please generate the assessment report based on the area evaluations above. If the open floor transcript addresses gaps in any insufficient areas, note that in your assessment.`),
)

// buildReportUserMessage constructs the user prompt with area summaries and open floor text.
func buildReportUserMessage(summaries []AreaSummary, openFloorTranscript string) string {
	data := reportUserPromptData{
		Summaries:           summaries,
		OpenFloorTranscript: openFloorTranscript,
	}
	var b bytes.Buffer
	if err := reportUserMessageTemplate.Execute(&b, data); err != nil {
		slog.Warn("failed to render report user prompt template; using fallback", "error", err)
		return buildReportUserMessageFallback(summaries, openFloorTranscript)
	}
	return b.String()
}

func buildReportUserMessageFallback(summaries []AreaSummary, openFloorTranscript string) string {
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
						"description": "Full preparation feedback summary in English. Focus only on whether the applicant has practiced articulating the relevant elements clearly. Cover each area's result and specific recommendations.",
					},
					"content_es": map[string]interface{}{
						"type":        "string",
						"description": "Full preparation feedback summary in Spanish. Same content as content_en but translated to Spanish.",
					},
					"areas_of_clarity": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Array of 'areas of clarity' bullet points (in English). Keep each point specific and actionable.",
					},
					"areas_to_develop_further": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Array of 'areas to develop further' bullet points (in English). Each should identify a gap and suggest how to address it.",
					},
					"recommendation": map[string]interface{}{
						"type":        "string",
						"description": "Overall recommendation (in English). Focus on preparation quality and whether key elements were articulated clearly.",
					},
				},
				"required":             []string{"content_en", "content_es", "areas_of_clarity", "areas_to_develop_further", "recommendation"},
				"additionalProperties": false,
			},
		},
	}
}
