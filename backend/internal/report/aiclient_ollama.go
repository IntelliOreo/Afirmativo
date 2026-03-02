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
	BaseURL            string
	Model              string
	TimeoutSeconds     int
	ReportPrompt       string
	OutputFormatPrompt string
}

// OllamaReportAIClient implements AIClient using Ollama's OpenAI-compatible endpoint.
type OllamaReportAIClient struct {
	baseURL            string
	model              string
	reportPrompt       string
	outputFormatPrompt string
	client             *http.Client
}

// NewOllamaReportAIClient creates a new report AI client backed by Ollama.
func NewOllamaReportAIClient(cfg OllamaReportAIClientConfig) *OllamaReportAIClient {
	return &OllamaReportAIClient{
		baseURL:            cfg.BaseURL,
		model:              cfg.Model,
		reportPrompt:       cfg.ReportPrompt,
		outputFormatPrompt: cfg.OutputFormatPrompt,
		client:             &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second},
	}
}

// GenerateReport sends area summaries to Ollama and parses structured JSON output.
func (c *OllamaReportAIClient) GenerateReport(ctx context.Context, areaSummaries []AreaSummary, openFloorTranscript string) (*ReportAIResponse, error) {
	userContent := buildReportUserMessage(areaSummaries, openFloorTranscript)
	systemPrompt := c.reportPrompt
	if c.outputFormatPrompt != "" {
		systemPrompt += "\n\n" + c.outputFormatPrompt
	}

	requestBody := map[string]interface{}{
		"model": c.model,
		"messages": []map[string]interface{}{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userContent},
		},
		"temperature":     0.3,
		"response_format": map[string]interface{}{"type": "json_object"},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(c.baseURL, "/") + "/v1/chat/completions"
	slog.Debug("calling Ollama API for report", "url", url, "model", c.model)
	if messages, ok := requestBody["messages"].([]map[string]interface{}); ok {
		shared.DebugChatMessages("report Ollama request messages", messages)
	}
	shared.DebugJSON("report Ollama request body", requestBody)

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
	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("empty choices in API response")
	}
	if apiResp.Choices[0].FinishReason == "length" {
		return nil, fmt.Errorf("response truncated: finish_reason=length")
	}

	jsonStr := apiResp.Choices[0].Message.Content
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
		"strengths_count", len(result.Strengths),
		"weaknesses_count", len(result.Weaknesses),
		"recommendation_len", len(result.Recommendation),
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
	if result.Strengths == nil {
		return fmt.Errorf("invalid report AI response: strengths must be an array")
	}
	if result.Weaknesses == nil {
		return fmt.Errorf("invalid report AI response: weaknesses must be an array")
	}
	if strings.TrimSpace(result.Recommendation) == "" {
		return fmt.Errorf("invalid report AI response: recommendation is empty")
	}
	return nil
}
