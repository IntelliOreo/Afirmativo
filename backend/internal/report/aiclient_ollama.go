package report

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/afirmativo/backend/internal/shared"
)

// OllamaReportAIClientConfig holds config for the Ollama report AI client.
type OllamaReportAIClientConfig struct {
	BaseURL                 string
	Model                   string
	MaxTokens               int
	Temperature             float64
	TimeoutSeconds          int
	ReportPrompt            string
	OutputFormatPrompt      string
	AllowSensitiveDebugLogs bool
}

// OllamaReportAIClient implements ReportAIClient using Ollama's OpenAI-compatible endpoint.
type OllamaReportAIClient struct {
	baseURL                 string
	model                   string
	maxTokens               int
	temperature             float64
	reportPrompt            string
	outputFormatPrompt      string
	allowSensitiveDebugLogs bool
	client                  *http.Client
}

// NewOllamaReportAIClient creates a new report AI client backed by Ollama.
func NewOllamaReportAIClient(cfg OllamaReportAIClientConfig) *OllamaReportAIClient {
	return &OllamaReportAIClient{
		baseURL:                 cfg.BaseURL,
		model:                   cfg.Model,
		maxTokens:               cfg.MaxTokens,
		temperature:             cfg.Temperature,
		reportPrompt:            cfg.ReportPrompt,
		outputFormatPrompt:      cfg.OutputFormatPrompt,
		allowSensitiveDebugLogs: cfg.AllowSensitiveDebugLogs,
		client:                  &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second},
	}
}

// GenerateReport sends area summaries to Ollama and parses structured JSON output.
func (c *OllamaReportAIClient) GenerateReport(ctx context.Context, areaSummaries []AreaSummary, openFloorTranscript string) (*ReportAIResponse, error) {
	userContent := buildReportUserMessage(areaSummaries, openFloorTranscript)
	systemPrompt := c.reportPrompt
	if c.outputFormatPrompt != "" {
		systemPrompt += "\n\n" + c.outputFormatPrompt
	}
	systemPrompt += "\n\n" + c.buildReportFieldInstruction()

	requestBody := map[string]interface{}{
		"model":      c.model,
		"max_tokens": c.maxTokens,
		"messages": []map[string]interface{}{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userContent},
		},
		"temperature":     c.temperature,
		"response_format": map[string]interface{}{"type": "json_object"},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(c.baseURL, "/") + "/v1/chat/completions"
	slog.Debug("calling Ollama API for report",
		"url", url,
		"model", c.model,
		"sensitive_debug_logs_enabled", c.allowSensitiveDebugLogs,
	)
	if c.allowSensitiveDebugLogs {
		if messages, ok := requestBody["messages"].([]map[string]interface{}); ok {
			shared.DebugChatMessages("report Ollama request messages", messages)
		}
		shared.DebugJSON("report Ollama request body", requestBody)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Ollama API call failed: %w", err)
	}
	defer resp.Body.Close()

	slog.Debug("report Ollama API responded", "status", resp.StatusCode, "duration", time.Since(start))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama API returned status %d", resp.StatusCode)
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode API response: %w", err)
	}
	if c.allowSensitiveDebugLogs {
		shared.DebugJSON("report Ollama raw response envelope", apiResp)
	}
	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("empty choices in API response")
	}
	if apiResp.Choices[0].FinishReason == "length" {
		return nil, fmt.Errorf("response truncated: finish_reason=length")
	}

	jsonStr := apiResp.Choices[0].Message.Content
	if c.allowSensitiveDebugLogs {
		shared.DebugJSONText("report Ollama raw response", jsonStr)
		slog.Debug("report Ollama raw response captured",
			"chars", len(jsonStr),
			"sensitive_debug_logs_enabled", c.allowSensitiveDebugLogs,
		)
	}
	if strings.TrimSpace(jsonStr) == "" {
		return nil, fmt.Errorf("empty message content in API response")
	}

	var result ReportAIResponse
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("parse report AI response: %w", err)
	}
	if err := validateReportAIResponse(&result); err != nil {
		return nil, err
	}

	slog.Debug("report Ollama response parsed",
		"areas_of_clarity_count", len(result.AreasOfClarity),
		"areas_to_develop_further_count", len(result.AreasToDevelopFurther),
		"recommendation_len", len(result.Recommendation),
		"areas_of_clarity_es_count", len(result.AreasOfClarityEs),
		"areas_to_develop_further_es_count", len(result.AreasToDevelopFurtherEs),
		"recommendation_es_len", len(result.RecommendationEs),
	)

	return &result, nil
}

func validateReportAIResponse(result *ReportAIResponse) error {
	if strings.TrimSpace(result.ContentEn) == "" {
		return fmt.Errorf("invalid report AI response: content_en is empty")
	}
	if strings.TrimSpace(result.ContentEs) == "" {
		return fmt.Errorf("invalid report AI response: content_es is empty")
	}
	if result.AreasOfClarity == nil {
		return fmt.Errorf("invalid report AI response: areas_of_clarity must be an array")
	}
	if result.AreasOfClarityEs == nil {
		return fmt.Errorf("invalid report AI response: areas_of_clarity_es must be an array")
	}
	if result.AreasToDevelopFurther == nil {
		return fmt.Errorf("invalid report AI response: areas_to_develop_further must be an array")
	}
	if result.AreasToDevelopFurtherEs == nil {
		return fmt.Errorf("invalid report AI response: areas_to_develop_further_es must be an array")
	}
	if strings.TrimSpace(result.Recommendation) == "" {
		return fmt.Errorf("invalid report AI response: recommendation is empty")
	}
	if strings.TrimSpace(result.RecommendationEs) == "" {
		return fmt.Errorf("invalid report AI response: recommendation_es is empty")
	}
	return nil
}

func (c *OllamaReportAIClient) buildReportFieldInstruction() string {
	return `Use JSON keys "areas_of_clarity", "areas_of_clarity_es", "areas_to_develop_further", "areas_to_develop_further_es", "recommendation", and "recommendation_es".`
}
